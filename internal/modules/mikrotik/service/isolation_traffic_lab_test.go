//go:build mikrotiklab

package service

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Traffic from an isolated PPPoE client (runbook section 10 items 2, 3, 4).
// Probes run on the CPE over SSH (management path 192.168.56.x, which does not
// go through the PPPoE session) before isolation, while isolated, and after
// restore, so every result has a control.
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 MIKROTIK_LAB_LIVE_USER='10mbps@debug' \
//	  MIKROTIK_LAB_CPE_SSH='root@192.168.56.50' MIKROTIK_LAB_CPE_KEY=~/.ssh/zyad_lab_cpe \
//	  go test -tags mikrotiklab -run TestLabLiveIsolatedTraffic -v -count=1 -timeout 25m ./internal/modules/mikrotik/service/
//
// The probes are read-only (nslookup, wget to /dev/null, ping); the router side
// is the same setup + Executor.Isolate/Restore as TestLabLiveIsolateRestore.

func cpeRun(t *testing.T, cmd string) (string, int) {
	t.Helper()
	host, key := os.Getenv("MIKROTIK_LAB_CPE_SSH"), os.Getenv("MIKROTIK_LAB_CPE_KEY")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-i", key, "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", host, cmd).CombinedOutput()
	code := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		code = -1
	}
	return string(out), code
}

type cpeProbe struct {
	name string
	cmd  string
	ok   func(out string, code int) bool
}

