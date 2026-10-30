package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

const (
	pathPool    = "/ip/pool"
	pathAddr    = "/ip/address"
	pathProfile = "/ppp/profile"
	pathList    = "/ip/firewall/address-list"
	pathFilter  = "/ip/firewall/filter"
	pathNAT     = "/ip/firewall/nat"
	pathProxy   = "/ip/proxy/access"
	pathSecret  = "/ppp/secret"
)

type setupFixture struct {
	svc      *SetupService
	repo     *fakeRepo
	client   *pmk.FakeClient
	locker   *fakeLocker
	routerID string
}

// fakeLocker records TryLock/unlock calls. held simulates another holder.
type fakeLocker struct {
	held     bool
	err      error
	keys     []string
	acquired int
	released int
	// onLock runs inside TryLock, e.g. to observe what was read before locking.
	onLock func()
}

func (l *fakeLocker) TryLock(_ context.Context, key string) (func(), bool, error) {
	l.keys = append(l.keys, key)
	if l.onLock != nil {
		l.onLock()
	}
	if l.err != nil {
		return nil, false, l.err
	}
	if l.held {
		return nil, false, nil
	}
	l.acquired++
	return func() { l.released++ }, true, nil
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	rs, repo, factory := newTestService(t)
	// Port 8728 routers are detected as binary; REST setup has its own gate.
	factory.detectAPI = pmk.APITypeBinary
	router, err := rs.Register(context.Background(), testScope(t), validInput())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	locker := &fakeLocker{}
	return &setupFixture{svc: NewSetupService(rs, repo, locker, SetupOptions{}), repo: repo, client: factory.client, locker: locker, routerID: router.ID}
}

func validSetup() SetupInput {
	return SetupInput{
		PoolCIDR:         "10.255.254.0/24",
		PlatformHost:     "zyad.online",
		TenantIsolirHost: "tenant.example.com",
		DokuHosts:        []string{"sandbox.doku.com"},
	}
}

func (f *setupFixture) preview(t *testing.T, in SetupInput) domain.SetupPlan {
	t.Helper()
	plan, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, in)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return plan
}

func countKinds(p domain.SetupPlan) map[string]int {
	m := map[string]int{}
	for _, a := range p.Actions {
		m[a.Kind]++
	}
	return m
}

func TestPreviewOnEmptyRouterListsCreateActions(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	if len(plan.Actions) != 9 {
		t.Fatalf("actions = %d, want 9 (pool, profile, 3 allowlist, F1-F4): %+v", len(plan.Actions), plan.Actions)
	}
	for _, a := range plan.Actions {
		if a.Kind != domain.SetupCreate {
			t.Errorf("%s %v: kind %s, want create", a.Path, a.Match, a.Kind)
		}
		if a.Attrs["comment"] != "zyad-managed" {
			t.Errorf("%s: missing comment=zyad-managed: %v", a.Path, a.Attrs)
		}
		if a.Optional {
			t.Errorf("%s: default plan must not contain optional actions", a.Path)
		}
	}
	// Order: pool, profile, allowlist, forward rules F1..F4.
	wantPaths := []string{pathPool, pathProfile, pathList, pathList, pathList, pathFilter, pathFilter, pathFilter, pathFilter}
	for i, a := range plan.Actions {
		if a.Path != wantPaths[i] {
			t.Errorf("action %d path = %s, want %s", i, a.Path, wantPaths[i])
		}
	}
	pool, profile := plan.Actions[0], plan.Actions[1]
	if pool.Attrs["name"] != "zyad-isolir-pool" || pool.Attrs["ranges"] != "10.255.254.2-10.255.254.254" {
		t.Errorf("pool attrs = %v", pool.Attrs)
	}
	if profile.Attrs["name"] != "zyad-isolir" || profile.Attrs["local-address"] != "10.255.254.1" ||
		profile.Attrs["remote-address"] != "zyad-isolir-pool" || profile.Attrs["dns-server"] != "10.255.254.1" ||
		profile.Attrs["address-list"] != "zyad-isolir" || profile.Attrs["only-one"] != "yes" {
		t.Errorf("profile attrs = %v", profile.Attrs)
	}
	if _, ok := profile.Attrs["rate-limit"]; ok {
		t.Error("rate-limit must be omitted when empty")
	}
	f4 := plan.Actions[8]
	if f4.Attrs["action"] != "drop" || f4.Attrs["src-address-list"] != "zyad-isolir" || f4.Attrs["chain"] != "forward" {
		t.Errorf("F4 attrs = %v", f4.Attrs)
	}
	if _, ok := f4.Attrs["place-before"]; ok {
		t.Error("no place-before on an empty forward chain")
	}
	if plan.Hash == "" {
		t.Error("hash empty")
	}
}

