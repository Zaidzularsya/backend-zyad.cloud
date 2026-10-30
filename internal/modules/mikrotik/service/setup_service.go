package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// setupWriteTimeout bounds the write phase of Apply and all of Cleanup. These
// run under context.WithoutCancel so a client or proxy disconnect cannot leave
// the router half-configured; the timeout still prevents a hung router from
// holding the request forever. Preview is read-only and keeps the request ctx.
const setupWriteTimeout = 3 * time.Minute

func writeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), setupWriteTimeout)
}

// ErrPlanChanged is returned by Apply when the freshly computed plan differs
// from the previewed one. Alias of mikrotik.ErrPlanChanged.
var ErrPlanChanged = mikrotik.ErrPlanChanged

// Object names and markers fixed by docs/runbook-mikrotik-isolir-setup.md.
const (
	setupManaged       = "zyad-managed"
	setupPoolName      = "zyad-isolir-pool"
	setupDefaultProf   = "zyad-isolir"
	setupIsolirList    = "zyad-isolir"
	setupAllowList     = "zyad-isolir-allow"
	pathIPPool         = "/ip/pool"
	pathIPAddress      = "/ip/address"
	pathPPPProfile     = "/ppp/profile"
	pathPPPSecret      = "/ppp/secret"
	pathAddressList    = "/ip/firewall/address-list"
	pathFirewallFilter = "/ip/firewall/filter"
	pathFirewallNAT    = "/ip/firewall/nat"
	pathProxyAccess    = "/ip/proxy/access"

	redirectNote = "Optional/unproven: HTTP redirect needs /ip proxy enabled, a global router " +
		"setting this setup never changes. Enable it yourself only after verifying with a real CPE; " +
		"HTTPS cannot be redirected and the proxy access rule uses RouterOS v7 action-data."
)