// okFetch means the TCP/TLS connection worked and the server answered: a
// completed download or any HTTP status (e.g. DOKU sandbox answers 503 on "/").
// A dropped flow ends in a connect error or timeout instead.
func okFetch(out string, code int) bool {
	return (code == 0 && strings.Contains(out, "Download completed")) || strings.Contains(out, "HTTP error")
}
func okDNS(out string, code int) bool {
	return code == 0 && strings.Contains(out, "Name:") && !strings.Contains(out, "NXDOMAIN") && !strings.Contains(out, "can't resolve")
}
func okPing(out string, code int) bool { return code == 0 && strings.Contains(out, "bytes from") }

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func TestLabLiveIsolatedTraffic(t *testing.T) {
	liveUser := os.Getenv("MIKROTIK_LAB_LIVE_USER")
	if liveUser == "" || os.Getenv("MIKROTIK_LAB_CPE_SSH") == "" || os.Getenv("MIKROTIK_LAB_CPE_KEY") == "" {
		t.Skip("MIKROTIK_LAB_LIVE_USER / MIKROTIK_LAB_CPE_SSH / MIKROTIK_LAB_CPE_KEY not set")
	}
	env := newLabSetupEnv(t)
	ctx := context.Background()
	env.in.DokuHosts = []string{"sandbox.doku.com", "api-sandbox.doku.com"}
	env.applyFresh(t)
	pool, err := netip.ParsePrefix(env.in.PoolCIDR)
	if err != nil {
		t.Fatal(err)
	}
	isolirGW := pool.Masked().Addr().Next().String() // profile local-address = first host of the pool

	rows, err := env.raw.Print(ctx, pppSecretPath(), pmk.Row{"name": liveUser})
	if err != nil || len(rows) != 1 || rows[0]["name"] != liveUser {
		t.Fatalf("secret %s: rows=%d err=%v", liveUser, len(rows), err)
	}
	secretID, origProfile := rows[0][".id"], rows[0]["profile"]
	if origProfile == setupDefaultProf || rows[0]["remote-address"] != "" {
		t.Skipf("secret %s is not in its normal pool-addressed state", liveUser)
	}
	if liveSession(t, env.raw, liveUser) == nil {
		t.Skipf("no active session for %s", liveUser)
	}
	t.Cleanup(func() {
		cur, err := env.raw.Print(context.Background(), pppSecretPath(), pmk.Row{"name": liveUser})
		if err != nil || len(cur) != 1 {
			t.Errorf("cleanup: read secret: rows=%d err=%v (restore %s manually: profile=%q)", len(cur), err, liveUser, origProfile)
			return
		}
		if cur[0]["profile"] != origProfile {
			if err := env.raw.Set(context.Background(), pppSecretPath(), secretID, pmk.Row{"profile": origProfile}); err != nil {
				t.Errorf("cleanup: restore secret %s: %v (restore manually: profile=%q)", liveUser, err, origProfile)
			}
		}
	})

	probes := func(dnsGW string) []cpeProbe {
		return []cpeProbe{
			{"dns via router (" + dnsGW + ")", "nslookup zyad.online " + dnsGW, okDNS},
			{"dns external resolver 8.8.8.8", "nslookup zyad.online 8.8.8.8", okDNS},
			{"http  allowlisted zyad.online", "wget -T 6 -O /dev/null http://zyad.online/ 2>&1", okFetch},
			{"https allowlisted zyad.online", "wget -T 6 -O /dev/null https://zyad.online/ 2>&1", okFetch},
			{"https doku sandbox.doku.com", "wget -T 6 -O /dev/null https://sandbox.doku.com/ 2>&1", okFetch},
			{"https doku api-sandbox.doku.com", "wget -T 6 -O /dev/null https://api-sandbox.doku.com/ 2>&1", okFetch},
			{"http  other example.com", "wget -T 6 -O /dev/null http://example.com/ 2>&1", okFetch},
			{"https other example.com", "wget -T 6 -O /dev/null https://example.com/ 2>&1", okFetch},
			{"icmp  other 8.8.8.8", "ping -c2 -W2 8.8.8.8 2>&1", okPing},
		}
	}
	run := func(phase string, ps []cpeProbe) map[string]bool {
		res := map[string]bool{}
		for _, p := range ps {
			out, code := cpeRun(t, p.cmd)
			ok := p.ok(out, code)
			res[p.name] = ok
			t.Logf("%-9s %-36s ok=%-5v exit=%d | %s", phase, p.name, ok, code, lastLine(out))
		}
		return res
	}
	want := func(phase string, got map[string]bool, name string, expect bool) {
		t.Helper()
		for k, v := range got {
			if strings.HasPrefix(k, name) && v != expect {
				t.Errorf("%s: %q ok=%v, want %v", phase, k, v, expect)
			}
		}
	}

	// Phase 1: control, normal profile.
	normalGW := "10.255.5.1"
	before := run("normal", probes(normalGW))
	for _, k := range []string{"dns", "http", "https", "icmp"} {
		want("normal", before, k, true)
	}

	// Phase 2: isolated.
	ex := NewExecutor(env.svc.routers)
	scope := testScope(t)
	sec := domain.PPPSecret{RouterID: env.router.ID, Name: liveUser, OriginalProfile: origProfile}
	normal := liveSession(t, env.raw, liveUser)
	cBefore := managedForwardCounters(t, env.raw)
	if _, err := ex.Isolate(ctx, scope, sec); err != nil {
		t.Fatalf("Isolate: %v", err)
	}
	iso := waitSession(t, env.raw, liveUser, normal[".id"], func(r pmk.Row) bool { return addrIn(pool, r["address"]) })
	t.Logf("isolated session %s address=%s (pool %s, gateway %s)", iso[".id"], iso["address"], pool, isolirGW)
	time.Sleep(15 * time.Second) // let netifd refresh resolv.conf / routes
	t.Logf("CPE resolv: %s", strings.ReplaceAll(strings.TrimSpace(func() string {
		o, _ := cpeRun(t, "cat /tmp/resolv.conf.d/resolv.conf.auto 2>/dev/null | grep nameserver")
		return o
	}()), "\n", " ; "))
	isolated := run("isolated", append(probes(isolirGW), cpeProbe{"dns via lo-dns (10.255.5.1)", "nslookup zyad.online 10.255.5.1", okDNS}))
	cAfter := managedForwardCounters(t, env.raw)
	keys := make([]string, 0, len(cAfter))
	for k := range cAfter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("forward rule %-34s before[%s] after[%s]", k, cBefore[k], cAfter[k])
	}
	for _, k := range []string{"dns via router (" + isolirGW, "dns external", "http  allowlisted", "https allowlisted"} {
		want("isolated", isolated, k, true)
	}
	for _, k := range []string{"http  other", "https other", "icmp"} {
		want("isolated", isolated, k, false)
	}

	// Phase 3: restored.
	if err := ex.Restore(ctx, scope, sec); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	waitSession(t, env.raw, liveUser, iso[".id"], func(r pmk.Row) bool { return !addrIn(pool, r["address"]) })
	time.Sleep(15 * time.Second)
	after := run("restored", probes(normalGW))
	for _, k := range []string{"dns", "http", "https", "icmp"} {
		want("restored", after, k, true)
	}
}