func TestPreviewIsDeterministic(t *testing.T) {
	f := newSetupFixture(t)
	a, b := f.preview(t, validSetup()), f.preview(t, validSetup())
	if a.Hash != b.Hash {
		t.Fatalf("hash differs: %s vs %s", a.Hash, b.Hash)
	}
	in := validSetup()
	in.DokuHosts = []string{"x.doku.com"}
	if c := f.preview(t, in); c.Hash == a.Hash {
		t.Fatal("different input must change hash")
	}
}

func TestPreviewDoesNotWrite(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "accept"})
	audits := len(f.repo.audits)
	f.preview(t, validSetup())
	if n := f.client.WriteCalls(); n != 0 {
		t.Fatalf("preview wrote %d times", n)
	}
	if len(f.repo.setups) != 0 || len(f.repo.audits) != audits {
		t.Fatal("preview must not touch repo state")
	}
}

func TestApplyWritesExactlyThePreviewedPlan(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "jump", "comment": "owner"})
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if n := f.client.WriteCalls(); n != len(plan.Actions) {
		t.Fatalf("writes = %d, want %d", n, len(plan.Actions))
	}
	if got := len(f.client.Rows(pathPool)); got != 1 {
		t.Errorf("pools = %d", got)
	}
	if got := len(f.client.Rows(pathProfile)); got != 1 {
		t.Errorf("profiles = %d", got)
	}
	if got := len(f.client.Rows(pathList)); got != 3 {
		t.Errorf("allowlist rows = %d", got)
	}
	// Forward chain: F1..F4 in order, all before the owner's rule.
	fwd := f.client.Rows(pathFilter)
	if len(fwd) != 5 || fwd[4]["comment"] != "owner" {
		t.Fatalf("forward chain = %+v", fwd)
	}
	wantActions := []string{"accept", "accept", "accept", "drop"}
	for i, w := range wantActions {
		if fwd[i]["action"] != w || fwd[i]["comment"] != "zyad-managed" {
			t.Errorf("forward[%d] = %v, want zyad %s", i, fwd[i], w)
		}
	}
	if fwd[0]["dst-port"] != "53" || fwd[0]["protocol"] != "udp" || fwd[1]["protocol"] != "tcp" || fwd[2]["dst-address-list"] != "zyad-isolir-allow" {
		t.Errorf("forward order wrong: %+v", fwd[:4])
	}
	// Redirect objects never appear by default.
	if len(f.client.Rows(pathNAT)) != 0 || len(f.client.Rows(pathProxy)) != 0 {
		t.Error("redirect objects written by default")
	}
}

func TestApplyTwiceDoesNotDuplicateObjects(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	before := f.client.WriteCalls()
	second := f.preview(t, validSetup())
	if k := countKinds(second); k[domain.SetupExists] != len(second.Actions) {
		t.Fatalf("second preview kinds = %v, want all exists", k)
	}
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), second.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	if f.client.WriteCalls() != before {
		t.Fatalf("second apply wrote %d times", f.client.WriteCalls()-before)
	}
	if len(f.client.Rows(pathFilter)) != 4 || len(f.client.Rows(pathList)) != 3 {
		t.Fatal("objects duplicated")
	}
}

func TestApplyWithStaleHashReturnsErrPlanChangedAndWritesNothing(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	// Router drifts between preview and apply: someone adds a forward rule.
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"})
	_, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor)
	if !errors.Is(err, mikrotik.ErrPlanChanged) || !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("err = %v, want ErrPlanChanged", err)
	}
	if n := f.client.WriteCalls(); n != 0 {
		t.Fatalf("stale apply wrote %d times", n)
	}
	if len(f.repo.setups) != 0 {
		t.Fatal("must not mark setup")
	}
}