var (
	profileNamePattern = regexp.MustCompile(`^zyad-[A-Za-z0-9][A-Za-z0-9._-]{0,57}$`)
	rateLimitPattern   = regexp.MustCompile(`^[0-9]+[kKmMgG]?(/[0-9]+[kKmMgG]?)?$`)
	fqdnPattern        = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// SetupInput is the operator-chosen part of the runbook placeholder table.
// The pool CIDR yields {ISOLIR_SUBNET}, {ISOLIR_LOCAL_ADDRESS} (first host) and
// {ISOLIR_POOL_RANGE} (second host .. last host).
type SetupInput struct {
	// ProfileName defaults to "zyad-isolir"; must start with "zyad-".
	ProfileName string
	// PoolCIDR is the isolir subnet, e.g. 10.255.254.0/24 (private IPv4, /16../29,
	// must not overlap an existing pool or router address).
	PoolCIDR string
	// RateLimit is optional, e.g. "512k/512k".
	RateLimit string
	// PlatformHost is required; TenantIsolirHost and DokuHosts are optional
	// FQDNs added to the allowlist address-list.
	PlatformHost     string
	TenantIsolirHost string
	DokuHosts        []string
	// RedirectURL is optional; empty means no redirect objects are planned.
	RedirectURL string
}

type normalizedSetup struct {
	profile      string
	subnet       netip.Prefix
	localAddress string
	poolRange    string
	rateLimit    string
	allowHosts   []string
	platformHost string
	redirectURL  string
	redirectData string
}

// SetupLocker serializes Apply and Cleanup per router across processes
// (repository.AdvisoryLocker in production). TryLock never waits: ok=false
// means another holder has the key. When ok, unlock must be called once.
type SetupLocker interface {
	TryLock(ctx context.Context, key string) (unlock func(), ok bool, err error)
}

// SetupService plans, applies and removes the isolir objects on a router.
type SetupService struct {
	routers *RouterService
	repo    repository.RouterRepository
	locker  SetupLocker
}

func NewSetupService(routers *RouterService, repo repository.RouterRepository, locker SetupLocker) *SetupService {
	return &SetupService{routers: routers, repo: repo, locker: locker}
}

// lockSetup takes the per-router setup lock. Without it a Cleanup could list
// and remove objects while an Apply that saw them as existing marks the router
// ready (isolir_ready=true with the objects gone). Preview does not lock.
func (s *SetupService) lockSetup(ctx context.Context, routerID string) (func(), error) {
	if !validID(routerID) {
		return nil, mikrotik.ErrRouterNotFound
	}
	unlock, ok, err := s.locker.TryLock(ctx, "mikrotik.setup:"+routerID)
	if err != nil {
		slog.Warn("mikrotik: acquire setup lock failed", "router_id", routerID, "error", scrub(err.Error()))
		return nil, errors.New("acquire setup lock failed")
	}
	if !ok {
		return nil, mikrotik.ErrSetupInProgress
	}
	return unlock, nil
}

// ---- input validation --------------------------------------------------

func normalizeHost(label, h string) (string, error) {
	h = strings.ToLower(strings.TrimSpace(h))
	if len(h) > 253 {
		return "", invalid(label + " is too long")
	}
	if a, err := netip.ParseAddr(h); err == nil {
		// IPv4 literals only: the allowlist is an IPv4 address-list and the
		// router would reject IPv6 or zoned addresses mid-apply.
		if !a.Is4() {
			return "", invalid(label + " must be an IPv4 address or hostname")
		}
		return h, nil
	}
	if !fqdnPattern.MatchString(h) {
		return "", invalid(label + " must be a bare hostname such as example.com")
	}
	return h, nil
}

func normalizeSetup(in SetupInput) (normalizedSetup, error) {
	var n normalizedSetup
	n.profile = strings.TrimSpace(in.ProfileName)
	if n.profile == "" {
		n.profile = setupDefaultProf
	}
	if !profileNamePattern.MatchString(n.profile) {
		return n, invalid("profile name must start with zyad- and use letters, digits, '.', '_' or '-'")
	}
	n.rateLimit = strings.TrimSpace(in.RateLimit)
	if n.rateLimit != "" && !rateLimitPattern.MatchString(n.rateLimit) {
		return n, invalid("rate limit must look like 512k/512k")
	}

	prefix, err := netip.ParsePrefix(strings.TrimSpace(in.PoolCIDR))
	switch {
	case err != nil:
		return n, invalid("pool_cidr must be a CIDR such as 10.255.254.0/24")
	case !prefix.Addr().Is4():
		return n, invalid("pool_cidr must be IPv4")
	case prefix.Masked() != prefix:
		return n, invalid("pool_cidr must be a network address (host bits zero)")
	case !prefix.Addr().IsPrivate():
		return n, invalid("pool_cidr must be inside private address space (10/8, 172.16/12, 192.168/16)")
	case prefix.Bits() < 16 || prefix.Bits() > 29:
		return n, invalid("pool_cidr prefix length must be between /16 and /29")
	}
	n.subnet = prefix
	first := prefix.Addr()
	n.localAddress = first.Next().String()
	last := lastAddr(prefix)
	n.poolRange = first.Next().Next().String() + "-" + last.Prev().String()

	if strings.TrimSpace(in.PlatformHost) == "" {
		return n, invalid("platform_host is required")
	}
	if n.platformHost, err = normalizeHost("platform_host", in.PlatformHost); err != nil {
		return n, err
	}
	seen := map[string]bool{n.platformHost: true}
	n.allowHosts = []string{n.platformHost}
	if strings.TrimSpace(in.TenantIsolirHost) != "" {
		h, err := normalizeHost("tenant_isolir_host", in.TenantIsolirHost)
		if err != nil {
			return n, err
		}
		if !seen[h] {
			seen[h] = true
			n.allowHosts = append(n.allowHosts, h)
		}
	}
	var doku []string
	for _, raw := range in.DokuHosts {
		h, err := normalizeHost("doku_hosts entry", raw)
		if err != nil {
			return n, err
		}
		if !seen[h] {
			seen[h] = true
			doku = append(doku, h)
		}
	}
	sort.Strings(doku)
	n.allowHosts = append(n.allowHosts, doku...)

	if raw := strings.TrimSpace(in.RedirectURL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
			u.RawQuery != "" || u.Fragment != "" || len(raw) > 255 || strings.ContainsAny(raw, " \t\r\n") {
			return n, invalid("redirect_url must be an http(s) URL without credentials, query or fragment")
		}
		n.redirectURL = raw
		n.redirectData = u.Host + strings.TrimRight(u.EscapedPath(), "/")
	}
	return n, nil
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As4()
	host := 32 - p.Bits()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	v |= (uint32(1) << host) - 1
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// ---- matching helpers ---------------------------------------------------

func rowManaged(r pmk.Row) bool { return r["comment"] == setupManaged }

func rowDynamic(r pmk.Row) bool { return r["dynamic"] == "true" || r["dynamic"] == "yes" }

// rowDisabled normalizes the per-path shapes of "disabled" (runbook 3.5): a
// missing key, "", false and no all mean enabled.
func rowDisabled(r pmk.Row) bool { return r["disabled"] == "true" || r["disabled"] == "yes" }

// objSpec describes one router object by the runbook match keys.
type objSpec struct {
	kind              string // label used in conflict errors
	path              string
	name              string
	match             pmk.Row  // key attributes that must be equal
	absent            []string // attributes that must be empty
	compare           pmk.Row  // attributes compared after a match; empty value = must be unset
	conflictUnmanaged bool     // an unmanaged row with the same key is a refusal
	placeBefore       string
	optional          bool
	note              string
}

func (sp objSpec) matches(r pmk.Row) bool {
	for k, v := range sp.match {
		if r[k] != v {
			return false
		}
	}
	for _, k := range sp.absent {
		if r[k] != "" {
			return false
		}
	}
	return true
}

// find returns the index of the zyad-managed row matching sp, or -1. An
// unmanaged row with the same key aborts with SetupConflictError when
// conflictUnmanaged is set. Dynamic rows are ignored.
func (sp objSpec) find(rows []pmk.Row) (int, error) {
	idx := -1
	for i, r := range rows {
		if rowDynamic(r) || !sp.matches(r) {
			continue
		}
		if rowManaged(r) {
			if idx < 0 {
				idx = i
			}
			continue
		}
		if sp.conflictUnmanaged {
			return -1, &mikrotik.SetupConflictError{
				Kind: sp.kind, Name: sp.name,
				Detail: "an object with this name already exists and is not managed by zyad; rename or remove it manually",
			}
		}
	}
	return idx, nil
}

func (sp objSpec) desired() pmk.Row {
	d := pmk.Row{"comment": setupManaged}
	for k, v := range sp.match {
		d[k] = v
	}
	for k, v := range sp.compare {
		if v != "" {
			d[k] = v
		}
	}
	return d
}

func (sp objSpec) build(rows []pmk.Row, idx int) domain.SetupAction {
	a := domain.SetupAction{Path: sp.path, Match: copyRow(sp.match), Optional: sp.optional, Note: sp.note}
	attrs := sp.desired()
	if idx < 0 {
		a.Kind = domain.SetupCreate
		if sp.placeBefore != "" {
			attrs["place-before"] = sp.placeBefore
		}
		a.Attrs = attrs
		return a
	}
	row := rows[idx]
	a.ID = row[".id"]
	differs := rowDisabled(row)
	for k, v := range sp.compare {
		if row[k] != v {
			differs = true
		}
	}
	if !differs {
		a.Kind, a.Attrs = domain.SetupExists, attrs
		return a
	}
	a.Kind = domain.SetupUpdate
	for k, v := range sp.compare {
		attrs[k] = v // empty values clear the attribute on Set
	}
	if rowDisabled(row) {
		attrs["disabled"] = "no"
	}
	a.Attrs = attrs
	return a
}

func copyRow(r pmk.Row) pmk.Row {
	out := make(pmk.Row, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// filterAbsent lists attributes a forward/NAT rule must not have so that only
// the exact runbook tuple matches.
var filterAbsent = []string{
	"src-address", "dst-address", "src-address-list", "dst-address-list", "protocol", "dst-port", "src-port",
	"in-interface", "out-interface", "in-interface-list", "out-interface-list", "connection-state",
	"connection-mark", "to-addresses", "to-ports",
}

func without(list []string, drop ...string) []string {
	var out []string
	for _, k := range list {
		keep := true
		for _, d := range drop {
			if k == d {
				keep = false
			}
		}
		if keep {
			out = append(out, k)
		}
	}
	return out
}

func forwardSpecs() []objSpec {
	mk := func(name string, match pmk.Row) objSpec {
		match["chain"] = "forward"
		match["src-address-list"] = setupIsolirList
		keys := []string{}
		for k := range match {
			keys = append(keys, k)
		}
		return objSpec{kind: "firewall filter rule", path: pathFirewallFilter, name: name, match: match, absent: without(filterAbsent, keys...)}
	}
	return []objSpec{
		mk("F1", pmk.Row{"action": "accept", "protocol": "udp", "dst-port": "53"}),
		mk("F2", pmk.Row{"action": "accept", "protocol": "tcp", "dst-port": "53"}),
		mk("F3", pmk.Row{"action": "accept", "dst-address-list": setupAllowList}),
		mk("F4", pmk.Row{"action": "drop"}),
	}
}

// ---- planning -------------------------------------------------------------

func conflictOrder(detail string) error {
	return &mikrotik.SetupConflictError{Kind: "firewall rule order", Detail: detail + "; run cleanup, then apply again"}
}

// plan reads the router (read-only) and returns the ordered actions.
func (s *SetupService) plan(ctx context.Context, c pmk.RouterClient, n normalizedSetup) (domain.SetupPlan, error) {
	var actions []domain.SetupAction

	// 1. Pool: refuse unmanaged same-name pool, reject overlap.
	pools, err := c.Print(ctx, pathIPPool, nil)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	poolSpec := objSpec{
		kind: "ip pool", path: pathIPPool, name: setupPoolName, match: pmk.Row{"name": setupPoolName},
		compare: pmk.Row{"ranges": n.poolRange}, conflictUnmanaged: true,
	}
	poolIdx, err := poolSpec.find(pools)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	addrs, err := c.Print(ctx, pathIPAddress, nil)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	if err := checkOverlap(n.subnet, pools, poolIdx, addrs); err != nil {
		return domain.SetupPlan{}, err
	}
	actions = append(actions, poolSpec.build(pools, poolIdx))

	// 2. Profile.
	profiles, err := c.Print(ctx, pathPPPProfile, pmk.Row{"name": n.profile})
	if err != nil {
		return domain.SetupPlan{}, err
	}
	profSpec := objSpec{
		kind: "ppp profile", path: pathPPPProfile, name: n.profile, match: pmk.Row{"name": n.profile},
		compare: pmk.Row{
			"address-list": setupIsolirList, "local-address": n.localAddress, "remote-address": setupPoolName,
			"dns-server": n.localAddress, "rate-limit": n.rateLimit, "only-one": "yes",
		},
		conflictUnmanaged: true,
	}
	profIdx, err := profSpec.find(profiles)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	actions = append(actions, profSpec.build(profiles, profIdx))

	// 3. Allowlist entries.
	entries, err := c.Print(ctx, pathAddressList, pmk.Row{"list": setupAllowList})
	if err != nil {
		return domain.SetupPlan{}, err
	}
	for _, e := range entries {
		if e["list"] == setupAllowList && !rowDynamic(e) && !rowManaged(e) {
			return domain.SetupPlan{}, &mikrotik.SetupConflictError{
				Kind: "address-list", Name: setupAllowList,
				Detail: "the allowlist already has entries not managed by zyad; remove or move them manually",
			}
		}
	}
	for _, h := range n.allowHosts {
		sp := objSpec{
			kind: "address-list", path: pathAddressList, name: h,
			match: pmk.Row{"list": setupAllowList, "address": h}, conflictUnmanaged: true,
		}
		idx, err := sp.find(entries)
		if err != nil {
			return domain.SetupPlan{}, err
		}
		actions = append(actions, sp.build(entries, idx))
	}

	// 4. Forward rules F1..F4, order-preserving.
	fwd, err := c.Print(ctx, pathFirewallFilter, pmk.Row{"chain": "forward"})
	if err != nil {
		return domain.SetupPlan{}, err
	}
	fwdActions, err := planForward(fwd)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	actions = append(actions, fwdActions...)

	// 5. Optional redirect (runbook 4.5B), only when requested.
	if n.redirectURL != "" {
		ra, err := s.planRedirect(ctx, c, n)
		if err != nil {
			return domain.SetupPlan{}, err
		}
		actions = append(actions, ra...)
	}
	return finalizePlan(actions), nil
}

func planForward(fwd []pmk.Row) ([]domain.SetupAction, error) {
	specs := forwardSpecs()
	found := make([]int, len(specs))
	for i, sp := range specs {
		idx, err := sp.find(fwd)
		if err != nil {
			return nil, err
		}
		found[i] = idx
	}
	// The first forward rule that is not zyad-managed ({FIRST_FORWARD_ID}).
	firstForeign := ""
	for _, r := range fwd {
		// Dynamic rows (e.g. the fasttrack "special dummy rule") are never anchors.
		if !rowManaged(r) && !rowDynamic(r) {
			firstForeign = r[".id"]
			break
		}
	}
	lo, hi, last := -1, -1, -1
	for _, idx := range found {
		if idx < 0 {
			continue
		}
		if idx < last {
			return nil, conflictOrder("zyad forward rules are in the wrong relative order")
		}
		last = idx
		if lo < 0 {
			lo = idx
		}
		hi = idx
	}
	if lo >= 0 {
		for i := 0; i <= hi; i++ {
			if !rowManaged(fwd[i]) && !rowDynamic(fwd[i]) {
				return nil, conflictOrder("a non-zyad forward rule sits before or between zyad rules")
			}
		}
	}
	out := make([]domain.SetupAction, 0, len(specs))
	for i := range specs {
		if found[i] < 0 {
			switch next := nextFound(found, i); {
			case next >= 0:
				specs[i].placeBefore = fwd[next][".id"]
			default:
				specs[i].placeBefore = firstForeign
			}
		}
		out = append(out, specs[i].build(fwd, found[i]))
	}
	return out, nil
}

func nextFound(found []int, i int) int {
	for j := i + 1; j < len(found); j++ {
		if found[j] >= 0 {
			return found[j]
		}
	}
	return -1
}

func (s *SetupService) planRedirect(ctx context.Context, c pmk.RouterClient, n normalizedSetup) ([]domain.SetupAction, error) {
	nat, err := c.Print(ctx, pathFirewallNAT, pmk.Row{"chain": "dstnat"})
	if err != nil {
		return nil, err
	}
	nb := objSpec{
		kind: "nat rule", path: pathFirewallNAT, name: "NB", optional: true, note: redirectNote,
		match: pmk.Row{
			"chain": "dstnat", "action": "redirect", "src-address-list": setupIsolirList,
			"dst-address-list": "!" + setupAllowList, "protocol": "tcp", "dst-port": "80", "to-ports": "8080",
		},
	}
	nb.absent = without(filterAbsent, "src-address-list", "dst-address-list", "protocol", "dst-port", "to-ports")
	nbIdx, err := nb.find(nat)
	if err != nil {
		return nil, err
	}

	proxy, err := c.Print(ctx, pathProxyAccess, nil)
	if err != nil {
		return nil, err
	}
	pa := objSpec{
		kind: "proxy access rule", path: pathProxyAccess, name: "PA", optional: true, note: redirectNote,
		match: pmk.Row{"src-address": n.subnet.String(), "dst-host": n.platformHost, "action": "allow"},
	}
	pr := objSpec{
		kind: "proxy access rule", path: pathProxyAccess, name: "PR", optional: true, note: redirectNote,
		match:   pmk.Row{"src-address": n.subnet.String(), "action": "redirect"},
		compare: pmk.Row{"action-data": n.redirectData},
	}
	paIdx, err := pa.find(proxy)
	if err != nil {
		return nil, err
	}
	prIdx, err := pr.find(proxy)
	if err != nil {
		return nil, err
	}
	if paIdx >= 0 && prIdx >= 0 && paIdx > prIdx {
		return nil, conflictOrder("proxy access rules are in the wrong relative order")
	}
	if paIdx < 0 && prIdx >= 0 {
		pa.placeBefore = proxy[prIdx][".id"]
	}
	return []domain.SetupAction{nb.build(nat, nbIdx), pa.build(proxy, paIdx), pr.build(proxy, prIdx)}, nil
}

func finalizePlan(actions []domain.SetupAction) domain.SetupPlan {
	if actions == nil {
		actions = []domain.SetupAction{}
	}
	// encoding/json sorts map keys, so the encoding is canonical; ID is excluded.
	raw, _ := json.Marshal(actions)
	sum := sha256.Sum256(raw)
	return domain.SetupPlan{Actions: actions, Hash: hex.EncodeToString(sum[:])}
}

// ipRange is an inclusive IPv4 interval.
type ipRange struct{ lo, hi netip.Addr }

func (r ipRange) overlaps(o ipRange) bool { return r.lo.Compare(o.hi) <= 0 && o.lo.Compare(r.hi) <= 0 }

func prefixRange(p netip.Prefix) ipRange {
	p = p.Masked()
	return ipRange{lo: p.Addr(), hi: lastAddr(p)}
}

// parsePoolRanges understands "a-b", a single address, a CIDR and comma lists.
// Tokens that are not IPv4 (or unparsable) cannot overlap an IPv4 subnet and are skipped.
func parsePoolRanges(s string) []ipRange {
	var out []ipRange
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		switch {
		case tok == "":
		case strings.Contains(tok, "/"):
			if p, err := netip.ParsePrefix(tok); err == nil && p.Addr().Is4() {
				out = append(out, prefixRange(p))
			}
		case strings.Contains(tok, "-"):
			parts := strings.SplitN(tok, "-", 2)
			lo, e1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
			hi, e2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
			if e1 == nil && e2 == nil && lo.Is4() && hi.Is4() {
				out = append(out, ipRange{lo, hi})
			}
		default:
			if a, err := netip.ParseAddr(tok); err == nil && a.Is4() {
				out = append(out, ipRange{a, a})
			}
		}
	}
	return out
}

func checkOverlap(subnet netip.Prefix, pools []pmk.Row, ownPool int, addrs []pmk.Row) error {
	want := prefixRange(subnet)
	for i, p := range pools {
		if i == ownPool {
			continue // our own pool is rewritten, not competing
		}
		for _, r := range parsePoolRanges(p["ranges"]) {
			if r.overlaps(want) {
				return invalid("pool_cidr overlaps an existing address pool on the router")
			}
		}
	}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a["address"]); err == nil && p.Addr().Is4() && prefixRange(p).overlaps(want) {
			return invalid("pool_cidr overlaps an address configured on the router")
		}
	}
	return nil
}

// ---- public operations -----------------------------------------------------

// Preview computes the plan from the current router state without writing.
func (s *SetupService) Preview(ctx context.Context, scope coretenant.Scope, routerID string, in SetupInput) (domain.SetupPlan, error) {
	n, err := normalizeSetup(in)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	_, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return domain.SetupPlan{}, err
	}
	plan, err := s.plan(ctx, client, n)
	if err != nil {
		return domain.SetupPlan{}, planError(err, secrets)
	}
	return plan, nil
}

