package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const (
	testOrg  = "11111111-1111-1111-1111-111111111111"
	testUser = "22222222-2222-2222-2222-222222222222"
)

func TestGrantWindow_UTC(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, 11, 11, 14, 6, 0, 0, wib) // 07:06 UTC
	from, until := grantWindow(now, 90)
	if from.Location() != time.UTC || until.Location() != time.UTC {
		t.Fatalf("locations = %v / %v, want UTC", from.Location(), until.Location())
	}
	if from.After(now.UTC()) {
		t.Fatalf("from %v is in the future relative to %v", from, now.UTC())
	}
	if want := time.Date(2027, 2, 9, 7, 6, 0, 0, time.UTC); !until.Equal(want) {
		t.Fatalf("until = %v, want %v", until, want)
	}
}

type fakeRepo struct {
	calls  int
	params repository.UpsertEntitlementParams
}

func (f *fakeRepo) Upsert(_ context.Context, p repository.UpsertEntitlementParams) (model.Entitlement, error) {
	f.calls++
	f.params = p
	return model.Entitlement{}, nil
}

func TestGrant_RequiresReasonAndBy(t *testing.T) {
	base := grantInput{OrganizationID: testOrg, FeatureKey: "mikrotik.enabled", Days: 30, Reason: "pilot", ActorUserID: testUser}
	for name, mutate := range map[string]func(*grantInput){
		"no reason":     func(in *grantInput) { in.Reason = "  " },
		"no by":         func(in *grantInput) { in.ActorUserID = "" },
		"by not a uuid": func(in *grantInput) { in.ActorUserID = "admin" },
		"bad org":       func(in *grantInput) { in.OrganizationID = "ethernet" },
		"no feature":    func(in *grantInput) { in.FeatureKey = "" },
		"zero days":     func(in *grantInput) { in.Days = 0 },
		"too many days": func(in *grantInput) { in.Days = maxDays + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeRepo{}
			in := base
			mutate(&in)
			if _, err := grant(context.Background(), f, in, time.Now()); err == nil {
				t.Fatal("want validation error")
			}
			if f.calls != 0 {
				t.Fatal("must not touch the database on invalid input")
			}
		})
	}
}

func TestGrant_WritesPlatformOverrideWithAuditFields(t *testing.T) {
	f := &fakeRepo{}
	in := grantInput{OrganizationID: testOrg, FeatureKey: "mikrotik.enabled", Days: 30, Reason: " pilot ", ActorUserID: testUser}
	if _, err := grant(context.Background(), f, in, time.Now()); err != nil {
		t.Fatal(err)
	}
	p := f.params
	if p.Source != model.EntitlementSourcePlatformOverride || p.EffectiveUntil == nil || p.ActorUserID != testUser || strings.TrimSpace(p.Reason) != "pilot" {
		t.Fatalf("params = %+v", p)
	}
	if p.EffectiveFrom.Location() != time.UTC || p.EffectiveUntil.Location() != time.UTC {
		t.Fatal("window must be UTC")
	}
}