func TestApplyMarksRouterReadyAndAudits(t *testing.T) {
	f := newSetupFixture(t)
	f.repo.audits = nil
	plan := f.preview(t, validSetup())
	router, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor)
	if err != nil {
		t.Fatal(err)
	}
	if !router.IsolirReady || router.IsolirProfileName != "zyad-isolir" {
		t.Fatalf("router = %+v", router)
	}
	last := f.repo.setups[len(f.repo.setups)-1]
	if !last.ready || last.profile != "zyad-isolir" || last.redirectURL != "" {
		t.Fatalf("MarkSetup = %+v", last)
	}
	if len(f.repo.audits) != 1 || f.repo.audits[0].Event != "router.setup_applied" {
		t.Fatalf("audits = %+v", f.repo.audits)
	}
	md := f.repo.audits[0].Metadata
	if md["profile"] != "zyad-isolir" || md["created"] != 9 {
		t.Errorf("metadata = %v", md)
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(md)), "pass") {
		t.Errorf("metadata leaks secrets: %v", md)
	}
}

func TestApplyFailureMidwayLeavesRouterNotReady(t *testing.T) {
	f := newSetupFixture(t)
	f.client.WriteHook = func(op, path string, _ pmk.Row) error {
		if path == pathFilter {
			return errors.New("failure: could not add " + testPassword)
		}
		return nil
	}
	plan := f.preview(t, validSetup())
	_, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor)
	var step *mikrotik.SetupStepError
	if !errors.As(err, &step) || !errors.Is(err, mikrotik.ErrSetupFailed) {
		t.Fatalf("err = %v, want SetupStepError", err)
	}
	if !strings.Contains(err.Error(), pathFilter) || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("error text = %q", err.Error())
	}
	for _, c := range f.repo.setups {
		if c.ready {
			t.Fatal("router must not be marked ready")
		}
	}
	if len(f.repo.audits) > 0 && f.repo.audits[len(f.repo.audits)-1].Event == "router.setup_applied" {
		t.Fatal("must not audit setup_applied")
	}
	// Retry after the fault clears finishes the job without duplicates.
	f.client.WriteHook = nil
	again := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), again.Hash, testActor); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(f.client.Rows(pathPool)) != 1 || len(f.client.Rows(pathFilter)) != 4 {
		t.Fatal("retry duplicated objects")
	}
}

func TestMarkSetupFailureAfterApplyStatesRouterWasWritten(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	f.repo.markErr = errors.New("db down")
	_, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor)
	if !errors.Is(err, mikrotik.ErrSetupStateNotSaved) || !strings.Contains(err.Error(), "router objects were changed") {
		t.Fatalf("err = %v", err)
	}
}

