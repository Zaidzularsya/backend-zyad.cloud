//go:build integration

package app

import "testing"

func TestAnyTriggerProduct_OnlyEnabledTriggersOfTheOrg(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	sa, sb := scopeOf(t, f, a), scopeOf(t, f, b)
	f.trigger(t, a, f.prodP, true, f.today)
	f.trigger(t, a, f.prodQ, false, f.today)

	cases := []struct {
		name string
		ids  []string
		want bool
	}{
		{"enabled product", []string{f.prodP}, true},
		{"disabled product", []string{f.prodQ}, false},
		{"mixed", []string{f.prodQ, f.prodP}, true},
		{"empty", []string{}, false},
	}
	for _, c := range cases {
		got, err := f.reader.AnyTriggerProduct(f.ctx, sa, c.ids)
		if err != nil || got != c.want {
			t.Fatalf("%s: got %v err=%v, want %v", c.name, got, err, c.want)
		}
	}
	if got, err := f.reader.AnyTriggerProduct(f.ctx, sb, []string{f.prodP}); err != nil || got {
		t.Fatalf("org B sees org A trigger: %v %v", got, err)
	}
}
