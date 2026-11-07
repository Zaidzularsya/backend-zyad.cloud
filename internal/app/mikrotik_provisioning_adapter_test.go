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

func newProvAdapter(st *fakeStager, gate fakeProvGate, now time.Time) mikrotikProvisioningAdapter {
	return newMikrotikProvisioningAdapter(st, gate, func() time.Time { return now })
}

func TestAdapter_NonPPPoEAccountNotApplicable(t *testing.T) {
	st := &fakeStager{applicable: false}
	a := newProvAdapter(st, fakeProvGate{enabled: true}, time.Now())
	stage, ok, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || ok || stage != "" {
		t.Fatalf("stage=%q ok=%v err=%v", stage, ok, err)
	}
}

func TestAdapter_GateDisabledNotApplicableWithoutCallingService(t *testing.T) {
	st := &fakeStager{stage: service.StageActive, applicable: true}
	a := newProvAdapter(st, fakeProvGate{enabled: false}, time.Now())
	_, ok, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || ok || st.calls != 0 {
		t.Fatalf("ok=%v err=%v calls=%d", ok, err, st.calls)
	}
}

func TestAdapter_NilPaidAtUsesNowAndBusinessToday(t *testing.T) {
	now := time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC) // 01:30 WIB 9 Nov
	st := &fakeStager{stage: service.StageReceived, applicable: true}
	a := newProvAdapter(st, fakeProvGate{enabled: true}, now)
	stage, ok, err := a.Stage(context.Background(), mkScope("o1"), "inv1", nil)
	if err != nil || !ok || stage != "received" {
		t.Fatalf("stage=%q ok=%v err=%v", stage, ok, err)
	}
	if !st.paidAt.Equal(now) || !st.now.Equal(now) || st.today.Format("2006-01-02") != "2026-11-09" {
		t.Fatalf("paidAt=%v now=%v today=%v", st.paidAt, st.now, st.today)
	}
	paid := now.Add(-time.Hour)
	if _, _, err := a.Stage(context.Background(), mkScope("o1"), "inv1", &paid); err != nil || !st.paidAt.Equal(paid) {
		t.Fatalf("paidAt=%v err=%v", st.paidAt, err)
	}
}

func TestAdapter_ErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	a := newProvAdapter(&fakeStager{err: boom}, fakeProvGate{enabled: true}, time.Now())
	if _, _, err := a.Stage(context.Background(), mkScope("o1"), "i", nil); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	a = newProvAdapter(&fakeStager{}, fakeProvGate{err: boom}, time.Now())
	if _, _, err := a.Stage(context.Background(), mkScope("o1"), "i", nil); !errors.Is(err, boom) {
		t.Fatalf("gate err=%v", err)
	}
}
