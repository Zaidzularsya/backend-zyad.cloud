package mikrotik

import (
	"context"
	"strconv"
	"sync"
)

// FakeClient is an in-memory RouterClient for tests: one table per path, with
// automatic ids. Set Err to make every call fail.
type FakeClient struct {
	Err error
	// VersionString is returned by Version (default "7.0-fake").
	VersionString string

	// IgnoreFilters makes Print return every row of the path regardless of the
	// query, to model a router/transport whose server-side filtering is unreliable.
	IgnoreFilters bool
	// WriteHook, when set, runs before every Add/Set/Remove (op is "add",
	// "set" or "remove"); a non-nil result fails that call. The call still
	// counts in WriteCalls.
	WriteHook func(op, path string, attrs Row) error

	mu     sync.Mutex
	tables map[string][]Row
	nextID int
	writes int
}

func NewFakeClient() *FakeClient {
	return &FakeClient{tables: map[string][]Row{}}
}

// WriteCalls counts Add, Set and Remove calls (including failed ones).
func (f *FakeClient) WriteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

// Seed inserts rows directly (ids assigned when missing) without counting a
// write, to model pre-existing router state.
func (f *FakeClient) Seed(path string, rows ...Row) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tables == nil {
		f.tables = map[string][]Row{}
	}
	for _, r := range rows {
		row := Row{}
		for k, v := range r {
			row[k] = v
		}
		if row[".id"] == "" {
			f.nextID++
			row[".id"] = "*" + strconv.FormatInt(int64(f.nextID), 16)
		}
		f.tables[path] = append(f.tables[path], row)
	}
}

// Rows returns a copy of the table at path, in router order.
func (f *FakeClient) Rows(path string) []Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Row, 0, len(f.tables[path]))
	for _, r := range f.tables[path] {
		cp := make(Row, len(r))
		for k, v := range r {
			cp[k] = v
		}
		out = append(out, cp)
	}
	return out
}

func (f *FakeClient) hook(op, path string, attrs Row) error {
	if f.WriteHook != nil {
		return f.WriteHook(op, path, attrs)
	}
	return nil
}

func (f *FakeClient) Version(context.Context) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	if f.VersionString != "" {
		return f.VersionString, nil
	}
	return "7.0-fake", nil
}

func (f *FakeClient) Print(_ context.Context, path string, query Row) ([]Row, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	var out []Row
	for _, r := range f.tables[path] {
		match := true
		for k, v := range query {
			if f.IgnoreFilters {
				break
			}
			if r[k] != v {
				match = false
				break
			}
		}
		if match {
			cp := make(Row, len(r))
			for k, v := range r {
				cp[k] = v
			}
			out = append(out, cp)
		}
	}
	return out, nil
}

func (f *FakeClient) Add(ctx context.Context, path string, attrs Row) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.Err != nil {
		return "", f.Err
	}
	if f.tables == nil {
		f.tables = map[string][]Row{}
	}
	if err := f.hook("add", path, attrs); err != nil {
		return "", err
	}
	f.nextID++
	id := "*" + strconv.FormatInt(int64(f.nextID), 16)
	row := Row{".id": id}
	before := ""
	for k, v := range attrs {
		if k == "place-before" {
			// Like RouterOS: a placement hint, never stored on the record.
			before = v
			continue
		}
		row[k] = v
	}
	rows := f.tables[path]
	if before != "" {
		for i, r := range rows {
			if r[".id"] == before {
				out := make([]Row, 0, len(rows)+1)
				out = append(out, rows[:i]...)
				out = append(out, row)
				out = append(out, rows[i:]...)
				f.tables[path] = out
				return id, nil
			}
		}
		return "", ErrNotFound
	}
	f.tables[path] = append(rows, row)
	return id, nil
}

func (f *FakeClient) Set(ctx context.Context, path, id string, attrs Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.Err != nil {
		return f.Err
	}
	if err := f.hook("set", path, attrs); err != nil {
		return err
	}
	for _, r := range f.tables[path] {
		if r[".id"] == id {
			for k, v := range attrs {
				r[k] = v
			}
			return nil
		}
	}
	return ErrNotFound
}

func (f *FakeClient) Remove(ctx context.Context, path, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.Err != nil {
		return f.Err
	}
	if err := f.hook("remove", path, nil); err != nil {
		return err
	}
	rows := f.tables[path]
	for i, r := range rows {
		if r[".id"] == id {
			f.tables[path] = append(rows[:i:i], rows[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}
