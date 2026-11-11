package service

import (
	"context"
	"testing"

	pmk "zyad.cloud/internal/platform/mikrotik"
)

func seedAllow(f *setupFixture, names []string, dynamic []pmk.Row) {
	for _, n := range names {
		f.client.Seed(pathAddressList, pmk.Row{"list": setupAllowList, "address": n, "comment": setupManaged})
	}
	for _, d := range dynamic {
		d["list"], d["dynamic"] = setupAllowList, "true"
		f.client.Seed(pathAddressList, d)
	}
}

func TestAllowlistHealth_CountsUnresolved(t *testing.T) {
	f := newSetupFixture(t)
	seedAllow(f, []string{"a.example.com", "b.example.com", "c.example.com", "d.example.com"}, []pmk.Row{
		{"address": "1.1.1.1", "comment": "a.example.com"},
		{"address": "1.1.1.2", "comment": "b.example.com"},
		{"address": "1.1.1.3", "comment": "c.example.com"},
	})
	got, err := f.svc.AllowlistHealth(context.Background(), testScope(t), f.routerID)
	if err != nil {
		t.Fatal(err)
	}
	if got != (AllowlistHealth{Total: 4, Unresolved: 1}) {
		t.Fatalf("health = %+v", got)
	}
}

func TestAllowlistHealth_AllResolved(t *testing.T) {
	f := newSetupFixture(t)
	seedAllow(f, []string{"a.example.com", "b.example.com"}, []pmk.Row{
		{"address": "1.1.1.1", "comment": "a.example.com"}, {"address": "1.1.1.2", "comment": "b.example.com"},
	})
	got, _ := f.svc.AllowlistHealth(context.Background(), testScope(t), f.routerID)
	if got != (AllowlistHealth{Total: 2}) {
		t.Fatalf("health = %+v", got)
	}
}

func TestAllowlistHealth_NoDynamicRowsMeansDNSDown(t *testing.T) {
	f := newSetupFixture(t)
	seedAllow(f, []string{"a.example.com", "b.example.com"}, nil)
	got, _ := f.svc.AllowlistHealth(context.Background(), testScope(t), f.routerID)
	if got != (AllowlistHealth{Total: 2, Unresolved: 2}) {
		t.Fatalf("health = %+v", got)
	}
}

func TestAllowlistHealth_UnattributedDynamicRowsAreNotAFalseAlarm(t *testing.T) {
	f := newSetupFixture(t)
	seedAllow(f, []string{"a.example.com", "b.example.com"}, []pmk.Row{{"address": "1.1.1.1"}})
	got, _ := f.svc.AllowlistHealth(context.Background(), testScope(t), f.routerID)
	if got.Unresolved != 0 {
		t.Fatalf("health = %+v", got)
	}
}

func TestAllowlistHealth_NeverWrites(t *testing.T) {
	f := newSetupFixture(t)
	seedAllow(f, []string{"a.example.com"}, nil)
	_, _ = f.svc.AllowlistHealth(context.Background(), testScope(t), f.routerID)
	if f.client.WriteCalls() != 0 {
		t.Fatal("wrote")
	}
}
