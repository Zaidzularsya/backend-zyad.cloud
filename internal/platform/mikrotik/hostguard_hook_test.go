package mikrotik

import (
	"net/netip"
	"os"
	"testing"
)

// loopbackDefault is allowLoopbackForTests as production code sees it, captured
// before TestMain enables loopback for the in-process fake servers.
var loopbackDefault bool

func TestMain(m *testing.M) {
	loopbackDefault = allowLoopbackForTests
	allowLoopbackForTests = true // httptest/fake RouterOS servers listen on 127.0.0.1
	os.Exit(m.Run())
}

// withoutLoopbackHook runs fn with production loopback behavior.
func withoutLoopbackHook(t *testing.T) {
	t.Helper()
	allowLoopbackForTests = false
	t.Cleanup(func() { allowLoopbackForTests = true })
}

func TestLoopbackHookIsOffByDefault(t *testing.T) {
	if loopbackDefault {
		t.Fatal("allowLoopbackForTests must default to false in production code")
	}
}

func TestLoopbackNeverAllowedInProductionEvenIfListed(t *testing.T) {
	withoutLoopbackHook(t)
	for _, tc := range []struct{ addr, allow string }{
		{"127.0.0.1", "127.0.0.1/32"},
		{"127.0.0.1", "64.0.0.0/2"},
		{"::1", "::1/128"},
		{"::ffff:127.0.0.1", "127.0.0.0/8"},
	} {
		if ok, _ := checkAddress(netip.MustParseAddr(tc.addr), []netip.Prefix{netip.MustParsePrefix(tc.allow)}); ok {
			t.Errorf("%s allowed with allow list %s", tc.addr, tc.allow)
		}
		if err := DialControl([]netip.Prefix{netip.MustParsePrefix(tc.allow)})("tcp", netip.AddrPortFrom(netip.MustParseAddr(tc.addr), 8728).String(), nil); err == nil {
			t.Errorf("DialControl allowed %s with allow list %s", tc.addr, tc.allow)
		}
	}
	// A listed private range still works.
	if ok, _ := checkAddress(netip.MustParseAddr("10.8.0.5"), []netip.Prefix{netip.MustParsePrefix("10.8.0.0/24")}); !ok {
		t.Fatal("listed private address rejected")
	}
}
