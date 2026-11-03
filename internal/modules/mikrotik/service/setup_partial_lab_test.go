//go:build mikrotiklab

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Lab tests for runbook section 10 items 11 and 12 (docs/runbook-mikrotik-isolir-setup.md):
//
//	11. cleanup while a PPP secret still uses the isolir profile
//	12. setup is idempotent when only part of the objects exist
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run 'TestLabSetupPartial|TestLabSetupCleanupRefuses' -v -count=1 ./internal/modules/mikrotik/service/
//
// They only create and remove zyad-* objects (the same ones TestLabSetupLifecycle
// manages), refuse to run while the router already has zyad objects, and require a
// pre-zyad-isolir*.backup file. Credentials come from MIKROTIK_LAB_HOST/USER/PASS
// and are never logged.

type labSetupEnv struct {
	raw     pmk.RouterClient
	counter *countingClient
	svc     *SetupService
	repo    *fakeRepo
	router  domain.Router
	in      SetupInput
}

func newLabSetupEnv(t *testing.T) *labSetupEnv {
	t.Helper()
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
	svc := NewSetupService(rs, repo, &fakeLocker{}, SetupOptions{})
	t.Cleanup(func() {
		// Safety net: removes only zyad-managed objects, idempotent.
		if _, err := svc.Cleanup(ctx, testScope(t), router.ID, testActor); err != nil {
			t.Logf("safety cleanup: %v", err)
		}
	})
	return &labSetupEnv{
		raw: raw, counter: counter, svc: svc, repo: repo, router: router,
		in: SetupInput{
			PlatformHost: "zyad.online", TenantIsolirHost: "isolir-lab.zyad.online",
			DokuHosts: []string{"sandbox.doku.com"}, RateLimit: "512k/512k",
		},
	}
}

