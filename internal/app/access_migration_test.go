package app

import (
	"context"
	"errors"
	"testing"
	"time"

	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type fakeAccessStore struct {
	// active[org][source] = ada baris aktif
	active map[string]map[organizationmodel.EntitlementSource]bool
	// expireCalls mencatat source yang di-expire; hanya `plan` yang boleh muncul.
	expireCalls []organizationmodel.EntitlementSource
	provisioned []string
}

func newFakeAccessStore() *fakeAccessStore {
	return &fakeAccessStore{active: map[string]map[organizationmodel.EntitlementSource]bool{}}
}

func (f *fakeAccessStore) set(org string, sources ...organizationmodel.EntitlementSource) {
	f.active[org] = map[organizationmodel.EntitlementSource]bool{}
	for _, s := range sources {
		f.active[org][s] = true
	}
}

func (f *fakeAccessStore) HasActiveSource(_ context.Context, org string, source organizationmodel.EntitlementSource, _ time.Time) (bool, error) {
	return f.active[org][source], nil
}

func (f *fakeAccessStore) ExpireBySource(_ context.Context, org string, source organizationmodel.EntitlementSource, _ *string, _ time.Time, _ string, reason string) (int64, error) {
	f.expireCalls = append(f.expireCalls, source)
	if reason != planMigrationReason {
		return 0, errors.New("unexpected reason " + reason)
	}
	if f.active[org][source] {
		delete(f.active[org], source)
		return 1, nil
	}
	return 0, nil
}

// ProvisionDefaultAccess meniru provisioner nyata: menambah baris default aktif.
func (f *fakeAccessStore) ProvisionDefaultAccess(_ context.Context, org string) error {
	f.provisioned = append(f.provisioned, org)
	if f.active[org] == nil {
		f.active[org] = map[organizationmodel.EntitlementSource]bool{}
	}
	f.active[org][organizationmodel.EntitlementSourceDefault] = true
	return nil
}

const (
	srcPlan     = organizationmodel.EntitlementSourcePlan
	srcContract = organizationmodel.EntitlementSourceContract
	srcDefault  = organizationmodel.EntitlementSourceDefault
	srcOverride = organizationmodel.EntitlementSourcePlatformOverride
)

func TestBackfillGrantsDefaultAndExpiresPlan(t *testing.T) {
	store := newFakeAccessStore()
	store.set("old-free", srcPlan)

	res, err := backfillDefault(context.Background(), []string{"old-free"}, store, store, "bot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res != (BackfillResult{Granted: 1, ExpiredPlanRows: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if store.active["old-free"][srcPlan] || !store.active["old-free"][srcDefault] {
		t.Fatalf("state = %+v", store.active["old-free"])
	}
}

// Review Focus 2: workspace self-serve (contract aktif) tidak diberi default dan contract tetap aktif.
func TestBackfillKeepsActiveContract(t *testing.T) {
	store := newFakeAccessStore()
	store.set("paid", srcContract, srcPlan)

	res, err := backfillDefault(context.Background(), []string{"paid"}, store, store, "bot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res != (BackfillResult{SkippedContract: 1, ExpiredPlanRows: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if len(store.provisioned) != 0 || store.active["paid"][srcDefault] {
		t.Fatal("default must not be granted when contract is active")
	}
	if !store.active["paid"][srcContract] {
		t.Fatal("contract must stay active")
	}
}

// Review Focus 3: override admin tetap berlaku; hanya `plan` yang di-expire.
func TestBackfillLeavesPlatformOverride(t *testing.T) {
	store := newFakeAccessStore()
	store.set("vip", srcOverride, srcPlan)

	res, err := backfillDefault(context.Background(), []string{"vip"}, store, store, "bot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res != (BackfillResult{Granted: 1, ExpiredPlanRows: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if !store.active["vip"][srcOverride] || store.active["vip"][srcPlan] || !store.active["vip"][srcDefault] {
		t.Fatalf("state = %+v", store.active["vip"])
	}
	for _, s := range store.expireCalls {
		if s != srcPlan {
			t.Fatalf("expire called for source %q, only plan allowed", s)
		}
	}
}

// Review Focus 4: dijalankan dua kali → run kedua tidak melakukan apa pun.
func TestBackfillIsIdempotent(t *testing.T) {
	store := newFakeAccessStore()
	store.set("a", srcPlan)
	store.set("b", srcContract)
	store.set("c")
	orgs := []string{"a", "b", "c"}

	first, err := backfillDefault(context.Background(), orgs, store, store, "bot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if first.Granted != 2 || first.SkippedContract != 1 {
		t.Fatalf("first = %+v", first)
	}
	provisionedAfterFirst := len(store.provisioned)

	second, err := backfillDefault(context.Background(), orgs, store, store, "bot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if second.Granted != 0 || second.ExpiredPlanRows != 0 || len(store.provisioned) != provisionedAfterFirst {
		t.Fatalf("second = %+v, provisioned %d→%d", second, provisionedAfterFirst, len(store.provisioned))
	}
}

// Terputus di tengah (provisioning gagal) → workspace tidak kehilangan baris plan-nya.
func TestBackfillProvisionFailureKeepsPlanRows(t *testing.T) {
	store := newFakeAccessStore()
	store.set("a", srcPlan)
	failing := failingProvisioner{err: errors.New("default product missing")}

	_, err := backfillDefault(context.Background(), []string{"a"}, failing, store, "bot", time.Now())
	if err == nil {
		t.Fatal("want error")
	}
	if !store.active["a"][srcPlan] {
		t.Fatal("plan row must stay active when provisioning fails")
	}
}

type failingProvisioner struct{ err error }

func (f failingProvisioner) ProvisionDefaultAccess(context.Context, string) error { return f.err }

func TestPreflightReportClean(t *testing.T) {
	if !(PreflightReport{WorkspacesNeedingBackfill: 3}).Clean() {
		t.Fatal("workspaces needing backfill must not make the report dirty")
	}
	if (PreflightReport{PaidActiveSubscriptions: 1}).Clean() || (PreflightReport{OpenLegacyInvoices: 1}).Clean() {
		t.Fatal("paid subscription / open invoice must make the report dirty")
	}
}
