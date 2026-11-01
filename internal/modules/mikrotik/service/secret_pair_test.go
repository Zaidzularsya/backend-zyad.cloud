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

func (f fakeAccounts) List(context.Context, coretenant.Scope, string, int) ([]AccountRef, error) {
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

func TestUnpair_ActiveTaskRejected_NoAuditPairingKept(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	e.srepo.unpairErr = mikrotik.ErrSecretHasActiveTask
	audits := len(e.rrepo.audits)
	if err := e.svc.Unpair(context.Background(), e.scope, id, testActor); !errors.Is(err, mikrotik.ErrSecretHasActiveTask) {
		t.Fatalf("err = %v", err)
	}
	if e.srepo.byID(id).AccountID == nil || len(e.rrepo.audits) != audits {
		t.Fatal("pairing must be kept and nothing audited")
	}
}

func TestPairBulk_PartialFailureReportsPerRow(t *testing.T) {
	e := newPairEnv(t)
	ok := e.importOne(t, "alice", "paket")
	taken := e.importOne(t, "bob", "paket")
	if _, err := e.svc.Pair(context.Background(), e.scope, taken, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	audits := len(e.rrepo.audits)

	res := e.svc.PairBulk(context.Background(), e.scope, []PairItem{
		{SecretID: taken, AccountID: "acct-1"},
		{SecretID: ok, AccountID: "acct-1"},
		{SecretID: ok, AccountID: "acct-missing"},
		{SecretID: "nope", AccountID: "acct-1"},
	}, testActor)
	if len(res) != 4 {
		t.Fatalf("results = %+v", res)
	}
	if res[0].OK || res[0].Code != "MIKROTIK_SECRET_ALREADY_PAIRED" || res[0].SecretID != taken || res[0].Message == "" {
		t.Errorf("row0 = %+v", res[0])
	}
	if !res[1].OK || res[1].Code != "" {
		t.Errorf("row1 = %+v", res[1])
	}
	// Row 2 targets a secret row 1 just paired: the earlier row did not abort the later ones.
	if res[2].OK || res[2].Code != "MIKROTIK_SECRET_ALREADY_PAIRED" {
		t.Errorf("row2 = %+v", res[2])
	}
	if res[3].OK || res[3].Code != "MIKROTIK_SECRET_NOT_FOUND" {
		t.Errorf("row3 = %+v", res[3])
	}
	if got := len(e.rrepo.audits) - audits; got != 1 {
		t.Errorf("audit events = %d, want 1 (one per success)", got)
	}
}

func TestPairBulk_UnknownErrorIsGeneric(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "paket")
	e.svc.accounts = errAccounts{err: errors.New("dial 10.0.0.1 password=hunter2")}
	res := e.svc.PairBulk(context.Background(), e.scope, []PairItem{{SecretID: id, AccountID: "acct-1"}}, testActor)
	if res[0].OK || res[0].Code != "MIKROTIK_INTERNAL" || strings.Contains(res[0].Message, "hunter2") {
		t.Fatalf("row = %+v", res[0])
	}
}

type errAccounts struct {
	fakeAccounts
	err error
}

func (f errAccounts) Get(context.Context, coretenant.Scope, string) (AccountRef, error) {
	return AccountRef{}, f.err
}

func TestPairBulk_CancelledContextSkipsRemainingRows(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "paket")
	audits := len(e.rrepo.audits)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := e.svc.PairBulk(ctx, e.scope, []PairItem{{SecretID: id, AccountID: "acct-1"}, {SecretID: "x", AccountID: "acct-1"}}, testActor)
	if len(res) != 2 {
		t.Fatalf("results = %+v", res)
	}
	for i, r := range res {
		if r.OK || r.Code != "MIKROTIK_INTERNAL" || r.Message != "internal error" {
			t.Errorf("row%d = %+v", i, r)
		}
	}
	if res[0].SecretID != id {
		t.Errorf("secret id lost: %+v", res[0])
	}
	got, err := e.svc.repo.FindByID(context.Background(), e.scope, id)
	if err != nil || got.AccountID != nil {
		t.Fatalf("secret must stay unpaired: %+v %v", got, err)
	}
	if len(e.rrepo.audits) != audits {
		t.Error("no audit expected")
	}
}