func TestCleanupRemovesOnlyZyadManagedObjects(t *testing.T) {
	f := newSetupFixture(t)
	// Pre-existing non-managed objects of every type, including look-alikes.
	f.client.Seed(pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.1.0.2-10.1.0.50"})
	f.client.Seed(pathProfile, pmk.Row{"name": "default"})
	f.client.Seed(pathList, pmk.Row{"list": "owner-suspend", "address": "10.9.9.9", "comment": "owner"})
	f.client.Seed(pathFilter,
		pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"},
		pmk.Row{"chain": "forward", "action": "drop", "src-address-list": "zyad-isolir"}, // no comment: not ours
	)
	f.client.Seed(pathNAT, pmk.Row{"chain": "dstnat", "action": "dst-nat", "comment": "owner"})
	f.client.Seed(pathProxy, pmk.Row{"src-address": "10.0.0.0/8", "action": "deny", "comment": "owner"})
	snapshot := map[string]int{}
	for _, p := range []string{pathPool, pathProfile, pathList, pathFilter, pathNAT, pathProxy} {
		snapshot[p] = len(f.client.Rows(p))
	}

	in := validSetup()
	in.RedirectURL = "https://tenant.example.com/isolir"
	plan := f.preview(t, in)
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, in, plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(f.client.Rows(pathNAT)) != 2 || len(f.client.Rows(pathProxy)) != 3 {
		t.Fatal("redirect objects expected when RedirectURL set")
	}
	// Look-alikes seeded after apply (Preview would refuse some of them).
	f.client.Seed(pathList,
		pmk.Row{"list": "zyad-isolir-allow", "address": "9.9.9.9", "comment": "owner"},                             // unmanaged static entry
		pmk.Row{"list": "zyad-isolir-allow", "address": "10.77.0.1", "comment": "zyad.online", "dynamic": "true"},  // dynamic, comment = FQDN
		pmk.Row{"list": "zyad-isolir-allow", "address": "10.77.0.2", "comment": "zyad-managed", "dynamic": "true"}, // dynamic even with the marker
	)
	f.client.Seed(pathPool, pmk.Row{"name": "zyad-isolir-pool-other", "ranges": "10.3.0.2-10.3.0.9", "comment": "owner"})
	f.client.Seed(pathProfile, pmk.Row{"name": "zyad-isolir-other", "address-list": "zyad-isolir", "comment": "owner"})
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "accept", "src-address-list": "zyad-isolir", "protocol": "udp", "dst-port": "53", "comment": "owner-lookalike"})
	f.client.Seed(pathNAT, pmk.Row{"chain": "dstnat", "action": "redirect", "src-address-list": "zyad-isolir", "comment": "owner-lookalike"})
	snapshot[pathList] += 3
	snapshot[pathPool]++
	snapshot[pathProfile]++
	snapshot[pathFilter]++
	snapshot[pathNAT]++
	f.repo.audits = nil
	router, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if router.IsolirReady {
		t.Fatal("cleanup must clear isolir_ready")
	}
	last := f.repo.setups[len(f.repo.setups)-1]
	if last.ready || last.profile != "" || last.redirectURL != "" {
		t.Fatalf("MarkSetup = %+v", last)
	}
	for p, n := range snapshot {
		rows := f.client.Rows(p)
		if len(rows) != n {
			t.Errorf("%s: %d rows after cleanup, want %d (pre-existing)", p, len(rows), n)
		}
		for _, r := range rows {
			if r["comment"] == "zyad-managed" && r["dynamic"] != "true" {
				t.Errorf("%s: managed row left: %v", p, r)
			}
		}
	}
	if len(f.repo.audits) != 1 || f.repo.audits[0].Event != "router.setup_cleaned" {
		t.Fatalf("audits = %+v", f.repo.audits)
	}
	// Second cleanup is a no-op.
	before := f.client.WriteCalls()
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); err != nil {
		t.Fatal(err)
	}
	if f.client.WriteCalls() != before {
		t.Fatal("second cleanup wrote")
	}
}

func TestCleanupRefusesWhileSecretsUseProfile(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	f.client.Seed(pathSecret, pmk.Row{"name": "pelanggan1", "profile": "zyad-isolir"})
	before := f.client.WriteCalls()
	_, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	if !errors.Is(err, mikrotik.ErrSetupConflict) {
		t.Fatalf("err = %v", err)
	}
	if f.client.WriteCalls() != before {
		t.Fatal("cleanup wrote despite secrets in isolir profile")
	}
	if strings.Contains(err.Error(), "pelanggan1") {
		t.Error("error must not echo secret names")
	}
}

func TestSetupRefusesUnmanagedProfileOrPoolWithSameName(t *testing.T) {
	for _, tc := range []struct{ path, name, kind string }{
		{pathPool, "zyad-isolir-pool", "ip pool"},
		{pathProfile, "zyad-isolir", "ppp profile"},
		{pathList, "", "address-list"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			f := newSetupFixture(t)
			row := pmk.Row{"name": tc.name, "comment": "owner"}
			if tc.path == pathList {
				row = pmk.Row{"list": "zyad-isolir-allow", "address": "zyad.online", "comment": "owner"}
			}
			f.client.Seed(tc.path, row)
			_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, validSetup())
			var conflict *mikrotik.SetupConflictError
			if !errors.Is(err, mikrotik.ErrSetupConflict) || !errors.As(err, &conflict) || conflict.Kind != tc.kind {
				t.Fatalf("err = %v, want conflict %s", err, tc.kind)
			}
			if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), "anything", testActor); !errors.Is(err, mikrotik.ErrSetupConflict) {
				t.Fatalf("apply err = %v", err)
			}
			if n := f.client.WriteCalls(); n != 0 {
				t.Fatalf("wrote %d times despite conflict", n)
			}
		})
	}
}

