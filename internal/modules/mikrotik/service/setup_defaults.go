package service

import (
	"net/url"
	"sort"
	"strings"

	"zyad.cloud/internal/platform/doku"
)

// DefaultPoolCIDR is the temporary subnet isolated customers are moved into.
const DefaultPoolCIDR = "10.255.254.0/24"

// SetupDefaults derives everything an operator should never have to type from
// platform configuration: the allowlist (platform API, pay page, DOKU checkout
// and its assets) and the default isolir pool. Operator input always wins for
// the scalar fields; hosts are a union so the walled garden stays complete.
type SetupDefaults struct {
	PoolCIDR         string
	PlatformHost     string
	PayHost          string
	TenantIsolirHost string
	// DokuBaseURL selects the sandbox or production checkout host set. Empty is
	// treated as sandbox, matching config.LoadDoku's default.
	DokuBaseURL string
	ExtraHosts  []string
}

// Fill returns in with empty fields completed from the defaults.
func (d SetupDefaults) Fill(in SetupInput) SetupInput {
	if d.isZero() {
		return in // unconfigured service: behave exactly as before defaults existed
	}
	if strings.TrimSpace(in.PoolCIDR) == "" {
		in.PoolCIDR = d.PoolCIDR
		if strings.TrimSpace(in.PoolCIDR) == "" {
			in.PoolCIDR = DefaultPoolCIDR
		}
	}
	if strings.TrimSpace(in.PlatformHost) == "" {
		in.PlatformHost = hostOnly(d.PlatformHost)
	}
	if strings.TrimSpace(in.TenantIsolirHost) == "" {
		in.TenantIsolirHost = hostOnly(d.TenantIsolirHost)
	}

	base := d.DokuBaseURL
	if strings.TrimSpace(base) == "" {
		base = doku.SandboxBaseURL
	}
	hosts := append([]string(nil), in.DokuHosts...)
	hosts = append(hosts, hostOnly(d.PayHost))
	hosts = append(hosts, doku.CheckoutHosts(base)...)
	for _, h := range d.ExtraHosts {
		hosts = append(hosts, hostOnly(h))
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	sort.Strings(out)
	in.DokuHosts = out
	return in
}

// hostOnly reduces "https://ZYAD.cloud:443/i/x" or "zyad.cloud:8080" to a
// lowercase bare host. Input that cannot be reduced is returned trimmed so
// normalizeHost can reject it with a field-specific message.
func hostOnly(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "//" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return strings.ToLower(strings.TrimSpace(raw))
	}
	return u.Hostname()
}

func (d SetupDefaults) isZero() bool {
	return d.PoolCIDR == "" && d.PlatformHost == "" && d.PayHost == "" && d.TenantIsolirHost == "" &&
		d.DokuBaseURL == "" && len(d.ExtraHosts) == 0
}