// planError keeps typed setup errors and maps transport errors; anything else
// is treated like an unclassified router error (logged scrubbed, fixed text).
func planError(err error, secrets []string) error {
	var conflict *mikrotik.SetupConflictError
	var invalidErr *mikrotik.InvalidInputError
	if errors.As(err, &conflict) || errors.As(err, &invalidErr) {
		return err
	}
	return translateClientError(err, secrets...)
}

// stepCause classifies a failed write: transport errors keep their mapped
// sentinel, everything else becomes ErrSetupFailed with the raw text only in
// the (scrubbed) log.
func stepCause(err error, secrets []string) error {
	var tlsErr *pmk.TLSUntrustedError
	switch {
	case errors.As(err, &tlsErr), errors.Is(err, pmk.ErrHostNotAllowed), errors.Is(err, pmk.ErrAuthFailed), errors.Is(err, pmk.ErrUnreachable):
		return translateClientError(err, secrets...)
	}
	slog.Warn("mikrotik: setup step rejected by router", "error", scrub(err.Error(), secrets...), "type", fmt.Sprintf("%T", err))
	return mikrotik.ErrSetupFailed
}

func (s *SetupService) audit(ctx context.Context, scope coretenant.Scope, event, actor string, r domain.Router, extra map[string]any) {
	md := auditMetadata(r)
	for k, v := range extra {
		md[k] = v
	}
	if err := s.repo.InsertAudit(ctx, scope, repository.AuditEntry{Event: event, ActorUserID: actor, TargetID: r.ID, Metadata: md}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "router_id", r.ID, "event", event, "error", scrub(err.Error()))
	}
}