func TestPartialRuleSetPreservesOrder(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"})
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	// Operator deletes F3 by hand: partial set.
	var f3 pmk.Row
	for _, r := range f.client.Rows(pathFilter) {
		if r["dst-address-list"] == "zyad-isolir-allow" {
			f3 = r
		}
	}
	if err := f.client.Remove(context.Background(), pathFilter, f3[".id"]); err != nil {
		t.Fatal(err)
	}
	before := f.client.WriteCalls()
	p2 := f.preview(t, validSetup())
	if k := countKinds(p2); k[domain.SetupCreate] != 1 {
		t.Fatalf("kinds = %v, want exactly one create", k)
	}
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), p2.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	if f.client.WriteCalls()-before != 1 {
		t.Fatalf("writes = %d, want 1", f.client.WriteCalls()-before)
	}
	fwd := f.client.Rows(pathFilter)
	order := []string{"udp", "tcp", "allow", "drop", "owner"}
	for i, r := range fwd {
		var tag string
		switch {
		case r["comment"] == "owner":
			tag = "owner"
		case r["protocol"] == "udp":
			tag = "udp"
		case r["protocol"] == "tcp":
			tag = "tcp"
		case r["dst-address-list"] != "":
			tag = "allow"
		default:
			tag = "drop"
		}
		if tag != order[i] {
			t.Fatalf("position %d = %s, want %s (chain %+v)", i, tag, order[i], fwd)
		}
	}
}

func TestWrongRelativeOrderRequiresCleanup(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter,
		pmk.Row{"chain": "forward", "action": "drop", "src-address-list": "zyad-isolir", "comment": "zyad-managed"},
		pmk.Row{"chain": "forward", "action": "accept", "src-address-list": "zyad-isolir", "protocol": "udp", "dst-port": "53", "comment": "zyad-managed"},
	)
	_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, validSetup())
	var c *mikrotik.SetupConflictError
	if !errors.As(err, &c) || !strings.Contains(c.Detail, "cleanup") {
		t.Fatalf("err = %v", err)
	}
}

func TestForeignRuleBetweenZyadRulesRequiresCleanup(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter,
		pmk.Row{"chain": "forward", "action": "accept", "src-address-list": "zyad-isolir", "protocol": "udp", "dst-port": "53", "comment": "zyad-managed"},
		pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"},
		pmk.Row{"chain": "forward", "action": "drop", "src-address-list": "zyad-isolir", "comment": "zyad-managed"},
	)
	if _, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, validSetup()); !errors.Is(err, mikrotik.ErrSetupConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestPoolOverlapRejected(t *testing.T) {
	cases := map[string]struct {
		path string
		row  pmk.Row
	}{
		"pool":    {pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.255.254.100-10.255.254.200"}},
		"pool2":   {pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.1.0.2-10.1.0.9,10.255.253.250-10.255.254.5"}},
		"address": {pathAddr, pmk.Row{"address": "10.255.0.1/16", "interface": "ether1"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSetupFixture(t)
			f.client.Seed(tc.path, tc.row)
			_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, validSetup())
			if !errors.Is(err, mikrotik.ErrInvalidInput) || !strings.Contains(err.Error(), "overlap") {
				t.Fatalf("err = %v, want overlap invalid input", err)
			}
			if f.client.WriteCalls() != 0 {
				t.Fatal("wrote")
			}
		})
	}
	// Adjacent (non-overlapping) pool is fine.
	f := newSetupFixture(t)
	f.client.Seed(pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.255.253.2-10.255.253.254"})
	f.client.Seed(pathAddr, pmk.Row{"address": "192.168.1.1/24"})
	f.preview(t, validSetup())
}

func TestSetupInputValidation(t *testing.T) {
	for name, mutate := range map[string]func(*SetupInput){
		"empty pool":       func(in *SetupInput) { in.PoolCIDR = "" },
		"not cidr":         func(in *SetupInput) { in.PoolCIDR = "10.255.254.1" },
		"public pool":      func(in *SetupInput) { in.PoolCIDR = "8.8.8.0/24" },
		"too small":        func(in *SetupInput) { in.PoolCIDR = "10.255.254.0/30" },
		"too big":          func(in *SetupInput) { in.PoolCIDR = "10.0.0.0/8" },
		"ipv6":             func(in *SetupInput) { in.PoolCIDR = "fd00::/64" },
		"no platform host": func(in *SetupInput) { in.PlatformHost = "" },
		"url as host":      func(in *SetupInput) { in.PlatformHost = "https://zyad.online/x" },
		"bad doku host":    func(in *SetupInput) { in.DokuHosts = []string{"a b.com"} },
		"bad profile":      func(in *SetupInput) { in.ProfileName = "x y;z" },
		"bad rate":         func(in *SetupInput) { in.RateLimit = "fast" },
		"bad redirect":     func(in *SetupInput) { in.RedirectURL = "javascript:alert(1)" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newSetupFixture(t)
			in := validSetup()
			mutate(&in)
			_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, in)
			if !errors.Is(err, mikrotik.ErrInvalidInput) {
				t.Fatalf("err = %v, want invalid input", err)
			}
			if f.client.WriteCalls() != 0 {
				t.Fatal("wrote")
			}
		})
	}
}

func TestNonUUIDRouterIDIsNotFound(t *testing.T) {
	f := newSetupFixture(t)
	if _, err := f.svc.Preview(context.Background(), testScope(t), "nope", validSetup()); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("preview err = %v", err)
	}
	if _, err := f.svc.Apply(context.Background(), testScope(t), "nope", validSetup(), "h", testActor); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("apply err = %v", err)
	}
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), "nope", testActor); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("cleanup err = %v", err)
	}
}

