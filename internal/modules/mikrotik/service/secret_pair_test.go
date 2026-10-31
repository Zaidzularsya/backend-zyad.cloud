package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

type fakeAccounts struct{ known map[string]AccountRef }

func (f fakeAccounts) Get(_ context.Context, _ coretenant.Scope, id string) (AccountRef, error) {
	if a, ok := f.known[id]; ok {
		return a, nil
	}
	return AccountRef{}, mikrotik.ErrAccountNotFound
}

func (f fakeAccounts) Search(context.Context, coretenant.Scope, string, int) ([]AccountRef, error) {
	return nil, nil
}

type fakeProvisioner struct {
	calls   int
	gotName string
	err     error
}

func (f *fakeProvisioner) CreateCustomerFromSecret(_ context.Context, _ coretenant.Scope, name, _, _ string) (AccountRef, error) {
	f.calls++
	f.gotName = name
	if f.err != nil {
		return AccountRef{}, f.err
	}
	return AccountRef{ID: "acct-new", Name: name}, nil
}

type pairEnv struct {
	secretEnv
	prov *fakeProvisioner
}

func newPairEnv(t *testing.T) pairEnv {
	t.Helper()
	e := newSecretEnv(t)
	prov := &fakeProvisioner{}
	e.svc.accounts = fakeAccounts{known: map[string]AccountRef{"acct-1": {ID: "acct-1", Name: "Alice"}}}
	e.svc.provisioner = prov
	return pairEnv{secretEnv: e, prov: prov}
}

// importOne seeds one secret on the router and returns its cached id.
func (e pairEnv) importOne(t *testing.T, name, profile string) string {
	t.Helper()
	e.seed(pmk.Row{"name": name, "service": "pppoe", "profile": profile})
	e.doImport(t)
	return e.srepo.byName(e.router.ID, name).ID
}

func (e pairEnv) lastAudit(t *testing.T) (event string, meta map[string]any) {
	t.Helper()
	a := e.rrepo.audits[len(e.rrepo.audits)-1]
	return a.Event, a.Metadata
}

func TestPair_Success(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "paket-10m")
	got, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if got.AccountID == nil || *got.AccountID != "acct-1" {
		t.Fatalf("secret = %+v", got)
	}
	ev, meta := e.lastAudit(t)
	if ev != "secret.paired" || meta["target"] != "secret" || meta["secret_id"] != id || meta["account_id"] != "acct-1" {
		t.Fatalf("audit = %s %+v", ev, meta)
	}
}

func TestPair_SetsOriginalProfileFromCurrent(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "paket-10m")
	got, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if err != nil || got.OriginalProfile != "paket-10m" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestPair_AccountOfOtherOrgNotFound(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	_, err := e.svc.Pair(context.Background(), e.scope, id, "acct-other-org", testActor)
	if !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
	if e.srepo.pairCalls != 0 {
		t.Fatal("repo.Pair must not run")
	}
}

func TestPair_AlreadyPairedConflict(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if !errors.Is(err, mikrotik.ErrSecretAlreadyPaired) {
		t.Fatalf("err = %v", err)
	}
}

func TestPair_NeedsOriginalProfileRejected(t *testing.T) {
	e := newPairEnv(t)
	e.setIsolirProfile("isolir")
	id := e.importOne(t, "alice", "isolir")
	_, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("err = %v", err)
	}
}

func TestPair_MissingSecretRejected(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	e.srepo.byID(id).Present = false
	_, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("err = %v", err)
	}
}

func TestPair_UnknownSecret(t *testing.T) {
	e := newPairEnv(t)
	if _, err := e.svc.Pair(context.Background(), e.scope, "nope", "acct-1", testActor); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestPair_UsesRouterIsolirProfile(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "isolir") // router has no isolir profile yet -> not flagged
	e.setIsolirProfile("isolir")
	// Secret is on the (now configured) isolir profile without an original profile.
	_, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor)
	if !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("err = %v, want not pairable via router isolir profile", err)
	}
}

