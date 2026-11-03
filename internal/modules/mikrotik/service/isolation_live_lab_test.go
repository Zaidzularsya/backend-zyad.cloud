//go:build mikrotiklab

package service

import (
	"context"
	"net/netip"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Live isolate/restore against a real PPPoE client (CPE) connected to the lab
// router (runbook section 10 items 1, 5, 6).
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 MIKROTIK_LAB_LIVE_USER='10mbps@debug' \
//	  go test -tags mikrotiklab -run TestLabLiveIsolateRestore -v -count=1 -timeout 20m ./internal/modules/mikrotik/service/
//
// It builds the full setup (SetupService) on the router, then isolates and
// restores the named secret through the same Executor the worker uses. The
// secret's profile is put back by a cleanup that runs before the setup cleanup.
// Only the profile and session of ONE named secret are touched; passwords and
// credentials are never logged.
//
// Item 7 (a secret with its own remote-address) was verified once by hand on
// 2 Nov 2026 and is deliberately NOT part of this test: RouterClient has no
// "unset", so the remote-address could not be removed again from the test.

const liveWait = 150 * time.Second

func liveSession(t *testing.T, c pmk.RouterClient, user string) pmk.Row {
	t.Helper()
	rows, err := c.Print(context.Background(), pppActivePath, pmk.Row{"name": user})
	if err != nil {
		t.Fatalf("print /ppp/active: %v", err)
	}
	for _, r := range rows {
		if r["name"] == user {
			return r
		}
	}
	return nil
}

// waitSession polls until a session for user exists whose id differs from notID
// and accept(row) is true, or fails after liveWait.
func waitSession(t *testing.T, c pmk.RouterClient, user, notID string, accept func(pmk.Row) bool) pmk.Row {
	t.Helper()
	deadline := time.Now().Add(liveWait)
	for time.Now().Before(deadline) {
		if r := liveSession(t, c, user); r != nil && r[".id"] != notID && accept(r) {
			return r
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("no matching new session for %s within %s", user, liveWait)
	return nil
}

func addrIn(prefix netip.Prefix, s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && prefix.Contains(a)
}

func isolirListEntries(t *testing.T, c pmk.RouterClient) map[string]string {
	t.Helper()
	rows, err := c.Print(context.Background(), pathAddressList, pmk.Row{"list": setupIsolirList})
	if err != nil {
		t.Fatalf("print address-list: %v", err)
	}
	out := map[string]string{}
	for _, r := range rows {
		if r["list"] == setupIsolirList {
			out[r["address"]] = r["dynamic"]
		}
	}
	return out
}

func managedForwardCounters(t *testing.T, c pmk.RouterClient) map[string]string {
	t.Helper()
	rows, err := c.Print(context.Background(), pathFirewallFilter, pmk.Row{"chain": "forward"})
	if err != nil {
		t.Fatalf("print filter: %v", err)
	}
	out := map[string]string{}
	for _, r := range rows {
		if r["comment"] == setupManaged {
			out[strings.Join([]string{r["action"], r["protocol"], r["dst-port"], r["dst-address-list"]}, "|")] = "packets=" + r["packets"] + " bytes=" + r["bytes"]
		}
	}
	return out
}

func TestLabLiveIsolateRestore(t *testing.T) {
	liveUser := os.Getenv("MIKROTIK_LAB_LIVE_USER")
	if liveUser == "" {
		t.Skip("MIKROTIK_LAB_LIVE_USER not set (PPPoE user of a connected lab CPE)")
	}
	env := newLabSetupEnv(t)
	ctx := context.Background()
	env.applyFresh(t)
	pool, err := netip.ParsePrefix(env.in.PoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	// Preconditions on the live secret and session.
	rows, err := env.raw.Print(ctx, pppSecretPath(), pmk.Row{"name": liveUser})
	if err != nil || len(rows) != 1 || rows[0]["name"] != liveUser {
		t.Fatalf("secret %s: rows=%d err=%v", liveUser, len(rows), err)
	}
	secretID, origProfile, origRemote := rows[0][".id"], rows[0]["profile"], rows[0]["remote-address"]
	if origRemote != "" {
		t.Skipf("secret %s has its own remote-address; this test expects pool addressing", liveUser)
	}
	if origProfile == setupDefaultProf {
		t.Fatalf("secret already uses the isolir profile %q; refusing to run", setupDefaultProf)
	}
	before := liveSession(t, env.raw, liveUser)
	if before == nil {
		t.Skipf("no active session for %s; start the CPE first", liveUser)
	}
	t.Logf("before: session id=%s address=%s profile=%q remote-address=%q pool=%s", before[".id"], before["address"], origProfile, origRemote, pool)
	if addrIn(pool, before["address"]) {
		t.Fatalf("session address already inside the isolir pool")
	}

	// Always put the secret back before the setup cleanup (LIFO) runs.
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

	ex := NewExecutor(env.svc.routers)
	scope := testScope(t)
	sec := domain.PPPSecret{RouterID: env.router.ID, Name: liveUser, OriginalProfile: origProfile}

	run := func(t *testing.T, label string) {
		cur := liveSession(t, env.raw, liveUser)
		if cur == nil {
			cur = waitSession(t, env.raw, liveUser, "", func(pmk.Row) bool { return true })
		}
		normalID, normalAddr := cur[".id"], cur["address"]
		cBefore := managedForwardCounters(t, env.raw)

		orig, err := ex.Isolate(ctx, scope, sec)
		if err != nil {
			t.Fatalf("%s: Isolate: %v", label, err)
		}
		if orig != origProfile {
			t.Fatalf("%s: Isolate returned original %q, want %q", label, orig, origProfile)
		}
		t.Logf("%s: Isolate ok (original %q); secret profile now isolir", label, orig)

		// Item 6: the cut session's id no longer exists; what does Remove say?
		if gone := liveSession(t, env.raw, liveUser); gone == nil || gone[".id"] != normalID {
			if err := env.raw.Remove(ctx, pppActivePath, normalID); err != nil {
				t.Logf("%s: item 6 Remove on a vanished session id %s -> error: %v", label, normalID, err)
			} else {
				t.Logf("%s: item 6 Remove on vanished session id %s -> NO error", label, normalID)
			}
		}

		iso := waitSession(t, env.raw, liveUser, normalID, func(r pmk.Row) bool { return addrIn(pool, r["address"]) })
		t.Logf("%s: item 1/5: old session %s (%s) cut; CPE reconnected as %s address=%s (inside isolir pool %s)", label, normalID, normalAddr, iso[".id"], iso["address"], pool)

		entries := isolirListEntries(t, env.raw)
		dyn, ok := entries[iso["address"]]
		t.Logf("%s: item 1: address-list %s has %d entries; entry for %s present=%v dynamic=%q", label, setupIsolirList, len(entries), iso["address"], ok, dyn)
		if !ok {
			t.Errorf("%s: isolated session address %s is not in address-list %s", label, iso["address"], setupIsolirList)
		}

		time.Sleep(20 * time.Second)
		cAfter := managedForwardCounters(t, env.raw)
		keys := make([]string, 0, len(cAfter))
		for k := range cAfter {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Logf("%s: forward rule %-34s before[%s] after[%s]", label, k, cBefore[k], cAfter[k])
		}

		if err := ex.Restore(ctx, scope, sec); err != nil {
			t.Fatalf("%s: Restore: %v", label, err)
		}
		back := waitSession(t, env.raw, liveUser, iso[".id"], func(r pmk.Row) bool { return !addrIn(pool, r["address"]) })
		t.Logf("%s: item 5 restore: CPE reconnected as %s address=%s (outside the isolir pool)", label, back[".id"], back["address"])
		if left := isolirListEntries(t, env.raw); func() bool { _, still := left[iso["address"]]; return still }() {
			t.Errorf("%s: address %s still in %s after restore", label, iso["address"], setupIsolirList)
		}
		now, err := env.raw.Print(ctx, pppSecretPath(), pmk.Row{"name": liveUser})
		if err != nil || len(now) != 1 || now[0]["profile"] != origProfile {
			t.Fatalf("%s: profile after restore: rows=%d err=%v", label, len(now), err)
		}
	}

	run(t, "profile-pool")
}

func pppSecretPath() string { return pathPPPSecret }
