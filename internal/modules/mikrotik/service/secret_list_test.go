package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

type recordingAccounts struct {
	fakeAccounts
	q     string
	limit int
}

func (r *recordingAccounts) Search(_ context.Context, _ coretenant.Scope, q string, limit int) ([]AccountRef, error) {
	r.q, r.limit = q, limit
	return []AccountRef{{ID: "a1", Name: "Alice"}}, nil
}

func TestSecretList_DelegatesToRepo(t *testing.T) {
	e := newSecretEnv(t)
	paired := true
	f := repository.SecretFilter{RouterID: "r1", Paired: &paired, Search: "ali"}
	if _, _, err := e.svc.List(context.Background(), e.scope, f, 25, 50); err != nil {
		t.Fatal(err)
	}
	if e.srepo.listFilter.RouterID != "r1" || e.srepo.listFilter.Search != "ali" || e.srepo.listLimit != 25 || e.srepo.listOffset != 50 {
		t.Fatalf("repo got %+v %d %d", e.srepo.listFilter, e.srepo.listLimit, e.srepo.listOffset)
	}
}

func TestSearchAccounts_ValidatesAndClamps(t *testing.T) {
	e := newSecretEnv(t)
	acc := &recordingAccounts{}
	e.svc.accounts = acc

	if _, err := e.svc.SearchAccounts(context.Background(), e.scope, " a ", 10); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("short q: err = %v", err)
	}
	if acc.q != "" {
		t.Fatal("directory must not be called for a short query")
	}
	for _, tc := range []struct{ in, want int }{{0, 20}, {-5, 20}, {500, 20}, {7, 7}} {
		if _, err := e.svc.SearchAccounts(context.Background(), e.scope, " alice ", tc.in); err != nil {
			t.Fatal(err)
		}
		if acc.q != "alice" || acc.limit != tc.want {
			t.Fatalf("limit %d: directory got %q %d, want alice %d", tc.in, acc.q, acc.limit, tc.want)
		}
	}
}

func TestSecretSummary_CountsActiveAndInactive(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(
		pmk.Row{"name": "a", "service": "pppoe", "profile": "p", "disabled": "false"},
		pmk.Row{"name": "b", "service": "pppoe", "profile": "p", "disabled": "true"},
		pmk.Row{"name": "c", "service": "pppoe", "profile": "p", "disabled": "false"},
	)
	e.doImport(t)
	got, err := e.svc.Summary(context.Background(), e.scope, e.router.ID)
	if err != nil || got.Active != 2 || got.Inactive != 1 {
		t.Fatalf("summary = %+v err=%v", got, err)
	}
	if e.srepo.summaryRouter != e.router.ID {
		t.Fatalf("router filter = %q", e.srepo.summaryRouter)
	}
}

func TestList_FilterByMappingStatus(t *testing.T) {
	yes, no := true, false
	cases := map[string]repository.SecretFilter{
		"":               {},
		"all":            {Paired: &no},
		"suggested":      {Paired: &no, Suggestion: repository.SuggestionFilterSuggested},
		"needs_decision": {Paired: &no, Suggestion: repository.SuggestionFilterNeedsDecision},
		"needs_profile":  {NeedsOriginalProfile: &yes},
		"paired":         {Paired: &yes},
	}
	for status, want := range cases {
		got, err := ApplyMappingStatus(repository.SecretFilter{RouterID: "r1"}, status)
		if err != nil {
			t.Fatalf("%q: %v", status, err)
		}
		want.RouterID = "r1"
		if fmt.Sprint(deref(got.Paired), got.Suggestion, deref(got.NeedsOriginalProfile), got.RouterID) !=
			fmt.Sprint(deref(want.Paired), want.Suggestion, deref(want.NeedsOriginalProfile), want.RouterID) {
			t.Errorf("%q: got %+v, want %+v", status, got, want)
		}
	}
	if _, err := ApplyMappingStatus(repository.SecretFilter{}, "bogus"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("bogus: err = %v", err)
	}
}

func deref(b *bool) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprint(*b)
}