// Apply holds the per-router setup lock for its whole run (ErrSetupInProgress
// when another Apply/Cleanup holds it), recomputes the plan from the current router state, refuses with
// ErrPlanChanged when it differs from the previewed planHash (writing nothing),
// executes the actions in order, verifies by re-planning and marks the router
// isolir-ready. Fail closed: a router that is already ready is marked not
// ready before the first write, and ready again only after verification.
func (s *SetupService) Apply(ctx context.Context, scope coretenant.Scope, routerID string, in SetupInput, planHash, actor string) (domain.Router, error) {
	n, err := normalizeSetup(in)
	if err != nil {
		return domain.Router{}, err
	}
	// Lock before reading router state, so the plan and the ready check below
	// cannot interleave with a concurrent Cleanup of the same router.
	unlock, err := s.lockSetup(ctx, routerID)
	if err != nil {
		return domain.Router{}, err
	}
	defer unlock()
	router, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return domain.Router{}, err
	}
	plan, err := s.plan(ctx, client, n)
	if err != nil {
		return domain.Router{}, planError(err, secrets)
	}
	if plan.Hash != planHash {
		return domain.Router{}, ErrPlanChanged
	}

	// Write phase: detached from request cancellation (see setupWriteTimeout).
	ctx, cancel := writeContext(ctx)
	defer cancel()

	needsWrite := false
	for _, a := range plan.Actions {
		if a.Kind != domain.SetupExists {
			needsWrite = true
		}
	}
	if needsWrite && router.IsolirReady {
		if err := s.repo.MarkSetup(ctx, scope, routerID, false, "", ""); err != nil {
			slog.Warn("mikrotik: mark not-ready before apply failed", "router_id", routerID, "error", scrub(err.Error(), secrets...))
			return domain.Router{}, mikrotik.ErrSetupMarkFailed
		}
	}

	created, updated, unchanged := 0, 0, 0
	for _, a := range plan.Actions {
		var werr error
		switch a.Kind {
		case domain.SetupCreate:
			_, werr = client.Add(ctx, a.Path, a.Attrs)
		case domain.SetupUpdate:
			// a.ID comes from a row that find() verified in Go: managed, not
			// dynamic and equal on every match key.
			if a.ID == "" {
				return domain.Router{}, &mikrotik.SetupStepError{Step: "update " + a.Path, Cause: mikrotik.ErrSetupFailed, RouterWritten: created+updated > 0}
			}
			werr = client.Set(ctx, a.Path, a.ID, withoutKey(a.Attrs, "place-before"))
		default:
			unchanged++
			continue
		}
		if werr != nil {
			return domain.Router{}, &mikrotik.SetupStepError{
				Step: a.Kind + " " + a.Path, Cause: stepCause(werr, secrets), RouterWritten: created+updated > 0,
			}
		}
		if a.Kind == domain.SetupCreate {
			created++
		} else {
			updated++
		}
	}

	verify, err := s.plan(ctx, client, n)
	if err != nil {
		return domain.Router{}, &mikrotik.SetupStepError{Step: "verify", Cause: planError(err, secrets), RouterWritten: true}
	}
	for _, a := range verify.Actions {
		if a.Kind != domain.SetupExists {
			return domain.Router{}, &mikrotik.SetupStepError{
				Step: "verify " + a.Path, Cause: mikrotik.ErrSetupFailed, RouterWritten: true,
			}
		}
	}

	if err := s.repo.MarkSetup(ctx, scope, routerID, true, n.profile, n.redirectURL); err != nil {
		slog.Warn("mikrotik: mark setup failed after apply", "router_id", routerID, "error", scrub(err.Error(), secrets...))
		return domain.Router{}, mikrotik.ErrSetupStateNotSaved
	}
	s.audit(ctx, scope, "router.setup_applied", actor, router, map[string]any{
		"profile": n.profile, "created": created, "updated": updated, "unchanged": unchanged, "redirect": n.redirectURL != "",
	})
	return s.routers.Get(ctx, scope, routerID)
}