func TestPairNewCustomer_CreatesContactAndAccount(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	got, err := e.svc.PairNewCustomer(context.Background(), e.scope, id, testActor)
	if err != nil {
		t.Fatal(err)
	}
	if e.prov.calls != 1 || e.prov.gotName != "alice" {
		t.Fatalf("provisioner calls=%d name=%q", e.prov.calls, e.prov.gotName)
	}
	if got.AccountID == nil || *got.AccountID != "acct-new" {
		t.Fatalf("secret = %+v", got)
	}
	if ev, meta := e.lastAudit(t); ev != "secret.paired" || meta["account_id"] != "acct-new" {
		t.Fatalf("audit = %s %+v", ev, meta)
	}
}

func TestPairNewCustomer_NotCalledWhenNotPairable(t *testing.T) {
	e := newPairEnv(t)
	e.setIsolirProfile("isolir")
	id := e.importOne(t, "alice", "isolir")
	_, err := e.svc.PairNewCustomer(context.Background(), e.scope, id, testActor)
	if !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("err = %v", err)
	}
	if e.prov.calls != 0 {
		t.Fatal("provisioner must not be called")
	}
}

func TestPairNewCustomer_AlreadyPairedSkipsProvisioner(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PairNewCustomer(context.Background(), e.scope, id, testActor); !errors.Is(err, mikrotik.ErrSecretAlreadyPaired) {
		t.Fatalf("err = %v", err)
	}
	if e.prov.calls != 0 {
		t.Fatal("provisioner must not be called")
	}
}

func TestUnpair_Success(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Unpair(context.Background(), e.scope, id, testActor); err != nil {
		t.Fatal(err)
	}
	if e.srepo.byID(id).AccountID != nil {
		t.Fatal("still paired")
	}
	if ev, meta := e.lastAudit(t); ev != "secret.unpaired" || meta["secret_id"] != id || meta["account_id"] != "acct-1" {
		t.Fatalf("audit = %s %+v", ev, meta)
	}
}

func TestUnpair_IsolatedRejected(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	e.srepo.byID(id).IsolationState = domain.IsolationIsolated
	if err := e.svc.Unpair(context.Background(), e.scope, id, testActor); !errors.Is(err, mikrotik.ErrSecretIsolated) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetOriginalProfile_RejectsIsolirProfileAndUnknownProfile(t *testing.T) {
	e := newPairEnv(t)
	e.setIsolirProfile("isolir")
	e.client.Seed("/ppp/profile", pmk.Row{"name": "paket-10m"}, pmk.Row{"name": "isolir"})
	id := e.importOne(t, "alice", "isolir")

	if err := e.svc.SetOriginalProfile(context.Background(), e.scope, id, "isolir", testActor); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("isolir profile: err = %v", err)
	}
	if err := e.svc.SetOriginalProfile(context.Background(), e.scope, id, "ghost", testActor); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("unknown profile: err = %v", err)
	}
	if err := e.svc.SetOriginalProfile(context.Background(), e.scope, id, "  ", testActor); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("blank profile: err = %v", err)
	}
	if !e.srepo.byID(id).NeedsOriginalProfile {
		t.Fatal("nothing may change on rejection")
	}

	if err := e.svc.SetOriginalProfile(context.Background(), e.scope, id, "paket-10m", testActor); err != nil {
		t.Fatal(err)
	}
	got := e.srepo.byID(id)
	if got.OriginalProfile != "paket-10m" || got.NeedsOriginalProfile {
		t.Fatalf("secret = %+v", got)
	}
	if ev, meta := e.lastAudit(t); ev != "secret.original_profile_set" || meta["secret_id"] != id {
		t.Fatalf("audit = %s %+v", ev, meta)
	}
	// Now pairable.
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatalf("pair after fix: %v", err)
	}
}

func TestSetOriginalProfile_RouterErrorScrubbed(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	e.client.Err = errors.New("boom " + testPassword)
	err := e.svc.SetOriginalProfile(context.Background(), e.scope, id, "x", testActor)
	if !errors.Is(err, mikrotik.ErrUnreachable) || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("err = %v", err)
	}
}
