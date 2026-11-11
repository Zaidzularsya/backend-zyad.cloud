package service

import (
	"context"
	"net/netip"

	coretenant "zyad.cloud/internal/core/tenant"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// AllowlistHealth summarises whether the router's DNS resolved the name-based
// entries of the isolir allowlist. Operators only see the counts, never hosts.
type AllowlistHealth struct {
	Total      int
	Unresolved int
}

// AllowlistHealth reads (never writes) the allowlist address-list. RouterOS
// keeps each FQDN as a static row (address = the name) and adds dynamic rows
// for the IPs its own DNS resolved, so a name with no resolved IP means the
// router cannot let isolated customers reach that host (finding B6).
//
// Attribution: when dynamic rows carry the name they came from (in "comment"),
// each name is judged on its own. Otherwise only total failure is detectable:
// no dynamic row at all means every name is unresolved. This is a read-only
// heuristic; the exact row shape must be confirmed on the lab router.
func (s *SetupService) AllowlistHealth(ctx context.Context, scope coretenant.Scope, routerID string) (AllowlistHealth, error) {
	_, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return AllowlistHealth{}, err
	}
	rows, err := client.Print(ctx, pathAddressList, pmk.Row{"list": setupAllowList})
	if err != nil {
		return AllowlistHealth{}, translateClientError(err, secrets...)
	}
	return summarizeAllowlist(rows), nil
}

func summarizeAllowlist(rows []pmk.Row) AllowlistHealth {
	var names []string
	resolvedFor := map[string]bool{}
	dynamic, attributed := 0, false
	for _, r := range rows {
		if r["list"] != "" && r["list"] != setupAllowList {
			continue
		}
		if rowDynamic(r) {
			dynamic++
			if c := r["comment"]; c != "" {
				resolvedFor[c] = true
				attributed = true
			}
			continue
		}
		if _, err := netip.ParseAddr(r["address"]); err == nil || r["address"] == "" {
			continue // IP literal: always "resolved"
		}
		names = append(names, r["address"])
	}
	h := AllowlistHealth{Total: len(names)}
	for _, n := range names {
		switch {
		case attributed && !resolvedFor[n]:
			h.Unresolved++
		case !attributed && dynamic == 0:
			h.Unresolved++
		}
	}
	return h
}