// applyFresh previews with the first isolir range the router accepts, then applies.
func (e *labSetupEnv) applyFresh(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	var plan domain.SetupPlan
	picked := false
	for _, cidr := range []string{"10.255.254.0/24", "10.255.250.0/24", "10.255.249.0/24", "172.31.254.0/24"} {
		e.in.PoolCIDR = cidr
		p, err := e.svc.Preview(ctx, testScope(t), e.router.ID, e.in)
		if errors.Is(err, mikrotik.ErrInvalidInput) {
			continue
		}
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		plan, picked = p, true
		break
	}
	if !picked {
		t.Skip("no free isolir range found on the router")
	}
	if _, err := e.svc.Apply(ctx, testScope(t), e.router.ID, e.in, plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

// forwardSignature lists the zyad-managed forward rules in router order as
// "action|protocol|dst-port|dst-address-list" and reports whether they sit
// contiguously at the top of the static forward rules (dynamic rows ignored).
func forwardSignature(t *testing.T, c pmk.RouterClient) (sig []string, contiguousTop bool) {
	t.Helper()
	rows, err := c.Print(context.Background(), pathFirewallFilter, pmk.Row{"chain": "forward"})
	if err != nil {
		t.Fatalf("print filter: %v", err)
	}
	contiguousTop = true
	static := 0
	for _, r := range rows {
		if rowDynamic(r) {
			continue
		}
		if r["comment"] == setupManaged {
			if static != len(sig) {
				contiguousTop = false
			}
			sig = append(sig, strings.Join([]string{r["action"], r["protocol"], r["dst-port"], r["dst-address-list"]}, "|"))
		}
		static++
	}
	return sig, contiguousTop
}

// removeManaged deletes the zyad-managed rows of path that match, returning how many.
func removeManaged(t *testing.T, c pmk.RouterClient, path string, match func(pmk.Row) bool) int {
	t.Helper()
	ctx := context.Background()
	rows, err := c.Print(ctx, path, nil)
	if err != nil {
		t.Fatalf("print %s: %v", path, err)
	}
	n := 0
	for _, r := range rows {
		if r["comment"] != setupManaged || !match(r) {
			continue
		}
		if err := c.Remove(ctx, path, r[".id"]); err != nil {
			t.Fatalf("remove %s %s: %v", path, r[".id"], err)
		}
		n++
	}
	return n
}

// TestLabSetupPartialRepair removes part of a finished setup and checks that
// preview plans only the missing objects, apply writes only those, the four
// forward rules end up in the original order at the top, and a second preview
// is all "exists".
func TestLabSetupPartialRepair(t *testing.T) {
	env := newLabSetupEnv(t)
	ctx := context.Background()
	env.applyFresh(t)

	want, top := forwardSignature(t, env.raw)
	if len(want) != 4 || !top {
		t.Fatalf("fresh setup: forward signature %v contiguousTop=%v", want, top)
	}
	t.Logf("fresh forward order: %v", want)

	type damage struct {
		name   string
		remove func() int
	}
	cases := []damage{
		{"allowlist entry", func() int {
			return removeManaged(t, env.raw, pathAddressList, func(r pmk.Row) bool {
				return r["list"] == setupAllowList && r["address"] == "sandbox.doku.com"
			})
		}},
		{"second forward rule (dns tcp)", func() int {
			return removeManaged(t, env.raw, pathFirewallFilter, func(r pmk.Row) bool {
				return r["chain"] == "forward" && r["protocol"] == "tcp" && r["dst-port"] == "53"
			})
		}},
		{"first forward rule (dns udp)", func() int {
			return removeManaged(t, env.raw, pathFirewallFilter, func(r pmk.Row) bool {
				return r["chain"] == "forward" && r["protocol"] == "udp" && r["dst-port"] == "53"
			})
		}},
		{"last forward rule (drop)", func() int {
			return removeManaged(t, env.raw, pathFirewallFilter, func(r pmk.Row) bool {
				return r["chain"] == "forward" && r["action"] == "drop"
			})
		}},
		{"all four forward rules", func() int {
			return removeManaged(t, env.raw, pathFirewallFilter, func(r pmk.Row) bool {
				return r["chain"] == "forward" && r["src-address-list"] == setupIsolirList
			})
		}},
		{"isolir profile", func() int {
			return removeManaged(t, env.raw, pathPPPProfile, func(r pmk.Row) bool { return r["name"] == setupDefaultProf })
		}},
		{"isolir pool and profile", func() int {
			n := removeManaged(t, env.raw, pathPPPProfile, func(r pmk.Row) bool { return r["name"] == setupDefaultProf })
			return n + removeManaged(t, env.raw, pathIPPool, func(r pmk.Row) bool { return r["name"] == setupPoolName })
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			removed := c.remove()
			if removed == 0 {
				t.Fatalf("nothing removed for %q; setup layout differs from the test", c.name)
			}
			plan, err := env.svc.Preview(ctx, testScope(t), env.router.ID, env.in)
			if err != nil {
				t.Fatalf("preview after damage: %v", err)
			}
			creates, others := 0, []string{}
			for _, a := range plan.Actions {
				switch a.Kind {
				case domain.SetupCreate:
					creates++
				case domain.SetupExists:
				default:
					others = append(others, fmt.Sprintf("%s %s", a.Kind, a.Path))
				}
			}
			t.Logf("removed=%d preview: %d actions, creates=%d other=%v", removed, len(plan.Actions), creates, others)
			if creates != removed {
				t.Fatalf("preview plans %d creates for %d removed objects", creates, removed)
			}
			if len(others) != 0 {
				t.Fatalf("unexpected non-create/exists actions: %v", others)
			}

			before := env.counter.writes.Load()
			if _, err := env.svc.Apply(ctx, testScope(t), env.router.ID, env.in, plan.Hash, testActor); err != nil {
				t.Fatalf("apply repair: %v", err)
			}
			if wrote := env.counter.writes.Load() - before; int(wrote) != creates {
				t.Fatalf("repair wrote %d objects, want %d", wrote, creates)
			}
			got, contiguous := forwardSignature(t, env.raw)
			if !contiguous || strings.Join(got, ";") != strings.Join(want, ";") {
				t.Fatalf("forward rules after repair: %v contiguousTop=%v, want %v", got, contiguous, want)
			}
			again, err := env.svc.Preview(ctx, testScope(t), env.router.ID, env.in)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range again.Actions {
				if a.Kind != domain.SetupExists {
					t.Fatalf("after repair %s %s is %s", a.Kind, a.Path, a.Kind)
				}
			}
			if !env.repo.routers[env.router.ID].IsolirReady {
				t.Fatal("router not ready after repair")
			}
		})
	}

	if _, err := env.svc.Cleanup(ctx, testScope(t), env.router.ID, testActor); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if left := labZyadObjects(t, env.raw); len(left) != 0 {
		t.Fatalf("zyad objects left after cleanup: %v", left)
	}
}

// TestLabSetupCleanupRefusesWhileProfileInUse checks that cleanup refuses with a
// conflict error and removes nothing while a PPP secret still uses the isolir
// profile, and succeeds once the secret is gone.
func TestLabSetupCleanupRefusesWhileProfileInUse(t *testing.T) {
	env := newLabSetupEnv(t)
	ctx := context.Background()
	env.applyFresh(t)
	before := len(labZyadObjects(t, env.raw))
	if before == 0 {
		t.Fatal("setup left no zyad objects")
	}

	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	const secretName = "zyad-labtest-inuse"
	secretID, err := env.raw.Add(ctx, pathPPPSecret, pmk.Row{
		"name": secretName, "password": hex.EncodeToString(buf), "service": "pppoe",
		"profile": setupDefaultProf, "disabled": "yes", "comment": "zyad-labtest",
	})
	if err != nil {
		t.Fatalf("add test secret: %v", err)
	}
	removed := false
	removeSecret := func() {
		if removed {
			return
		}
		if err := env.raw.Remove(ctx, pathPPPSecret, secretID); err != nil {
			t.Errorf("remove test secret %s: %v", secretName, err)
			return
		}
		removed = true
	}
	// Registered after the env safety cleanup, so it runs first.
	t.Cleanup(removeSecret)

	_, err = env.svc.Cleanup(ctx, testScope(t), env.router.ID, testActor)
	var conflict *mikrotik.SetupConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("cleanup with secret in use: err=%v, want SetupConflictError", err)
	}
	t.Logf("cleanup refused: kind=%q detail=%q", conflict.Kind, conflict.Detail)
	if after := len(labZyadObjects(t, env.raw)); after != before {
		t.Fatalf("refused cleanup still removed objects: %d -> %d", before, after)
	}
	if !env.repo.routers[env.router.ID].IsolirReady {
		t.Fatal("router lost isolir_ready although cleanup was refused")
	}

	removeSecret()
	if _, err := env.svc.Cleanup(ctx, testScope(t), env.router.ID, testActor); err != nil {
		t.Fatalf("cleanup after the secret is gone: %v", err)
	}
	if left := labZyadObjects(t, env.raw); len(left) != 0 {
		t.Fatalf("zyad objects left after cleanup: %v", left)
	}
	if env.repo.routers[env.router.ID].IsolirReady {
		t.Fatal("router still marked ready after cleanup")
	}
}
