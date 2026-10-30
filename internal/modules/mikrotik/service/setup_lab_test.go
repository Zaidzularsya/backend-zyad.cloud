//go:build mikrotiklab

package service

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Lab test for SetupService against a real RouterOS router (binary API).
//
//	set -a; . ~/.config/zyad-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run TestLabSetupLifecycle -v ./internal/modules/mikrotik/service/
//
// Credentials come only from MIKROTIK_LAB_HOST/USER/PASS and are never logged.
// It only creates and removes zyad-managed objects, and refuses to run unless a
// pre-zyad-isolir*.backup file exists on the router.

type countingClient struct {
	pmk.RouterClient
	writes atomic.Int64
}

func (c *countingClient) Add(ctx context.Context, p string, a pmk.Row) (string, error) {
	c.writes.Add(1)
	return c.RouterClient.Add(ctx, p, a)
}
func (c *countingClient) Set(ctx context.Context, p, id string, a pmk.Row) error {
	c.writes.Add(1)
	return c.RouterClient.Set(ctx, p, id, a)
}
func (c *countingClient) Remove(ctx context.Context, p, id string) error {
	c.writes.Add(1)
	return c.RouterClient.Remove(ctx, p, id)
}

type labFactory struct{ client pmk.RouterClient }

func (f labFactory) Detect(context.Context, pmk.Endpoint) (string, string, error) {
	return pmk.APITypeBinary, "lab", nil
}
func (f labFactory) New(string, pmk.Endpoint) (pmk.RouterClient, error) { return f.client, nil }

func labZyadObjects(t *testing.T, c pmk.RouterClient) []string {
	t.Helper()
	ctx := context.Background()
	var found []string
	for _, p := range []string{pathPool, pathProfile, pathList, pathFilter, pathNAT, pathProxy} {
		rows, err := c.Print(ctx, p, nil)
		if err != nil {
			t.Fatalf("print %s: %v", p, err)
		}
		for _, r := range rows {
			if r["comment"] == setupManaged || strings.HasPrefix(r["name"], "zyad-") || strings.HasPrefix(r["list"], "zyad-") {
				found = append(found, p)
			}
		}
	}
	return found
}

