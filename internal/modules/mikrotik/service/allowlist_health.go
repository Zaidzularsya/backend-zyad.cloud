package service

import (
	"context"
	"net/netip"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// AllowlistHealth summarises whether the router's DNS resolved the name-based
// entries of the isolir allowlist. Operators only see the counts, never hosts.
type AllowlistHealth struct {
	Total      int
	Unresolved int
}

// pathDNSCache is the router's DNS cache (read only), used to confirm names that
// have no dynamic address-list row of their own.
const pathDNSCache = "/ip/dns/cache/all"

// maxCNAMEHops bounds the CNAME chain followed in the DNS cache.
const maxCNAMEHops = 5

// AllowlistHealth reads (never writes) the allowlist address-list. RouterOS
// keeps each FQDN as a static row (address = the name) and adds dynamic rows
// for the IPs its own DNS resolved, so a name with no resolved IP means the
// router cannot let isolated customers reach that host (finding B6).
//
// Attribution: when dynamic rows carry the name they came from (in "comment"),
// each name is judged on its own. Otherwise only total failure is detectable:
// no dynamic row at all means every name is unresolved.
//
// RouterOS adds no second dynamic row for an IP already in the list, so a name
// sharing an IP with another allowlisted name has no row of its own (lab
// finding T13-C1: zyad.cloud vs api.zyad.cloud, staging.doku.com vs
// api-sandbox.doku.com). Such a name counts as resolved when the router's DNS
// cache maps it (following CNAMEs) to an IP that is already in the list.
func (s *SetupService) AllowlistHealth(ctx context.Context, scope coretenant.Scope, routerID string) (AllowlistHealth, error) {
	_, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return AllowlistHealth{}, err
	}
	rows, err := client.Print(ctx, pathAddressList, pmk.Row{"list": setupAllowList})
	if err != nil {
		return AllowlistHealth{}, translateClientError(err, secrets...)
	}
	h, unresolved, listed := summarizeAllowlist(rows)
	for _, name := range unresolved {
		if cachedIPListed(ctx, client, name, listed) {
			h.Unresolved--
		}
	}
	return h, nil
}

// cachedIPListed reports whether the DNS cache resolves name to an IP in listed.
// Lookup errors count as "not listed": this is a warning heuristic only.
func cachedIPListed(ctx context.Context, client pmk.RouterClient, name string, listed map[string]bool) bool {
	for hop := 0; hop < maxCNAMEHops && name != ""; hop++ {
		rows, err := client.Print(ctx, pathDNSCache, pmk.Row{"name": name})
		if err != nil {
			return false
		}
		next := ""
		for _, r := range rows {
			switch r["type"] {
			case "A", "AAAA":
				if listed[r["data"]] {
					return true
				}
			case "CNAME":
				next = strings.TrimSuffix(r["data"], ".")
			}
		}
		name = next
	}
	return false
}

// summarizeAllowlist returns the health, the names counted as unresolved, and
// the IPs already in the list (dynamic rows and IP literals).
func summarizeAllowlist(rows []pmk.Row) (AllowlistHealth, []string, map[string]bool) {
	var names []string
	resolvedFor := map[string]bool{}
	listed := map[string]bool{}
	dynamic, attributed := 0, false
	for _, r := range rows {
		if r["list"] != "" && r["list"] != setupAllowList {
			continue
		}
		if rowDynamic(r) {
			dynamic++
			listed[r["address"]] = true
			if c := r["comment"]; c != "" {
				resolvedFor[c] = true
				attributed = true
			}
			continue
		}
		if _, err := netip.ParseAddr(r["address"]); err == nil || r["address"] == "" {
			listed[r["address"]] = true
			continue // IP literal: always "resolved"
		}
		names = append(names, r["address"])
	}
	h := AllowlistHealth{Total: len(names)}
	var unresolved []string
	for _, n := range names {
		if (attributed && !resolvedFor[n]) || (!attributed && dynamic == 0) {
			h.Unresolved++
			unresolved = append(unresolved, n)
		}
	}
	return h, unresolved, listed
}
