package app

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakeStager struct {
	stage      service.Stage
	applicable bool
	err        error
	calls      int
	paidAt     time.Time
	now, today time.Time
}

func (f *fakeStager) Stage(_ context.Context, _ coretenant.Scope, _ string, paidAt, now, today time.Time) (service.Stage, bool, error) {
	f.calls++
	f.paidAt, f.now, f.today = paidAt, now, today
	return f.stage, f.applicable, f.err
}

type fakeProvGate struct {
	enabled bool
	err     error
}

func (g fakeProvGate) Enabled(context.Context, coretenant.Scope) (bool, error) {
	return g.enabled, g.err
}

type fakeProvProfile struct {
	c   service.TenantContact
	err error
}

func (p fakeProvProfile) Profile(context.Context, coretenant.Scope) (service.TenantContact, error) {
	return p.c, p.err
}

func newProvAdapter(st *fakeStager, gate fakeProvGate, now time.Time) mikrotikProvisioningAdapter {
	return newProvAdapterWithProfile(st, gate, fakeProvProfile{}, now)
}

func newProvAdapterWithProfile(st *fakeStager, gate fakeProvGate, prof fakeProvProfile, now time.Time) mikrotikProvisioningAdapter {
	return newMikrotikProvisioningAdapter(st, gate, prof, func() time.Time { return now })
}

func TestAdapter_NonPPPoEAccountNotApplicable(t *testing.T) {
	st := &fakeStager{applicable: false}
	a := newProvAdapter(st, fakeProvGate{enabled: true}, time.Now())
	res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || res.Applicable || res.Stage != "" || res.Contact != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestAdapter_GateDisabledNotApplicableWithoutCallingService(t *testing.T) {
	st := &fakeStager{stage: service.StageActive, applicable: true}
	a := newProvAdapter(st, fakeProvGate{enabled: false}, time.Now())
	res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || res.Applicable || st.calls != 0 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, st.calls)
	}
}

func TestAdapter_NilPaidAtUsesNowAndBusinessToday(t *testing.T) {
	now := time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC) // 01:30 WIB 9 Nov
	st := &fakeStager{stage: service.StageReceived, applicable: true}
	a := newProvAdapter(st, fakeProvGate{enabled: true}, now)
	res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || !res.Applicable || res.Stage != "received" || res.Contact != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !st.paidAt.Equal(now) || !st.now.Equal(now) || st.today.Format("2006-01-02") != "2026-11-09" {
		t.Fatalf("paidAt=%v now=%v today=%v", st.paidAt, st.now, st.today)
	}
	paid := now.Add(-time.Hour)
	if _, err := a.Stage(context.Background(), mkScope("o1"), "inv1", &paid); err != nil || !st.paidAt.Equal(paid) {
		t.Fatalf("paidAt=%v err=%v", st.paidAt, err)
	}
}

func TestAdapter_ErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	a := newProvAdapter(&fakeStager{err: boom}, fakeProvGate{enabled: true}, time.Now())
	if _, err := a.Stage(context.Background(), mkScope("o1"), "i", nil); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	a = newProvAdapter(&fakeStager{}, fakeProvGate{err: boom}, time.Now())
	if _, err := a.Stage(context.Background(), mkScope("o1"), "i", nil); !errors.Is(err, boom) {
		t.Fatalf("gate err=%v", err)
	}
}

func TestAdapter_NeedsHelpCarriesTenantContact(t *testing.T) {
	st := &fakeStager{stage: service.StageNeedsHelp, applicable: true}
	prof := fakeProvProfile{c: service.TenantContact{Name: "ISP Maju", Phone: "0811", Email: "cs@isp.id"}}
	a := newProvAdapterWithProfile(st, fakeProvGate{enabled: true}, prof, time.Now())
	res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || res.Stage != "needs_help" || !res.Applicable || res.Contact == nil ||
		res.Contact.Name != "ISP Maju" || res.Contact.Phone != "0811" || res.Contact.Email != "cs@isp.id" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestAdapter_OtherStagesHaveNoContact(t *testing.T) {
	prof := fakeProvProfile{c: service.TenantContact{Name: "ISP"}}
	for _, stg := range []service.Stage{service.StageReceived, service.StageRestoring, service.StageActive, service.StageOtherInvoices} {
		a := newProvAdapterWithProfile(&fakeStager{stage: stg, applicable: true}, fakeProvGate{enabled: true}, prof, time.Now())
		res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
		if err != nil || res.Contact != nil {
			t.Fatalf("%s: res=%+v err=%v", stg, res, err)
		}
	}
}

func TestAdapter_ProfileErrorKeepsNeedsHelpWithoutContact(t *testing.T) {
	st := &fakeStager{stage: service.StageNeedsHelp, applicable: true}
	a := newProvAdapterWithProfile(st, fakeProvGate{enabled: true}, fakeProvProfile{err: errors.New("db down")}, time.Now())
	res, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || res.Stage != "needs_help" || !res.Applicable || res.Contact != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
