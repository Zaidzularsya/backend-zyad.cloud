package mikrotik

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// ErrHostNotAllowed is returned when a router host resolves to an address the
// guard refuses to connect to.
var ErrHostNotAllowed = errors.New("mikrotik: host not allowed")

// Router hosts come from tenants, so connecting to them is an SSRF vector.
// Public addresses are always allowed. Non-public addresses (private, CGNAT,
// ULA) are allowed only when covered by the operator-configured allow list.
// Loopback, link-local (including cloud metadata 169.254.169.254) and
// unspecified addresses are never allowed, even if listed.

// allowLoopbackForTests lets this package's own tests reach in-process fake
// servers on 127.0.0.1 (they still have to list it). Only TestMain in a
// _test.go file sets it; production code never does.
var allowLoopbackForTests bool

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

// dnsLookupBudget bounds the host lookup in ValidateHost so a slow or
// blackholed resolver cannot eat the request budget. A variable only so tests
// can shorten it.
var dnsLookupBudget = 3 * time.Second

// lookupNetIP is a variable so tests can inject DNS answers.
var lookupNetIP = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func isPublicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!carrierGradeNAT.Contains(addr)
}

func isNeverAllowed(addr netip.Addr) bool {
	return addr.IsUnspecified() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()
}

// checkAddress reports whether addr may be dialed and whether it is public.
func checkAddress(addr netip.Addr, allowedPrivate []netip.Prefix) (allowed, public bool) {
	addr = addr.Unmap()
	if isNeverAllowed(addr) {
		return false, false
	}
	if addr.IsLoopback() && !allowLoopbackForTests {
		return false, false
	}
	if isPublicAddress(addr) {
		return true, true
	}
	for _, p := range allowedPrivate {
		if p.Contains(addr) {
			return true, false
		}
	}
	return false, false
}

// ValidateHost resolves host (a literal IP or a hostname) and requires every
// resolved address to be allowed. public is true only if all of them are public.
func ValidateHost(ctx context.Context, host string, allowedPrivate []netip.Prefix) (resolved []netip.Addr, public bool, err error) {
	if host == "" {
		return nil, false, fmt.Errorf("%w: empty host", ErrHostNotAllowed)
	}
	if addr, perr := netip.ParseAddr(host); perr == nil {
		resolved = []netip.Addr{addr.Unmap()}
	} else {
		lctx, cancel := context.WithTimeout(ctx, dnsLookupBudget)
		resolved, err = lookupNetIP(lctx, host)
		cancel()
		if err != nil {
			if ctx.Err() != nil && errors.Is(err, context.Canceled) {
				return nil, false, context.Canceled
			}
			// Lookup failure or timeout: the router cannot be reached from
			// here. Unreachable-class, never host-not-allowed.
			return nil, false, fmt.Errorf("%w: dns lookup failed for %q", ErrUnreachable, host)
		}
		if len(resolved) == 0 {
			return nil, false, fmt.Errorf("%w: %s resolves to no address", ErrHostNotAllowed, host)
		}
	}

	public = true
	for i, addr := range resolved {
		addr = addr.Unmap()
		resolved[i] = addr
		ok, pub := checkAddress(addr, allowedPrivate)
		if !ok {
			return nil, false, fmt.Errorf("%w: %s", ErrHostNotAllowed, addr)
		}
		public = public && pub
	}
	return resolved, public, nil
}

// DialControl returns a net.Dialer Control hook that re-checks the address at
// connect time, so a DNS answer changing after ValidateHost (rebinding) cannot
// redirect the connection.
func DialControl(allowedPrivate []netip.Prefix) func(network, address string, c syscall.RawConn) error {
	return func(_ string, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
		}
		if ok, _ := checkAddress(addr, allowedPrivate); !ok {
			return fmt.Errorf("%w: %s", ErrHostNotAllowed, addr.Unmap())
		}
		return nil
	}
}