func TestRedirectOffByDefault(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	for _, a := range plan.Actions {
		if a.Path == pathNAT || a.Path == pathProxy {
			t.Fatalf("redirect object planned by default: %+v", a)
		}
	}
	in := validSetup()
	in.RedirectURL = "https://tenant.example.com/isolir"
	withRedirect := f.preview(t, in)
	if len(withRedirect.Actions) != 12 {
		t.Fatalf("actions = %d, want 12", len(withRedirect.Actions))
	}
	optional := 0
	for _, a := range withRedirect.Actions {
		if a.Path == pathNAT || a.Path == pathProxy {
			if !a.Optional || a.Note == "" {
				t.Errorf("redirect action must be Optional with a note: %+v", a)
			}
			optional++
		} else if a.Optional {
			t.Errorf("non-redirect action marked optional: %+v", a)
		}
	}
	if optional != 3 {
		t.Fatalf("optional = %d, want 3 (NB, PA, PR)", optional)
	}
	// /ip proxy is a global setting and is never written.
	for _, a := range withRedirect.Actions {
		if a.Path == "/ip/proxy" {
			t.Fatal("setup must not touch /ip/proxy")
		}
	}
	// Apply with redirect records the URL.
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, in, withRedirect.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	last := f.repo.setups[len(f.repo.setups)-1]
	if last.redirectURL != in.RedirectURL {
		t.Fatalf("MarkSetup redirect = %q", last.redirectURL)
	}
	if rows := f.client.Rows("/ip/proxy"); len(rows) != 0 {
		t.Fatal("proxy settings touched")
	}
}

func TestSetupErrorsNeverContainPassword(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	leaky := errors.New("dial tcp: login failed for admin password=" + testPassword)
	f := newSetupFixture(t)
	f.client.Err = leaky
	ctx := context.Background()
	_, e1 := f.svc.Preview(ctx, testScope(t), f.routerID, validSetup())
	_, e2 := f.svc.Apply(ctx, testScope(t), f.routerID, validSetup(), "h", testActor)
	_, e3 := f.svc.Cleanup(ctx, testScope(t), f.routerID, testActor)
	for i, err := range []error{e1, e2, e3} {
		if err == nil {
			t.Fatalf("call %d: expected error", i)
		}
		if !errors.Is(err, mikrotik.ErrUnreachable) {
			t.Errorf("call %d: err = %v, want mapped unreachable", i, err)
		}
		if strings.Contains(err.Error(), testPassword) {
			t.Errorf("call %d leaks password: %v", i, err)
		}
	}

	// Mid-apply write failure with a leaky error.
	g := newSetupFixture(t)
	g.client.WriteHook = func(string, string, pmk.Row) error { return leaky }
	plan := g.preview(t, validSetup())
	_, err := g.svc.Apply(ctx, testScope(t), g.routerID, validSetup(), plan.Hash, testActor)
	if err == nil || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("apply err = %v", err)
	}
	if strings.Contains(logs.String(), testPassword) {
		t.Fatalf("logs leak password:\n%s", logs.String())
	}
}

