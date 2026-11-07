package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/repository"
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