func TestLabSetupLifecycle(t *testing.T) {
	host, user, pass := os.Getenv("MIKROTIK_LAB_HOST"), os.Getenv("MIKROTIK_LAB_USER"), os.Getenv("MIKROTIK_LAB_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("MIKROTIK_LAB_HOST/USER/PASS not set")
	}
	if os.Getenv("MIKROTIK_LAB_WRITE") != "1" {
		t.Skip("set MIKROTIK_LAB_WRITE=1 to run write tests")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatal("MIKROTIK_LAB_HOST must be an IP address")
	}
	allowed := []netip.Prefix{netip.PrefixFrom(addr, addr.BitLen())}
	port := 8728
	if p := os.Getenv("MIKROTIK_LAB_PORT"); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			t.Fatal("bad MIKROTIK_LAB_PORT")
		}
	}
	raw, err := pmk.NewClient(pmk.APITypeBinary, pmk.Endpoint{
		Host: host, Port: port, Username: user, Password: pass, AllowedPrivate: allowed, Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	files, err := raw.Print(ctx, "/file", nil)
	if err != nil {
		t.Fatalf("print /file: %v", err)
	}
	backup := false
	for _, f := range files {
		if strings.HasPrefix(f["name"], "pre-zyad-isolir") {
			backup = true
		}
	}
	if !backup {
		t.Skip("no pre-zyad-isolir* backup file on the router")
	}
	if pre := labZyadObjects(t, raw); len(pre) != 0 {
		t.Skipf("router already has %d zyad objects; refusing to run", len(pre))
	}

	counter := &countingClient{RouterClient: raw}
	rs := NewRouterService(newFakeRepo(), labFactory{counter}, testSecret, allowed)
	repo := rs.repo.(*fakeRepo)
	router, err := rs.Register(ctx, testScope(t), RegisterInput{Name: "lab", Host: host, Port: port, Username: user, Password: pass, ActorUserID: testActor})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := NewSetupService(rs, repo)
	t.Cleanup(func() {
		// Safety net: removes only zyad-managed objects, idempotent.
		if _, err := svc.Cleanup(ctx, testScope(t), router.ID, testActor); err != nil {
			t.Logf("safety cleanup: %v", err)
		}
	})

	in := SetupInput{
		PlatformHost: "zyad.online", TenantIsolirHost: "isolir-lab.zyad.online", DokuHosts: []string{"sandbox.doku.com"},
		RateLimit: "512k/512k",
	}
	var plan domain.SetupPlan
	picked := ""
	for _, cidr := range []string{"10.255.254.0/24", "10.255.250.0/24", "10.255.249.0/24", "172.31.254.0/24"} {
		in.PoolCIDR = cidr
		plan, err = svc.Preview(ctx, testScope(t), router.ID, in)
		if errors.Is(err, mikrotik.ErrInvalidInput) {
			t.Logf("pool %s rejected: %v", cidr, err)
			continue
		}
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		picked = cidr
		break
	}
	if picked == "" {
		t.Skip("no free isolir range found on the router")
	}
	t.Logf("preview with free range %s: %d actions", picked, len(plan.Actions))
	if counter.writes.Load() != 0 {
		t.Fatal("preview wrote")
	}
	for _, a := range plan.Actions {
		if a.Kind != domain.SetupCreate {
			t.Fatalf("fresh router should only create, got %s %s", a.Kind, a.Path)
		}
	}

	if _, err := svc.Apply(ctx, testScope(t), router.ID, in, plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Logf("apply wrote %d objects", counter.writes.Load())
	if int(counter.writes.Load()) != len(plan.Actions) {
		t.Fatalf("writes = %d, want %d", counter.writes.Load(), len(plan.Actions))
	}
	if !repo.routers[router.ID].IsolirReady {
		t.Fatal("router not marked ready")
	}

	// Verify with direct reads.
	pools, _ := raw.Print(ctx, pathPool, pmk.Row{"name": setupPoolName})
	profiles, _ := raw.Print(ctx, pathProfile, pmk.Row{"name": setupDefaultProf})
	fwd, _ := raw.Print(ctx, pathFilter, pmk.Row{"chain": "forward"})
	managedFwd := 0
	for i, r := range fwd {
		if r["comment"] == setupManaged {
			if i != managedFwd {
				t.Errorf("zyad forward rule at position %d, expected contiguous from top", i)
			}
			managedFwd++
		}
	}
	if len(pools) != 1 || len(profiles) != 1 || managedFwd != 4 {
		t.Fatalf("pools=%d profiles=%d zyad forward rules=%d", len(pools), len(profiles), managedFwd)
	}

	// Second apply: all exists, zero writes.
	before := counter.writes.Load()
	again, err := svc.Preview(ctx, testScope(t), router.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range again.Actions {
		if a.Kind != domain.SetupExists {
			t.Fatalf("second preview: %s %s is %s", a.Kind, a.Path, a.Kind)
		}
	}
	if _, err := svc.Apply(ctx, testScope(t), router.ID, in, again.Hash, testActor); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if counter.writes.Load() != before {
		t.Fatalf("second apply wrote %d times", counter.writes.Load()-before)
	}

	if _, err := svc.Cleanup(ctx, testScope(t), router.ID, testActor); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if left := labZyadObjects(t, raw); len(left) != 0 {
		t.Fatalf("zyad objects left after cleanup: %v", left)
	}
	if repo.routers[router.ID].IsolirReady {
		t.Fatal("router still marked ready after cleanup")
	}
	t.Log("cleanup verified: no zyad objects remain")
}
