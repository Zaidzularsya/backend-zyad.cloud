package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

func seedStats(c *pmk.FakeClient) {
	c.Seed("/system/identity", pmk.Row{"name": "core-1"})
	c.Seed("/system/resource", pmk.Row{
		"board-name": "RB4011", "version": "7.15", "architecture-name": "arm", "uptime": "3d4h",
		"cpu-load": "12", "total-memory": "1073741824", "free-memory": "536870912",
	})
	c.Seed("/ppp/active", pmk.Row{"name": "a"}, pmk.Row{"name": "b"}, pmk.Row{"name": "c"})
}

func scopeForOrg(t *testing.T, orgID string) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "org", OrganizationType: coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive, ResolutionSource: coretenant.ResolutionSourceWorker,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestStatsReadsIdentityResourceAndActive(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	seedStats(factory.client)
	got, err := svc.Stats(context.Background(), testScope(t), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity != "core-1" || got.BoardName != "RB4011" || got.Version != "7.15" || got.Architecture != "arm" || got.Uptime != "3d4h" {
		t.Fatalf("stats = %+v", got)
	}
	if got.CPULoad == nil || *got.CPULoad != 12 {
		t.Fatalf("cpu = %v", got.CPULoad)
	}
	if got.MemoryTotal == nil || *got.MemoryTotal != 1073741824 || got.MemoryFree == nil || *got.MemoryFree != 536870912 {
		t.Fatalf("memory = %v/%v", got.MemoryTotal, got.MemoryFree)
	}
	if got.PPPActive == nil || *got.PPPActive != 3 {
		t.Fatalf("ppp = %v", got.PPPActive)
	}
	if got.FetchedAt.IsZero() {
		t.Fatal("FetchedAt not set")
	}
}

func TestStatsMissingResourceLeavesFieldsEmpty(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	factory.client.Seed("/system/identity", pmk.Row{"name": "core-1"})
	got, err := svc.Stats(context.Background(), testScope(t), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity != "core-1" || got.CPULoad != nil || got.MemoryTotal != nil || got.MemoryFree != nil || got.BoardName != "" {
		t.Fatalf("stats = %+v", got)
	}
}

func TestStatsCachesPerOrganizationWithinTTL(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	seedStats(factory.client)
	now := time.Unix(1_000_000, 0)
	svc.now = func() time.Time { return now }
	scope := testScope(t)
	base := factory.newCalls

	if _, err := svc.Stats(context.Background(), scope, r.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Second)
	if _, err := svc.Stats(context.Background(), scope, r.ID); err != nil {
		t.Fatal(err)
	}
	if factory.newCalls-base != 1 {
		t.Fatalf("New calls = %d, want 1 within TTL", factory.newCalls-base)
	}
	now = now.Add(2 * time.Second) // 61s after first fetch
	if _, err := svc.Stats(context.Background(), scope, r.ID); err != nil {
		t.Fatal(err)
	}
	if factory.newCalls-base != 2 {
		t.Fatalf("New calls = %d, want 2 after TTL", factory.newCalls-base)
	}

	// Same router id, other organization: must not reuse the cache entry.
	other := scopeForOrg(t, "00000000-0000-0000-0000-000000000999")
	if _, err := svc.Stats(context.Background(), other, r.ID); err != nil {
		t.Fatal(err)
	}
	if factory.newCalls-base != 3 {
		t.Fatalf("New calls = %d, want 3 (cache keyed by organization)", factory.newCalls-base)
	}
}

func TestStatsDoesNotCacheFailure(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	seedStats(factory.client)
	factory.newFailures = 1
	if _, err := svc.Stats(context.Background(), testScope(t), r.ID); !errors.Is(err, mikrotik.ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if _, err := svc.Stats(context.Background(), testScope(t), r.ID); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestStatsFirstCommandFailureIsTranslatedAndNotCached(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	factory.client.Err = pmk.ErrUnreachable
	if _, err := svc.Stats(context.Background(), testScope(t), r.ID); !errors.Is(err, mikrotik.ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	factory.client.Err = nil
	seedStats(factory.client)
	if got, err := svc.Stats(context.Background(), testScope(t), r.ID); err != nil || got.Identity != "core-1" {
		t.Fatalf("after recovery: %+v %v", got, err)
	}
}

func TestStatsDoesNotRecordCheck(t *testing.T) {
	svc, repo, factory := newTestService(t)
	r := registerOne(t, svc)
	seedStats(factory.client)
	before := len(repo.checks)
	if _, err := svc.Stats(context.Background(), testScope(t), r.ID); err != nil {
		t.Fatal(err)
	}
	if len(repo.checks) != before {
		t.Fatalf("RecordCheck calls = %d, want 0", len(repo.checks)-before)
	}
}

func TestStatsUnknownRouterIsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Stats(context.Background(), testScope(t), "99999999-9999-9999-9999-999999999999")
	if !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("err = %v", err)
	}
}
