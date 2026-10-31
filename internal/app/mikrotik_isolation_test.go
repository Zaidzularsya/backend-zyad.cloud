package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"zyad.cloud/internal/config"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

type isoOrgs struct{ ids []string }

func (o isoOrgs) ListActive(_ context.Context, limit, offset int) ([]string, error) {
	if offset >= len(o.ids) {
		return nil, nil
	}
	end := offset + limit
	if end > len(o.ids) {
		end = len(o.ids)
	}
	return o.ids[offset:end], nil
}

type isoScopes struct{ identities []string }

func (s *isoScopes) ResolveWorkerOrganization(_ context.Context, orgID, identity string) (coretenant.Context, error) {
	s.identities = append(s.identities, identity)
	return coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "s-" + orgID,
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceWorker, DataPlacement: coretenant.DataPlacementShared,
	})
}

type isoFeatures struct{ denied map[string]bool }

func (f isoFeatures) RequireFeature(_ context.Context, orgID, key string) error {
	if key != "mikrotik.enabled" {
		return fmt.Errorf("unexpected feature %q", key)
	}
	if f.denied[orgID] {
		return errors.New("feature not entitled")
	}
	return nil
}

type isoReadiness struct{ ready, isolated map[string]bool }

func (r isoReadiness) HasReadyRouter(_ context.Context, scope coretenant.Scope) (bool, error) {
	return r.ready[scope.OrganizationID()], nil
}

func (r isoReadiness) HasIsolatedSecrets(_ context.Context, scope coretenant.Scope) (bool, error) {
	return r.isolated[scope.OrganizationID()], nil
}

type isoCalls struct {
	mu  sync.Mutex
	ids []string
}

func (c *isoCalls) add(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, id)
}

func (c *isoCalls) sorted() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string(nil), c.ids...)
	sort.Strings(out)
	return fmt.Sprint(out)
}

type isoReconciler struct{ calls isoCalls }

func (r *isoReconciler) RunOrganization(_ context.Context, scope coretenant.Scope) (mikrotikservice.ReconcileResult, error) {
	r.calls.add(scope.OrganizationID())
	return mikrotikservice.ReconcileResult{Accounts: 2}, nil
}

type isoProcessor struct {
	calls  isoCalls
	batchs []int
}

func (p *isoProcessor) RunOnce(_ context.Context, scope coretenant.Scope, batch int) (mikrotikservice.ProcessResult, error) {
	p.calls.add(scope.OrganizationID())
	p.batchs = append(p.batchs, batch)
	return mikrotikservice.ProcessResult{Done: 1}, nil
}

type isoEnv struct {
	runner *MikrotikIsolationRunner
	scopes *isoScopes
	rec    *isoReconciler
	proc   *isoProcessor
}

func newIsoEnv(mode string, orgs []string, feat isoFeatures, ready isoReadiness) isoEnv {
	e := isoEnv{scopes: &isoScopes{}, rec: &isoReconciler{}, proc: &isoProcessor{}}
	e.runner = newMikrotikIsolationRunner(mikrotikIsolationRunnerDeps{
		Mode: mode, Orgs: isoOrgs{ids: orgs}, Scopes: e.scopes, Features: feat, Readiness: ready,
		Reconciler: e.rec, Processor: e.proc,
	})
	return e
}

func TestRunner_ModeOff_ReturnsNil(t *testing.T) {
	for _, mode := range []string{"", "off", "bogus"} {
		r, err := NewMikrotikIsolationRunner(config.Config{Mikrotik: config.MikrotikConfig{IsolationMode: mode}}, nil, nil)
		if err != nil || r != nil {
			t.Fatalf("mode %q: runner=%v err=%v, want nil, nil", mode, r, err)
		}
	}
}

func TestRunner_SkipsOrgWithoutEntitlement(t *testing.T) {
	e := newIsoEnv(mikrotikservice.ModeOn, []string{"org-a", "org-b"},
		isoFeatures{denied: map[string]bool{"org-b": true}},
		isoReadiness{ready: map[string]bool{"org-a": true, "org-b": true}})
	res, err := e.runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := e.rec.calls.sorted(); got != "[org-a]" {
		t.Fatalf("reconciled %s, want [org-a]", got)
	}
	if got := e.proc.calls.sorted(); got != "[org-a]" {
		t.Fatalf("processed %s, want [org-a]", got)
	}
	if res.Organizations != 1 || res.Skipped != 1 {
		t.Fatalf("result %+v", res)
	}
	for _, id := range e.scopes.identities {
		if id != MikrotikIsolationWorkerIdentity {
			t.Fatalf("identity %q", id)
		}
	}
	if len(e.proc.batchs) != 1 || e.proc.batchs[0] != mikrotikIsolationBatch {
		t.Fatalf("batch %v", e.proc.batchs)
	}
}

func TestRunner_SkipsOrgWithoutReadyRouterAndNoIsolatedSecrets(t *testing.T) {
	e := newIsoEnv(mikrotikservice.ModeOn, []string{"org-a"}, isoFeatures{}, isoReadiness{})
	res, err := e.runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(e.rec.calls.ids)+len(e.proc.calls.ids) != 0 {
		t.Fatalf("reconciler=%v processor=%v, want none", e.rec.calls.ids, e.proc.calls.ids)
	}
	if res.Skipped != 1 || res.Organizations != 0 {
		t.Fatalf("result %+v", res)
	}
}

