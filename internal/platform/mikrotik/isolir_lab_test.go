//go:build mikrotiklab

package mikrotik

import (
	"context"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Lab helpers for the isolir setup runbook (docs/runbook-mikrotik-isolir-setup.md).
// They run against a real RouterOS router and never log secret names,
// passwords or credentials: only counts, versions and zyad-* objects.
//
// Env: MIKROTIK_LAB_HOST/USER/PASS (required), MIKROTIK_LAB_PORT (optional).
// Tests that write to the router also need MIKROTIK_LAB_WRITE=1, and only
// ever touch objects named zyad-* with comment=zyad-managed.
//
// Run: go test -tags mikrotiklab -run TestLabIsolir -v ./internal/platform/mikrotik/

const labManaged = "zyad-managed"

func labEndpoint(t *testing.T) Endpoint {
	t.Helper()
	host, user, pass := os.Getenv("MIKROTIK_LAB_HOST"), os.Getenv("MIKROTIK_LAB_USER"), os.Getenv("MIKROTIK_LAB_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("MIKROTIK_LAB_HOST/USER/PASS not set")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatal("MIKROTIK_LAB_HOST must be an IP address")
	}
	ep := Endpoint{
		Host: host, Username: user, Password: pass,
		Timeout:        15 * time.Second,
		AllowedPrivate: []netip.Prefix{netip.PrefixFrom(addr, addr.BitLen())},
	}
	if p := os.Getenv("MIKROTIK_LAB_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatal("bad MIKROTIK_LAB_PORT")
		}
		ep.Port = n
	}
	return ep
}

func labBinary(t *testing.T) *binaryClient {
	t.Helper()
	return newBinaryClient(labEndpoint(t))
}

func requireLabWrite(t *testing.T) {
	t.Helper()
	if os.Getenv("MIKROTIK_LAB_WRITE") != "1" {
		t.Skip("MIKROTIK_LAB_WRITE=1 not set")
	}
}

func labPrint(t *testing.T, c *binaryClient, path string) []Row {
	t.Helper()
	rows, err := c.Print(context.Background(), path, nil)
	if err != nil {
		t.Logf("%s: error: %v", path, err)
		return nil
	}
	return rows
}

func isZyad(r Row) bool {
	return strings.HasPrefix(r["name"], "zyad-") || strings.HasPrefix(r["list"], "zyad-") ||
		strings.Contains(r["comment"], labManaged) ||
		strings.HasPrefix(r["src-address-list"], "zyad-") || strings.HasPrefix(r["dst-address-list"], "zyad-")
}