func applyReady(t *testing.T, f *setupFixture) {
	t.Helper()
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

func TestCleanupIgnoresServerSideFilterAndRemovesOnlyOurs(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	f.client.Seed(pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.3.0.2-10.3.0.9"})
	f.client.Seed(pathProfile, pmk.Row{"name": "default"})
	f.client.Seed(pathList, pmk.Row{"list": "other", "address": "1.1.1.1", "comment": "zyad-managed"}, pmk.Row{"list": "zyad-isolir-allow", "address": "8.8.8.8"})
	f.client.Seed(pathFilter, pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"}, pmk.Row{"chain": "input", "action": "drop", "src-address-list": "zyad-isolir", "comment": "zyad-managed"})
	f.client.Seed(pathNAT, pmk.Row{"chain": "dstnat", "action": "dst-nat", "comment": "owner"})
	f.client.Seed(pathProxy, pmk.Row{"src-address": "10.0.0.0/8", "action": "deny", "comment": "owner"})
	f.client.IgnoreFilters = true
	_, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	var conflict *mikrotik.SetupConflictError
	if !errors.Is(err, mikrotik.ErrSetupConflict) || !errors.As(err, &conflict) || !strings.Contains(conflict.Detail, "could not be verified as zyad-managed") {
		t.Fatalf("cleanup err = %v, want typed unverified-rows error", err)
	}
	if f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("isolir_ready must stay false")
	}
	f.client.IgnoreFilters = false
	want := map[string]int{pathPool: 1, pathProfile: 1, pathList: 2, pathFilter: 2, pathNAT: 1, pathProxy: 1}
	for p, n := range want {
		if got := len(f.client.Rows(p)); got != n {
			t.Errorf("%s: %d rows, want %d (only pre-existing rows must remain): %+v", p, got, n, f.client.Rows(p))
		}
	}
	for _, r := range f.client.Rows(pathFilter) {
		if r["comment"] == "zyad-managed" && r["chain"] == "forward" {
			t.Errorf("our rule left: %v", r)
		}
	}
}

func TestCleanupFailsClosedWhenMarkNotReadyFails(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	f.repo.markErr = errors.New("db down")
	before := f.client.WriteCalls()
	_, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	if !errors.Is(err, mikrotik.ErrSetupMarkFailed) {
		t.Fatalf("err = %v", err)
	}
	if f.client.WriteCalls() != before {
		t.Fatal("cleanup removed objects although isolir_ready could not be cleared")
	}
}

func TestFailedCleanupLeavesRouterNotReady(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	if !f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("precondition: ready")
	}
	f.client.WriteHook = func(op, path string, _ pmk.Row) error {
		if op == "remove" && path == pathPool {
			return errors.New("failure: in use")
		}
		return nil
	}
	_, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	var step *mikrotik.SetupStepError
	if !errors.As(err, &step) || !strings.HasPrefix(step.Step, "remove ") {
		t.Fatalf("err = %v", err)
	}
	if f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("failed cleanup must leave isolir_ready=false")
	}
}

func TestFailedReapplyOfReadyRouterLeavesNotReady(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	for _, r := range f.client.Rows(pathFilter) {
		if r["dst-address-list"] == "zyad-isolir-allow" {
			_ = f.client.Remove(context.Background(), pathFilter, r[".id"])
		}
	}
	f.client.WriteHook = func(op, path string, _ pmk.Row) error {
		if op == "add" && path == pathFilter {
			return errors.New("failure")
		}
		return nil
	}
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err == nil {
		t.Fatal("expected failure")
	}
	if f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("failed re-apply must leave isolir_ready=false")
	}
	// A no-op re-apply (all exists) of a ready router keeps it ready and does not toggle.
	g := newSetupFixture(t)
	applyReady(t, g)
	calls := len(g.repo.setups)
	plan = g.preview(t, validSetup())
	if _, err := g.svc.Apply(context.Background(), testScope(t), g.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	for _, c := range g.repo.setups[calls:] {
		if !c.ready {
			t.Fatal("no-op re-apply must not mark not-ready")
		}
	}
}

func TestReapplyMarkNotReadyFailureWritesNothing(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	for _, r := range f.client.Rows(pathFilter) {
		if r["action"] == "drop" {
			_ = f.client.Remove(context.Background(), pathFilter, r[".id"])
		}
	}
	plan := f.preview(t, validSetup())
	f.repo.markErr = errors.New("db down")
	before := f.client.WriteCalls()
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); !errors.Is(err, mikrotik.ErrSetupMarkFailed) {
		t.Fatalf("err = %v", err)
	}
	if f.client.WriteCalls() != before {
		t.Fatal("wrote despite failed mark-not-ready")
	}
}

