//go:build mikrotiklab

package service

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Read-only lab test for RouterService.Stats against a real RouterOS router.
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	go test -tags mikrotiklab -run TestStatsLab -v -count=1 ./internal/modules/mikrotik/service/
//
// Binary API by default (MIKROTIK_LAB_PORT, default 8728). For REST set
// MIKROTIK_LAB_API=rest and MIKROTIK_LAB_PORT (80, or 443 with
// MIKROTIK_LAB_TLS=1). Credentials come only from MIKROTIK_LAB_HOST/USER/PASS
// and are never logged. Nothing is written to the router.

type statsLabFactory struct {
	apiType string
	client  pmk.RouterClient
}

func (f statsLabFactory) Detect(context.Context, pmk.Endpoint) (string, string, error) {
	return f.apiType, "lab", nil
}
func (f statsLabFactory) New(string, pmk.Endpoint) (pmk.RouterClient, error) { return f.client, nil }

func TestStatsLab(t *testing.T) {
	host, user, pass := os.Getenv("MIKROTIK_LAB_HOST"), os.Getenv("MIKROTIK_LAB_USER"), os.Getenv("MIKROTIK_LAB_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("MIKROTIK_LAB_HOST/USER/PASS not set")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatal("MIKROTIK_LAB_HOST must be an IP address")
	}
	allowed := []netip.Prefix{netip.PrefixFrom(addr, addr.BitLen())}

	apiType, port := pmk.APITypeBinary, 8728
	if os.Getenv("MIKROTIK_LAB_API") == "rest" {
		apiType, port = pmk.APITypeREST, 80
	}
	if p := os.Getenv("MIKROTIK_LAB_PORT"); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			t.Fatal("bad MIKROTIK_LAB_PORT")
		}
	}
	useTLS := os.Getenv("MIKROTIK_LAB_TLS") == "1"
	raw, err := pmk.NewClient(apiType, pmk.Endpoint{
		Host: host, Port: port, UseTLS: useTLS, Username: user, Password: pass, AllowedPrivate: allowed, Timeout: 8 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	rs := NewRouterService(newFakeRepo(), statsLabFactory{apiType: apiType, client: raw}, testSecret, allowed)
	ctx := context.Background()
	router, err := rs.Register(ctx, testScope(t), RegisterInput{Name: "lab-stats", Host: host, Port: port, UseTLS: useTLS, Username: user, Password: pass, ActorUserID: testActor})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	stats, err := rs.Stats(ctx, testScope(t), router.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Identity == "" || stats.Version == "" || stats.FetchedAt.IsZero() {
		t.Fatalf("identity/version/fetched_at not filled: %+v", stats)
	}
	if stats.CPULoad == nil || stats.MemoryTotal == nil || stats.MemoryFree == nil {
		t.Fatalf("numeric resource fields did not parse (api=%s): cpu=%v total=%v free=%v", apiType, stats.CPULoad, stats.MemoryTotal, stats.MemoryFree)
	}
	if *stats.MemoryTotal <= 0 || *stats.MemoryFree < 0 || *stats.MemoryFree > *stats.MemoryTotal {
		t.Fatalf("implausible memory: total=%d free=%d", *stats.MemoryTotal, *stats.MemoryFree)
	}
	if stats.PPPActive == nil {
		t.Fatal("ppp_active not read")
	}
	t.Logf("api=%s identity=%q version=%s board=%s arch=%s uptime=%s cpu=%d%% mem=%d/%d ppp_active=%d",
		apiType, stats.Identity, stats.Version, stats.BoardName, stats.Architecture, stats.Uptime,
		*stats.CPULoad, *stats.MemoryFree, *stats.MemoryTotal, *stats.PPPActive)

	again, err := rs.Stats(ctx, testScope(t), router.ID)
	if err != nil || !again.FetchedAt.Equal(stats.FetchedAt) {
		t.Fatalf("second call should be served from cache: err=%v", err)
	}
}