func TestRunner_ProcessesOrgWithIsolatedSecretsEvenIfRouterNotReady(t *testing.T) {
	e := newIsoEnv(mikrotikservice.ModeOn, []string{"org-a"}, isoFeatures{},
		isoReadiness{isolated: map[string]bool{"org-a": true}})
	res, err := e.runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.rec.calls.sorted() != "[org-a]" || e.proc.calls.sorted() != "[org-a]" {
		t.Fatalf("reconciler=%v processor=%v, want org-a in both", e.rec.calls.ids, e.proc.calls.ids)
	}
	if res.Organizations != 1 || res.Accounts != 2 || res.Tasks.Done != 1 {
		t.Fatalf("result %+v", res)
	}
}

func TestRunner_DryRun_RunsReconcilerButNotProcessor(t *testing.T) {
	e := newIsoEnv(mikrotikservice.ModeDryRun, []string{"org-a"}, isoFeatures{},
		isoReadiness{ready: map[string]bool{"org-a": true}})
	if _, err := e.runner.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.rec.calls.sorted() != "[org-a]" {
		t.Fatalf("reconciler=%v, want org-a", e.rec.calls.ids)
	}
	if len(e.proc.calls.ids) != 0 {
		t.Fatalf("dry_run must not touch routers: processor ran for %v", e.proc.calls.ids)
	}
}

func TestRunner_PagesThroughOrganizations(t *testing.T) {
	var orgs []string
	ready := map[string]bool{}
	for i := 0; i < mikrotikIsolationOrgPage+5; i++ {
		id := fmt.Sprintf("org-%03d", i)
		orgs = append(orgs, id)
		ready[id] = true
	}
	e := newIsoEnv(mikrotikservice.ModeOn, orgs, isoFeatures{}, isoReadiness{ready: ready})
	res, err := e.runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Organizations != len(orgs) {
		t.Fatalf("organizations %d, want %d", res.Organizations, len(orgs))
	}
}

// fakeInvoiceChanges records forwarded invoice events and the ctx it got.
type fakeInvoiceChanges struct {
	mu        sync.Mutex
	events    []string
	cancelled []bool
	deadline  []bool
}

func (f *fakeInvoiceChanges) OnInvoiceChanged(ctx context.Context, _ coretenant.Scope, invoiceID, event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, invoiceID+":"+event)
	f.cancelled = append(f.cancelled, ctx.Err() != nil)
	_, ok := ctx.Deadline()
	f.deadline = append(f.deadline, ok)
}

func TestListenerRegistration_OffMode_NotRegistered(t *testing.T) {
	for _, mode := range []string{"", mikrotikservice.ModeOff, "bogus"} {
		reg := receivableservice.NewRegistry()
		h := &fakeInvoiceChanges{}
		if registerMikrotikInvoiceListener(mode, reg, h) {
			t.Fatalf("mode %q: reported registered", mode)
		}
		reg.InvoicePaid(context.Background(), mkScope("org-1"), receivableservice.InvoiceRef{ID: "inv-1"})
		reg.InvoicePaidReplay(context.Background(), mkScope("org-1"), receivableservice.InvoiceRef{ID: "inv-1"})
		reg.InvoiceVoided(context.Background(), mkScope("org-1"), receivableservice.InvoiceRef{ID: "inv-1"})
		if len(h.events) != 0 {
			t.Fatalf("mode %q: listener received %v", mode, h.events)
		}
	}
	for _, mode := range []string{mikrotikservice.ModeDryRun, mikrotikservice.ModeOn} {
		reg := receivableservice.NewRegistry()
		h := &fakeInvoiceChanges{}
		if !registerMikrotikInvoiceListener(mode, reg, h) {
			t.Fatalf("mode %q: not registered", mode)
		}
		reg.InvoicePaid(context.Background(), mkScope("org-1"), receivableservice.InvoiceRef{ID: "inv-1"})
		if len(h.events) != 1 {
			t.Fatalf("mode %q: events %v", mode, h.events)
		}
	}
}

func TestListenerAdapter_ForwardsPaidReplayAndVoided(t *testing.T) {
	h := &fakeInvoiceChanges{}
	l := mikrotikReceivableListener{changes: h, timeout: time.Second}
	var _ receivableservice.Listener = l
	var _ receivableservice.ReplayableListener = l
	var _ receivableservice.VoidedListener = l

	// The request ctx is already cancelled: the evaluation must still run.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scope := mkScope("org-1")
	l.InvoicePaid(ctx, scope, receivableservice.InvoiceRef{ID: "inv-1"})
	l.InvoicePaidReplay(ctx, scope, receivableservice.InvoiceRef{ID: "inv-2"})
	l.InvoiceVoided(ctx, scope, receivableservice.InvoiceRef{ID: "inv-3"})
	l.ContractCreated(ctx, scope, receivableservice.ContractRef{ID: "c-1"})
	l.ContractEnded(ctx, scope, receivableservice.ContractRef{ID: "c-1"})

	if got := fmt.Sprint(h.events); got != "[inv-1:invoice_paid inv-2:invoice_paid_replay inv-3:invoice_voided]" {
		t.Fatalf("events %s", got)
	}
	for i := range h.events {
		if h.cancelled[i] || !h.deadline[i] {
			t.Fatalf("event %d: cancelled=%v deadline=%v, want detached ctx with timeout", i, h.cancelled[i], h.deadline[i])
		}
	}
}