func TestDynamicForwardRuleIsNeverAnAnchor(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathFilter,
		pmk.Row{"chain": "forward", "action": "passthrough", "dynamic": "true", "comment": "special dummy rule to show fasttrack counters"},
		pmk.Row{"chain": "forward", "action": "accept", "comment": "owner"},
	)
	plan := f.preview(t, validSetup())
	ownerID := ""
	for _, r := range f.client.Rows(pathFilter) {
		if r["comment"] == "owner" {
			ownerID = r[".id"]
		}
	}
	if got := plan.Actions[5].Attrs["place-before"]; got != ownerID {
		t.Fatalf("place-before = %q, want first static non-zyad rule %q", got, ownerID)
	}
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatalf("apply (incl. verify): %v", err)
	}
	// Second preview still all exists although the dynamic rule sits above the zyad block.
	if k := countKinds(f.preview(t, validSetup())); k[domain.SetupExists] != 9 {
		t.Fatalf("kinds = %v", k)
	}
}

func TestAnyStaticUnmanagedAllowlistEntryIsRefused(t *testing.T) {
	f := newSetupFixture(t)
	f.client.Seed(pathList, pmk.Row{"list": "zyad-isolir-allow", "address": "203.0.113.9", "comment": "owner"})
	_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, validSetup())
	var c *mikrotik.SetupConflictError
	if !errors.As(err, &c) || c.Kind != "address-list" || c.Name != "zyad-isolir-allow" || strings.Contains(err.Error(), "203.0.113.9") {
		t.Fatalf("err = %v", err)
	}
	// Dynamic rows and other lists do not count.
	g := newSetupFixture(t)
	g.client.Seed(pathList,
		pmk.Row{"list": "zyad-isolir-allow", "address": "10.1.1.1", "comment": "zyad.online", "dynamic": "true"},
		pmk.Row{"list": "someone-else", "address": "1.2.3.4", "comment": "owner"})
	g.preview(t, validSetup())
}

func TestApplyWritePhaseSurvivesRequestCancellation(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	ctx, cancel := context.WithCancel(context.Background())
	first := true
	f.client.WriteHook = func(string, string, pmk.Row) error {
		if first {
			first = false
			cancel() // client disconnects after the first write
		}
		return nil
	}
	if _, err := f.svc.Apply(ctx, testScope(t), f.routerID, validSetup(), plan.Hash, testActor); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if f.client.WriteCalls() != 9 || !f.repo.routers[f.routerID].IsolirReady {
		t.Fatalf("writes = %d ready = %v", f.client.WriteCalls(), f.repo.routers[f.routerID].IsolirReady)
	}
}

func TestCleanupSurvivesRequestCancellation(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	first := true
	f.client.WriteHook = func(string, string, pmk.Row) error {
		if first {
			first = false
			cancel()
		}
		return nil
	}
	if _, err := f.svc.Cleanup(ctx, testScope(t), f.routerID, testActor); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(f.client.Rows(pathFilter))+len(f.client.Rows(pathPool))+len(f.client.Rows(pathProfile))+len(f.client.Rows(pathList)) != 0 {
		t.Fatal("cleanup aborted midway")
	}
}

func TestAllowlistHostsMustBeIPv4OrFQDN(t *testing.T) {
	for _, bad := range []string{"fe80::1", "fe80::1%eth0", "::1", "::ffff:1.2.3.4", "a b.com", "evil.com;x", "a.com\nb", "a\"b.com", "a=b.com", "a.com/x", "http://a.com", "a.com:80", "1.2.3.4/32", "-a.com", "localhost"} {
		for _, field := range []string{"platform", "tenant", "doku"} {
			f := newSetupFixture(t)
			in := validSetup()
			switch field {
			case "platform":
				in.PlatformHost = bad
			case "tenant":
				in.TenantIsolirHost = bad
			default:
				in.DokuHosts = []string{bad}
			}
			if _, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, in); !errors.Is(err, mikrotik.ErrInvalidInput) {
				t.Errorf("%s %q: err = %v, want invalid input", field, bad, err)
			}
			if f.client.WriteCalls() != 0 {
				t.Fatal("wrote")
			}
		}
	}
	f := newSetupFixture(t)
	in := validSetup()
	in.DokuHosts = []string{"203.0.113.9"}
	f.preview(t, in) // IPv4 literal is fine
}
