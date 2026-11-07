package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{
		"0812-3456-7890":     "6281234567890",
		"+62 812 3456 7890":  "6281234567890",
		"6281234567890":      "6281234567890",
		"":                   "",
		"abc":                "",
		"123":                "",
		"81234567890":        "6281234567890",  // mobile without prefix
		"0062 812 3456 789":  "628123456789",   // 00 international prefix
		"1234567890123456":   "",               // 16 digits: over the 15 cap
		"+62 0812":           "",               // 6 digits left: too short
		"+62 0812 3456 7890": "62081234567890", // 62 kept as-is, no stripping of the inner 0
	} {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchSecret_Phone(t *testing.T) {
	accs := []AccountRef{{ID: "a1", Name: "Budi", Phone: "0812 3456 7890"}, {ID: "a2", Name: "081234567890"}}
	id, basis, n := MatchSecret("+6281234567890", accs)
	if id != "a1" || basis != "phone" || n != 1 {
		t.Fatalf("got %q %q %d", id, basis, n)
	}
}

func TestMatchSecret_NameExactCaseInsensitive(t *testing.T) {
	accs := []AccountRef{{ID: "a1", Name: "  Budi Santoso "}, {ID: "a2", Name: "Budi"}}
	id, basis, n := MatchSecret("budi santoso", accs)
	if id != "a1" || basis != "name" || n != 1 {
		t.Fatalf("got %q %q %d", id, basis, n)
	}
	if id, basis, n := MatchSecret("budi san", accs); id != "" || basis != "" || n != 0 {
		t.Fatalf("partial must not match: %q %q %d", id, basis, n)
	}
}

func TestMatchSecret_AmbiguousTwoAccounts(t *testing.T) {
	accs := []AccountRef{{ID: "a1", Name: "Budi"}, {ID: "a2", Name: "BUDI"}, {ID: "a3", Name: "Sari"}}
	id, basis, n := MatchSecret("budi", accs)
	if id != "" || basis != "ambiguous" || n != 2 {
		t.Fatalf("got %q %q %d", id, basis, n)
	}
}

func TestMatchSecret_EmptyNameNeverMatches(t *testing.T) {
	accs := []AccountRef{{ID: "a1", Name: "", Phone: ""}, {ID: "a2", Name: "  "}}
	for _, name := range []string{"", "   "} {
		if id, basis, n := MatchSecret(name, accs); id != "" || basis != "" || n != 0 {
			t.Fatalf("%q matched: %q %q %d", name, id, basis, n)
		}
	}
}

// ---- Analyze ----

const sugRouterID = "11111111-1111-1111-1111-111111111111"

type sugRouters struct{}

func (sugRouters) Get(_ context.Context, _ coretenant.Scope, id string) (domain.Router, error) {
	if id != sugRouterID {
		return domain.Router{}, mikrotik.ErrRouterNotFound
	}
	return domain.Router{ID: id}, nil
}

type sugSecrets struct{ rows []domain.PPPSecret }

func (f sugSecrets) List(_ context.Context, _ coretenant.Scope, flt repository.SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error) {
	var all []domain.PPPSecret
	for _, r := range f.rows {
		if r.RouterID != flt.RouterID || (flt.Paired != nil && !*flt.Paired && r.AccountID != nil) {
			continue
		}
		all = append(all, r)
	}
	total := int64(len(all))
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

// sugAccounts caps pages at 200 like the real adapter.
type sugAccounts struct {
	fakeAccounts
	all []AccountRef
}

func (f sugAccounts) List(_ context.Context, _ coretenant.Scope, after string, limit int) ([]AccountRef, error) {
	if limit > 200 {
		limit = 200
	}
	var out []AccountRef
	for _, a := range f.all {
		if a.ID > after && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

type sugStore struct {
	mu    sync.Mutex
	rows  []repository.Suggestion
	calls int
}

func (s *sugStore) ReplaceForRouter(_ context.Context, _ coretenant.Scope, _ string, rows []repository.Suggestion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.rows = append([]repository.Suggestion(nil), rows...)
	return nil
}

type sugLocker struct {
	mu   sync.Mutex
	held map[string]bool
}

func (l *sugLocker) TryLock(_ context.Context, key string) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held == nil {
		l.held = map[string]bool{}
	}
	if l.held[key] {
		return nil, false, nil
	}
	l.held[key] = true
	return func() { l.mu.Lock(); delete(l.held, key); l.mu.Unlock() }, true, nil
}

func sec(id, name string, present bool, acct *string) domain.PPPSecret {
	return domain.PPPSecret{ID: id, RouterID: sugRouterID, Name: name, Present: present, AccountID: acct}
}

func TestAnalyze_SkipsPairedAndMissing(t *testing.T) {
	paired := "a9"
	secrets := sugSecrets{rows: []domain.PPPSecret{
		sec("s1", "Budi", true, nil),        // name match
		sec("s2", "08123456789", true, nil), // phone match
		sec("s3", "kembar", true, nil),      // ambiguous
		sec("s4", "tak-ada", true, nil),     // no match
		sec("s5", "Budi", true, &paired),    // paired: skipped
		sec("s6", "Sari", false, nil),       // missing on router: skipped
	}}
	accs := sugAccounts{all: []AccountRef{
		{ID: "a1", Name: "budi"}, {ID: "a2", Name: "x", Phone: "+62 8123 456 789"},
		{ID: "a3", Name: "Kembar"}, {ID: "a4", Name: "kembar"}, {ID: "a5", Name: "Sari"},
	}}
	store := &sugStore{}
	svc := NewSuggestionService(sugRouters{}, secrets, accs, store, &sugLocker{})
	res, err := svc.Analyze(context.Background(), testScope(t), sugRouterID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res != (AnalyzeResult{Total: 4, Suggested: 2, NeedsDecision: 2}) {
		t.Fatalf("res = %+v", res)
	}
	got := map[string]repository.Suggestion{}
	for _, r := range store.rows {
		got[r.SecretID] = r
	}
	if len(got) != 3 || got["s1"].AccountID != "a1" || got["s1"].Basis != "name" ||
		got["s2"].AccountID != "a2" || got["s2"].Basis != "phone" ||
		got["s3"].Basis != "ambiguous" || got["s3"].AccountID != "" || got["s3"].Candidates != 2 {
		t.Fatalf("rows = %+v", store.rows)
	}
}

func TestAnalyze_ReadsAllAccountPages(t *testing.T) {
	var all []AccountRef
	for i := 0; i < 1234; i++ {
		all = append(all, AccountRef{ID: fmt.Sprintf("a%05d", i), Name: fmt.Sprintf("cust-%d", i)})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	secrets := sugSecrets{rows: []domain.PPPSecret{sec("s1", "cust-1233", true, nil)}}
	store := &sugStore{}
	svc := NewSuggestionService(sugRouters{}, secrets, sugAccounts{all: all}, store, &sugLocker{})
	res, err := svc.Analyze(context.Background(), testScope(t), sugRouterID, "u1")
	if err != nil || res.Suggested != 1 || store.rows[0].AccountID != "a01233" {
		t.Fatalf("res=%+v err=%v rows=%+v", res, err, store.rows)
	}
}

func TestAnalyze_ConcurrentCallsIdempotent(t *testing.T) {
	secrets := sugSecrets{rows: []domain.PPPSecret{sec("s1", "Budi", true, nil)}}
	accs := sugAccounts{all: []AccountRef{{ID: "a1", Name: "Budi"}}}
	store := &sugStore{}
	locker := &sugLocker{}
	svc := NewSuggestionService(sugRouters{}, secrets, accs, store, locker)
	scope := testScope(t)

	// Lock held by another analysis: refused, nothing written.
	unlock, ok, _ := locker.TryLock(context.Background(), "mikrotik.suggest:"+scope.OrganizationID()+":"+sugRouterID)
	if !ok {
		t.Fatal("setup lock")
	}
	if _, err := svc.Analyze(context.Background(), scope, sugRouterID, "u1"); !errors.Is(err, mikrotik.ErrAnalyzeInProgress) {
		t.Fatalf("err = %v", err)
	}
	if store.calls != 0 {
		t.Fatal("must not write while locked")
	}
	unlock()

	// Concurrent calls: each either completes or reports in-progress; final state is the same.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Analyze(context.Background(), scope, sugRouterID, "u1"); err != nil && !errors.Is(err, mikrotik.ErrAnalyzeInProgress) {
				t.Errorf("err = %v", err)
			}
		}()
	}
	wg.Wait()
	if len(store.rows) != 1 || store.rows[0].AccountID != "a1" {
		t.Fatalf("rows = %+v", store.rows)
	}
	if len(locker.held) != 0 {
		t.Fatal("lock leaked")
	}
}

// stuckAccounts always returns the same page, as a broken adapter would.
type stuckAccounts struct{ fakeAccounts }

func (stuckAccounts) List(context.Context, coretenant.Scope, string, int) ([]AccountRef, error) {
	return []AccountRef{{ID: "a1", Name: "Budi"}}, nil
}

func TestAnalyze_StopsOnNonAdvancingAccountPage(t *testing.T) {
	secrets := sugSecrets{rows: []domain.PPPSecret{sec("s1", "Budi", true, nil)}}
	svc := NewSuggestionService(sugRouters{}, secrets, stuckAccounts{}, &sugStore{}, &sugLocker{})
	done := make(chan error, 1)
	go func() { _, err := svc.Analyze(context.Background(), testScope(t), sugRouterID, "u1"); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Analyze did not terminate on a non-advancing account page")
	}
}
