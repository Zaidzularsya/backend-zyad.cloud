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
