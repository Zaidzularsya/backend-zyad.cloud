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

func (f *FakeClient) Add(_ context.Context, path string, attrs Row) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.Err != nil {
		return "", f.Err
	}
	if f.tables == nil {
		f.tables = map[string][]Row{}
	}
	f.nextID++
	id := "*" + strconv.FormatInt(int64(f.nextID), 16)
	row := Row{".id": id}
	for k, v := range attrs {
		row[k] = v
	}
	f.tables[path] = append(f.tables[path], row)
	return id, nil
}

func (f *FakeClient) Set(_ context.Context, path, id string, attrs Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.Err != nil {
		return f.Err
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

func (f *FakeClient) Remove(_ context.Context, path, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.Err != nil {
		return f.Err
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