// TestLabIsolirDiscover is read-only and logs a high-level picture of the router.
func TestLabIsolirDiscover(t *testing.T) {
	c := labBinary(t)
	for _, r := range labPrint(t, c, "/system/resource") {
		t.Logf("resource: version=%s board=%s arch=%s", r["version"], r["board-name"], r["architecture-name"])
	}
	for _, r := range labPrint(t, c, "/system/routerboard") {
		t.Logf("routerboard: routerboard=%s model=%s", r["routerboard"], r["model"])
	}
	t.Logf("identity rows: %d", len(labPrint(t, c, "/system/identity")))

	profiles := labPrint(t, c, "/ppp/profile")
	names := make([]string, 0, len(profiles))
	for _, r := range profiles {
		names = append(names, r["name"])
	}
	t.Logf("ppp profiles: %d %v", len(profiles), names)
	if len(profiles) > 0 {
		keys := make([]string, 0)
		for k := range profiles[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Logf("ppp profile attributes: %v", keys)
	}
	t.Logf("ppp secrets: %d", len(labPrint(t, c, "/ppp/secret")))
	t.Logf("ppp active sessions: %d", len(labPrint(t, c, "/ppp/active")))
	for _, r := range labPrint(t, c, "/interface/pppoe-server/server") {
		t.Logf("pppoe-server: service=%s interface=%s default-profile=%s disabled=%s", r["service-name"], r["interface"], r["default-profile"], r["disabled"])
	}
	for _, path := range []string{"/ip/firewall/nat", "/ip/firewall/filter", "/ip/firewall/mangle"} {
		rows := labPrint(t, c, path)
		chains := map[string]int{}
		zy := 0
		for i, r := range rows {
			chains[r["chain"]+"/"+r["action"]]++
			if isZyad(r) {
				zy++
			}
			if path == "/ip/firewall/filter" && r["chain"] == "forward" {
				t.Logf("filter forward #%d action=%s disabled=%s matchers=%s", i, r["action"], r["disabled"], matcherKeys(r))
			}
		}
		t.Logf("%s: %d rules, by chain/action %v, zyad-related %d", path, len(rows), chains, zy)
	}
	lists := map[string]int{}
	for _, r := range labPrint(t, c, "/ip/firewall/address-list") {
		lists[r["list"]]++
	}
	t.Logf("address-lists: %v", lists)
	for _, r := range labPrint(t, c, "/ip/service") {
		t.Logf("service %s port=%s disabled=%s", r["name"], r["port"], r["disabled"])
	}
	for _, r := range labPrint(t, c, "/ip/proxy") {
		t.Logf("proxy: enabled=%s port=%s", r["enabled"], r["port"])
	}
	t.Logf("proxy access rules: %d", len(labPrint(t, c, "/ip/proxy/access")))
	for _, r := range labPrint(t, c, "/ip/pool") {
		t.Logf("pool %s", r["name"])
	}
	for _, r := range labPrint(t, c, "/ip/dns") {
		t.Logf("dns: allow-remote-requests=%s", r["allow-remote-requests"])
	}
	t.Logf("hotspot servers: %d", len(labPrint(t, c, "/ip/hotspot")))
	for _, r := range labPrint(t, c, "/file") {
		if strings.HasSuffix(r["name"], ".backup") {
			t.Logf("backup file: %s", r["name"])
		}
	}
	for _, r := range labPrint(t, c, "/user") {
		if r["name"] == os.Getenv("MIKROTIK_LAB_USER") {
			t.Logf("api user group: %s", r["group"])
		}
	}
	for _, r := range labPrint(t, c, "/user/group") {
		t.Logf("user group %s policy=%s", r["name"], r["policy"])
	}
}

func matcherKeys(r Row) string {
	skip := map[string]bool{".id": true, "chain": true, "action": true, "disabled": true, "invalid": true,
		"dynamic": true, "bytes": true, "packets": true, "comment": true, "log": true, "log-prefix": true}
	keys := []string{}
	for k := range r {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// TestLabIsolirLeftovers is read-only and fails if any zyad-* object exists.
func TestLabIsolirLeftovers(t *testing.T) {
	c := labBinary(t)
	paths := []string{"/ppp/profile", "/ppp/secret", "/ip/firewall/address-list", "/ip/firewall/nat",
		"/ip/firewall/filter", "/ip/firewall/mangle", "/ip/proxy/access", "/ip/pool"}
	left := 0
	for _, p := range paths {
		for _, r := range labPrint(t, c, p) {
			if isZyad(r) {
				left++
				t.Errorf("leftover in %s: id=%s name=%s list=%s comment=%s", p, r[".id"], r["name"], r["list"], r["comment"])
			}
		}
	}
	t.Logf("zyad-* leftovers: %d", left)
}

// TestLabIsolirAddressing is read-only: how existing profiles and secrets get
// their addresses (values are pool names / addresses, never secret names).
func TestLabIsolirAddressing(t *testing.T) {
	c := labBinary(t)
	for _, r := range labPrint(t, c, "/ppp/profile") {
		t.Logf("profile %q local=%q remote=%q rate=%q dns=%q address-list=%q only-one=%q", r["name"], r["local-address"], r["remote-address"], r["rate-limit"], r["dns-server"], r["address-list"], r["only-one"])
	}
	byProfile := map[string]int{}
	withRemote, disabled, services := 0, 0, map[string]int{}
	for _, r := range labPrint(t, c, "/ppp/secret") {
		byProfile[r["profile"]]++
		services[r["service"]]++
		if r["remote-address"] != "" {
			withRemote++
		}
		if r["disabled"] == "true" {
			disabled++
		}
	}
	t.Logf("secrets by profile %v, by service %v, with remote-address %d, disabled %d", byProfile, services, withRemote, disabled)
	for _, r := range labPrint(t, c, "/ip/pool") {
		t.Logf("pool %q ranges=%q", r["name"], r["ranges"])
	}
	for _, r := range labPrint(t, c, "/ip/firewall/filter") {
		if r["chain"] == "forward" || r["chain"] == "input" {
			t.Logf("filter id=%s chain=%s action=%s disabled=%s src-list=%q dst-port=%q in-if=%q", r[".id"], r["chain"], r["action"], r["disabled"], r["src-address-list"], r["dst-port"], r["in-interface"])
		}
	}
	for _, r := range labPrint(t, c, "/ip/firewall/nat") {
		if r["chain"] == "dstnat" || r["chain"] == "srcnat" {
			t.Logf("nat id=%s chain=%s action=%s disabled=%s matchers=%s", r[".id"], r["chain"], r["action"], r["disabled"], matcherKeys(r))
		}
	}
	for _, r := range labPrint(t, c, "/ip/proxy") {
		t.Logf("proxy: %v", r)
	}
	for _, r := range labPrint(t, c, "/ip/proxy/access") {
		t.Logf("proxy access: action=%s disabled=%s matchers=%s", r["action"], r["disabled"], matcherKeys(r))
	}
	for _, r := range labPrint(t, c, "/ip/dns") {
		t.Logf("dns servers=%q dynamic=%q", r["servers"], r["dynamic-servers"])
	}
	for _, r := range labPrint(t, c, "/ip/firewall/connection/tracking") {
		t.Logf("conntrack enabled=%s", r["enabled"])
	}
}

// TestLabIsolirBackup saves a router backup before any write. It refuses to
// overwrite an existing file. Env MIKROTIK_LAB_BACKUP_NAME sets the name.
func TestLabIsolirBackup(t *testing.T) {
	requireLabWrite(t)
	c := labBinary(t)
	name := os.Getenv("MIKROTIK_LAB_BACKUP_NAME")
	if !strings.HasPrefix(name, "pre-zyad-isolir") {
		t.Fatal("MIKROTIK_LAB_BACKUP_NAME must start with pre-zyad-isolir")
	}
	for _, r := range labPrint(t, c, "/file") {
		if r["name"] == name+".backup" {
			t.Fatalf("backup %s.backup already exists", name)
		}
	}
	if _, err := c.run(context.Background(), "/system/backup/save", "=name="+name); err != nil {
		t.Fatalf("backup save: %v", err)
	}
	time.Sleep(3 * time.Second)
	for _, r := range labPrint(t, c, "/file") {
		if r["name"] == name+".backup" {
			t.Logf("backup saved: %s size=%s", r["name"], r["size"])
			return
		}
	}
	t.Fatal("backup file not found after save")
}

// TestLabIsolirTransports checks REST against this router, Detect ordering and
// a zyad-managed profile lifecycle over the binary API.
func TestLabIsolirTransports(t *testing.T) {
	ep := labEndpoint(t)
	ctx := context.Background()

	for _, port := range []int{0, 80, 443} {
		rep := ep
		rep.Port = port
		v, err := newRESTClient(rep).Version(ctx)
		t.Logf("REST Version port=%d: version=%q err=%v", port, v, err)
	}
	for _, port := range []int{0, 8728} {
		dep := ep
		dep.Port = port
		dctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		start := time.Now()
		apiType, v, err := Detect(dctx, dep)
		cancel()
		t.Logf("Detect port=%d: api_type=%q version=%q err=%v elapsed=%s", port, apiType, v, err, time.Since(start).Round(time.Millisecond))
	}

	requireLabWrite(t)
	c := newBinaryClient(ep)
	v, err := c.Version(ctx)
	if err != nil {
		t.Fatalf("binary version: %v", err)
	}
	t.Logf("binary Version: %s", v)
	rows, err := c.Print(ctx, "/ppp/profile", nil)
	if err != nil || len(rows) == 0 {
		t.Fatalf("print profiles: %v", err)
	}
	id, err := c.Add(ctx, "/ppp/profile", Row{"name": "zyad-labtest", "comment": labManaged})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Cleanup(func() { _ = c.Remove(context.Background(), "/ppp/profile", id) })
	if err := c.Set(ctx, "/ppp/profile", id, Row{"rate-limit": "1M/1M"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Print(ctx, "/ppp/profile", Row{"name": "zyad-labtest"})
	if err != nil || len(got) != 1 || got[0]["rate-limit"] != "1M/1M" || got[0]["comment"] != labManaged {
		t.Fatalf("print after set: %v %v", err, got)
	}
	if err := c.Remove(ctx, "/ppp/profile", id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ = c.Print(ctx, "/ppp/profile", Row{"name": "zyad-labtest"})
	if len(got) != 0 {
		t.Fatalf("profile still present after remove")
	}
	t.Log("binary Add/Set/Print/Remove zyad-labtest: OK")
}

type labCreated struct{ path, id, what string }

// TestLabIsolirMechanism creates the hypothesised isolir objects (all zyad-*,
// comment=zyad-managed), inspects how the router stores them, then removes
// them. Members of address-list zyad-isolir only appear when a PPP session
// uses profile zyad-isolir; the test secret is disabled, so every rule here
// matches nothing on the live router.
func TestLabIsolirMechanism(t *testing.T) {
	requireLabWrite(t)
	c := labBinary(t)
	ctx := context.Background()
	var created []labCreated
	t.Cleanup(func() {
		for i := len(created) - 1; i >= 0; i-- {
			o := created[i]
			if err := c.Remove(context.Background(), o.path, o.id); err != nil {
				t.Errorf("cleanup %s %s (%s): %v", o.path, o.id, o.what, err)
			}
		}
		t.Logf("cleanup: removed %d objects", len(created))
	})
	add := func(path, what string, attrs Row) string {
		attrs["comment"] = labManaged
		id, err := c.Add(ctx, path, attrs)
		if err != nil {
			t.Logf("ADD FAIL %s (%s): %v", path, what, err)
			return ""
		}
		created = append(created, labCreated{path, id, what})
		t.Logf("ADD OK %s (%s) id=%s", path, what, id)
		return id
	}

	// (a) pool + isolir profile.
	add("/ip/pool", "isolir pool", Row{"name": "zyad-isolir-pool", "ranges": "10.255.254.2-10.255.254.254"})
	add("/ppp/profile", "isolir profile", Row{
		"name": "zyad-isolir", "address-list": "zyad-isolir",
		"local-address": "10.255.254.1", "remote-address": "zyad-isolir-pool",
		"dns-server": "10.255.254.1", "rate-limit": "512k/512k", "only-one": "yes",
	})
	for _, r := range labPrint(t, c, "/ppp/profile") {
		if r["name"] == "zyad-isolir" {
			t.Logf("profile stored: address-list=%q local=%q remote=%q dns=%q rate=%q only-one=%q comment=%q",
				r["address-list"], r["local-address"], r["remote-address"], r["dns-server"], r["rate-limit"], r["only-one"], r["comment"])
		}
	}

	// (c) allowlist (FQDN entries) and forward filter rules placed before the
	// first existing forward rule.
	for _, host := range []string{"zyad.online", "api-sandbox.doku.com", "sandbox.doku.com"} {
		add("/ip/firewall/address-list", "allow fqdn "+host, Row{"list": "zyad-isolir-allow", "address": host})
	}
	add("/ip/firewall/address-list", "allow static ip", Row{"list": "zyad-isolir-allow", "address": "192.0.2.10"})
	firstForward := ""
	for _, r := range labPrint(t, c, "/ip/firewall/filter") {
		if r["chain"] == "forward" {
			firstForward = r[".id"]
			break
		}
	}
	t.Logf("first existing forward rule id=%s", firstForward)
	place := func(r Row) Row {
		if firstForward != "" {
			r["place-before"] = firstForward
		}
		return r
	}
	add("/ip/firewall/filter", "accept dns udp", place(Row{"chain": "forward", "src-address-list": "zyad-isolir", "protocol": "udp", "dst-port": "53", "action": "accept"}))
	add("/ip/firewall/filter", "accept dns tcp", place(Row{"chain": "forward", "src-address-list": "zyad-isolir", "protocol": "tcp", "dst-port": "53", "action": "accept"}))
	add("/ip/firewall/filter", "accept allowlist", place(Row{"chain": "forward", "src-address-list": "zyad-isolir", "dst-address-list": "zyad-isolir-allow", "action": "accept"}))
	add("/ip/firewall/filter", "drop rest", place(Row{"chain": "forward", "src-address-list": "zyad-isolir", "action": "drop"}))

	// (b1) redirect via dst-nat to an external redirect host (TEST-NET address).
	add("/ip/firewall/nat", "dst-nat http to redirect host", Row{
		"chain": "dstnat", "src-address-list": "zyad-isolir", "dst-address-list": "!zyad-isolir-allow",
		"protocol": "tcp", "dst-port": "80", "action": "dst-nat", "to-addresses": "192.0.2.10", "to-ports": "80",
	})
	// (b2) redirect into the router web proxy (not enabled by this test).
	add("/ip/firewall/nat", "redirect http to web-proxy", Row{
		"chain": "dstnat", "src-address-list": "zyad-isolir", "dst-address-list": "!zyad-isolir-allow",
		"protocol": "tcp", "dst-port": "80", "action": "redirect", "to-ports": "8080", "disabled": "yes",
	})
	add("/ip/proxy/access", "proxy allow platform", Row{"src-address": "10.255.254.0/24", "dst-host": "zyad.online", "action": "allow"})
	add("/ip/proxy/access", "proxy redirect v7 syntax", Row{"src-address": "10.255.254.0/24", "action": "redirect", "action-data": "zyad.online/isolir"})
	add("/ip/proxy/access", "proxy deny redirect-to v6 syntax", Row{"src-address": "10.255.254.0/24", "action": "deny", "redirect-to": "zyad.online/isolir"})

	// Order of the forward chain after placement (ids only, no foreign details).
	order := []string{}
	for _, r := range labPrint(t, c, "/ip/firewall/filter") {
		if r["chain"] == "forward" {
			tag := r[".id"]
			if r["comment"] == labManaged {
				tag += "(zyad:" + r["action"] + ")"
			}
			order = append(order, tag)
		}
	}
	t.Logf("forward chain order: %v", order)

	// (d) test secret: disabled, never usable for a login.
	secretID := add("/ppp/secret", "test secret", Row{"name": "zyad-labtest-user", "password": "zyad-" + strconv.FormatInt(time.Now().UnixNano(), 36), "service": "pppoe", "profile": "default", "disabled": "yes"})
	if secretID != "" {
		if err := c.Set(ctx, "/ppp/secret", secretID, Row{"profile": "zyad-isolir"}); err != nil {
			t.Errorf("isolate set profile: %v", err)
		}
		got, _ := c.Print(ctx, "/ppp/secret", Row{"name": "zyad-labtest-user"})
		if len(got) == 1 {
			t.Logf("after isolate: profile=%q", got[0]["profile"])
		}
		active, err := c.Print(ctx, "/ppp/active", Row{"name": "zyad-labtest-user"})
		t.Logf("active sessions for test secret: %d err=%v", len(active), err)
		if err := c.Remove(ctx, "/ppp/active", "*FFFFFF"); err != nil {
			t.Logf("remove non-existent active id: %v", err)
		}
		if err := c.Set(ctx, "/ppp/secret", secretID, Row{"profile": "default"}); err != nil {
			t.Errorf("restore set profile: %v", err)
		}
		got, _ = c.Print(ctx, "/ppp/secret", Row{"name": "zyad-labtest-user"})
		if len(got) == 1 {
			t.Logf("after restore: profile=%q", got[0]["profile"])
		}
	}

	// Profile still referenced by a secret: can it be removed? (cleanup order).
	time.Sleep(5 * time.Second)
	for _, r := range labPrint(t, c, "/ip/firewall/address-list") {
		if r["list"] == "zyad-isolir-allow" {
			t.Logf("allow entry address=%q dynamic=%q disabled=%q comment=%q", r["address"], r["dynamic"], r["disabled"], r["comment"])
		}
	}
	for _, r := range labPrint(t, c, "/ip/firewall/nat") {
		if r["comment"] == labManaged {
			t.Logf("nat stored: action=%s disabled=%s dst-address-list=%q to=%s:%s", r["action"], r["disabled"], r["dst-address-list"], r["to-addresses"], r["to-ports"])
		}
	}
	for _, r := range labPrint(t, c, "/ip/proxy/access") {
		if r["comment"] == labManaged {
			t.Logf("proxy access stored: action=%s action-data=%q redirect-to=%q", r["action"], r["action-data"], r["redirect-to"])
		}
	}
	for _, r := range labPrint(t, c, "/ip/proxy") {
		t.Logf("proxy still enabled=%s", r["enabled"])
	}
}
