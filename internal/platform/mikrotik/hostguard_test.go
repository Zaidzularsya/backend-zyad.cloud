package mikrotik

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

func TestValidateHost(t *testing.T) {
	orig := lookupNetIP
	t.Cleanup(func() { lookupNetIP = orig })
	lookupNetIP = func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "localhost":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")}, nil
		case "router.example.com":
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		case "mixed.example.com":
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("192.168.1.1")}, nil
		case "empty.example.com":
			return nil, nil
		}
		return nil, errors.New("no such host")
	}

	tests := []struct {
		name       string
		host       string
		allowed    []netip.Prefix
		wantErr    bool
		wantPublic bool
	}{
		{"loopback v4", "127.0.0.1", nil, true, false},
		{"loopback v6", "::1", nil, true, false},
		{"metadata", "169.254.169.254", nil, true, false},
		{"metadata even if listed", "169.254.169.254", prefixes(t, "169.254.0.0/16"), true, false},
		{"unspecified", "0.0.0.0", nil, true, false},
		{"unspecified even if listed", "0.0.0.0", prefixes(t, "0.0.0.0/0"), true, false},
		{"unspecified v6", "::", nil, true, false},
		{"private outside allow list", "192.168.1.1", nil, true, false},
		{"private with other prefix", "192.168.1.1", prefixes(t, "10.8.0.0/24"), true, false},
		{"cgnat", "100.64.0.1", nil, true, false},
		{"public", "8.8.8.8", nil, false, true},
		{"private allowed", "10.8.0.5", prefixes(t, "10.8.0.0/24"), false, false},
		{"private not allowed", "10.8.0.5", nil, true, false},
		{"loopback explicitly listed", "127.0.0.1", prefixes(t, "127.0.0.1/32"), false, false},
		{"v4-mapped loopback", "::ffff:127.0.0.1", nil, true, false},
		{"localhost", "localhost", nil, true, false},
		{"hostname public", "router.example.com", nil, false, true},
		{"hostname with one private result", "mixed.example.com", nil, true, false},
		{"hostname no results", "empty.example.com", nil, true, false},
		{"hostname lookup error", "nxdomain.example.com", nil, true, false},
		{"empty host", "", nil, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, public, err := ValidateHost(context.Background(), tc.host, tc.allowed)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got resolved=%v public=%v", resolved, public)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if public != tc.wantPublic {
				t.Fatalf("public = %v, want %v", public, tc.wantPublic)
			}
			if len(resolved) == 0 {
				t.Fatal("expected resolved addresses")
			}
		})
	}

	_, _, err := ValidateHost(context.Background(), "127.0.0.1", nil)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

func TestDialControl(t *testing.T) {
	tests := []struct {
		name    string
		address string
		allowed []netip.Prefix
		wantErr bool
	}{
		{"public", "8.8.8.8:443", nil, false},
		{"loopback rejected", "127.0.0.1:443", nil, true},
		{"loopback allowed when listed", "127.0.0.1:443", prefixes(t, "127.0.0.1/32"), false},
		{"private allowed when listed", "10.8.0.5:443", prefixes(t, "10.8.0.0/24"), false},
		{"metadata rejected even if listed", "169.254.169.254:80", prefixes(t, "169.254.0.0/16"), true},
		{"unspecified rejected even if listed", "0.0.0.0:80", prefixes(t, "0.0.0.0/0"), true},
		{"hostname rejected", "router.example.com:443", nil, true},
		{"missing port", "8.8.8.8", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := DialControl(tc.allowed)("tcp", tc.address, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