func withoutKey(r pmk.Row, key string) pmk.Row {
	out := copyRow(r)
	delete(out, key)
	return out
}

// classifyRemovable decides whether a row returned by Print may be removed: it
// must be zyad-managed, not dynamic and equal to every key/value of the query
// (re-checked in Go because server-side filtering is not trusted). Dynamic rows
// are never ours and are skipped quietly; any other failure is "unverified" and
// must be reported by the caller.
func classifyRemovable(r pmk.Row, query pmk.Row) (ok, unverified bool) {
	if rowDynamic(r) {
		return false, false
	}
	if !rowManaged(r) || r[".id"] == "" {
		return false, true
	}
	for k, v := range query {
		if r[k] != v {
			return false, true
		}
	}
	return true, false
}

func removable(r pmk.Row, query pmk.Row) bool {
	ok, _ := classifyRemovable(r, query)
	return ok
}

// Cleanup removes only zyad-managed objects created by this setup, in reverse
// dependency order, under the per-router setup lock (see Apply). Fail closed: isolir_ready is cleared before the first
// Remove, and if that fails nothing is removed.
func (s *SetupService) Cleanup(ctx context.Context, scope coretenant.Scope, routerID, actor string) (domain.Router, error) {
	unlock, err := s.lockSetup(ctx, routerID)
	if err != nil {
		return domain.Router{}, err
	}
	defer unlock()
	router, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return domain.Router{}, err
	}
	// Detached from request cancellation (see setupWriteTimeout).
	ctx, cancel := writeContext(ctx)
	defer cancel()
	fail := func(err error) (domain.Router, error) { return domain.Router{}, planError(err, secrets) }

	type target struct {
		path  string
		query pmk.Row
		keep  func(pmk.Row) bool
		rev   bool
	}
	managed := func(q pmk.Row) pmk.Row { q["comment"] = setupManaged; return q }

	profileQuery := managed(pmk.Row{"address-list": setupIsolirList})
	profiles, err := client.Print(ctx, pathPPPProfile, profileQuery)
	if err != nil {
		return fail(err)
	}
	// Refuse while any PPP secret still uses an isolir profile: the customer
	// would silently fall off the router profile.
	inUse := 0
	for _, p := range profiles {
		if !removable(p, profileQuery) {
			continue
		}
		rows, err := client.Print(ctx, pathPPPSecret, pmk.Row{"profile": p["name"]})
		if err != nil {
			return fail(err)
		}
		for _, r := range rows {
			if r["profile"] == p["name"] {
				inUse++
			}
		}
	}
	if inUse > 0 {
		return domain.Router{}, &mikrotik.SetupConflictError{
			Kind: "ppp secret", Detail: fmt.Sprintf("%d PPP secret(s) still use the isolir profile; restore them first", inUse),
		}
	}

	targets := []target{
		{path: pathProxyAccess, query: managed(pmk.Row{}), keep: func(r pmk.Row) bool { return r["action"] == "allow" || r["action"] == "redirect" }},
		{path: pathFirewallNAT, query: managed(pmk.Row{"chain": "dstnat", "src-address-list": setupIsolirList})},
		{path: pathFirewallFilter, query: managed(pmk.Row{"chain": "forward", "src-address-list": setupIsolirList}), rev: true},
		{path: pathAddressList, query: managed(pmk.Row{"list": setupAllowList})},
		{path: pathPPPProfile, query: profileQuery},
		{path: pathIPPool, query: managed(pmk.Row{"name": setupPoolName})},
	}
	type removal struct{ path, id string }
	var removals []removal
	unverified := 0
	for _, t := range targets {
		skipped := 0
		rows, err := client.Print(ctx, t.path, t.query)
		if err != nil {
			return fail(err)
		}
		if t.rev {
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
		for _, r := range rows {
			ok, bad := classifyRemovable(r, t.query)
			if bad {
				skipped++
			}
			if !ok || (t.keep != nil && !t.keep(r)) {
				continue
			}
			removals = append(removals, removal{t.path, r[".id"]})
		}
		if skipped > 0 {
			slog.Warn("mikrotik: cleanup skipped rows that could not be verified as zyad-managed", "path", t.path, "skipped", skipped)
			unverified += skipped
		}
	}

	// Fail closed before the first write.
	if err := s.repo.MarkSetup(ctx, scope, routerID, false, "", ""); err != nil {
		slog.Warn("mikrotik: mark not-ready before cleanup failed", "router_id", routerID, "error", scrub(err.Error(), secrets...))
		return domain.Router{}, mikrotik.ErrSetupMarkFailed
	}
	for i, rm := range removals {
		if err := client.Remove(ctx, rm.path, rm.id); err != nil {
			return domain.Router{}, &mikrotik.SetupStepError{Step: "remove " + rm.path, Cause: stepCause(err, secrets), RouterWritten: i > 0}
		}
	}
	s.audit(ctx, scope, "router.setup_cleaned", actor, router, map[string]any{"removed": len(removals), "unverified": unverified})
	if unverified > 0 {
		// Cleanup is incomplete as far as we can tell; isolir_ready is already false.
		return domain.Router{}, &mikrotik.SetupConflictError{
			Kind:   "router objects",
			Detail: fmt.Sprintf("%d objects could not be verified as zyad-managed and were left untouched", unverified),
		}
	}
	return s.routers.Get(ctx, scope, routerID)
}
