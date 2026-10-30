package config

import (
	"net/netip"
	"testing"
)

func TestLoadMikrotikParsesAllowedPrivateCIDRs(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"two valid prefixes with spaces", "10.8.0.0/24, 10.9.0.0/16", []string{"10.8.0.0/24", "10.9.0.0/16"}},
		{"invalid entry skipped", "not-a-cidr,10.8.0.0/24", []string{"10.8.0.0/24"}},
		{"empty", "", nil},
		{"blank entries", " , ,", nil},
		{"loopback link-local unspecified skipped", "127.0.0.0/8,169.254.0.0/16,0.0.0.0/0,::1/128,fe80::/10,::/0,10.8.0.0/24", []string{"10.8.0.0/24"}},
		{"wide prefix containing loopback rejected", "64.0.0.0/2", nil},
		{"single loopback host rejected", "127.0.0.1/32", nil},
		{"loopback /8 rejected", "127.0.0.0/8", nil},
		{"everything rejected", "0.0.0.0/0", nil},
		{"this-network rejected", "0.0.0.0/8", nil},
		{"private /8 accepted", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"ipv6 link-local rejected", "fe80::/10", nil},
		{"ipv4 link-local rejected", "169.254.0.0/16", nil},
		{"wide prefix containing link-local rejected", "128.0.0.0/1", nil},
		{"multicast rejected", "224.0.0.0/4", nil},
		{"ipv6 multicast rejected", "ff00::/8", nil},
		{"ipv6 ula accepted", "fd00:1234::/32", []string{"fd00:1234::/32"}},
		{"cgnat accepted", "100.64.0.0/10", []string{"100.64.0.0/10"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MIKROTIK_ALLOWED_PRIVATE_CIDRS", tc.env)
			got := LoadMikrotik().AllowedPrivateCIDRs
			if len(got) != len(tc.want) {
				t.Fatalf("AllowedPrivateCIDRs = %v, want %v", got, tc.want)
			}
			for i, w := range tc.want {
				if got[i] != netip.MustParsePrefix(w) {
					t.Fatalf("AllowedPrivateCIDRs[%d] = %v, want %s", i, got[i], w)
				}
			}
		})
	}
}

func TestLoadMikrotikSetupAllowREST(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{{"", false}, {"false", false}, {"true", true}, {"not-a-bool", false}} {
		t.Setenv("MIKROTIK_SETUP_ALLOW_REST", tc.env)
		if got := LoadMikrotik().SetupAllowREST; got != tc.want {
			t.Fatalf("MIKROTIK_SETUP_ALLOW_REST=%q: SetupAllowREST = %v, want %v", tc.env, got, tc.want)
		}
	}
}
