package mikrotik

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

// TestDetectHonorsTotalBudget: with both probes silent, the whole Detect must
// end within the total budget (not first probe + a full endpoint timeout) and
// return the bare unreachable error.
func TestDetectHonorsTotalBudget(t *testing.T) {
	origBudget, origFirst := detectTotalBudget, detectFirstProbeMax
	t.Cleanup(func() { detectTotalBudget, detectFirstProbeMax = origBudget, origFirst })
	detectTotalBudget = 1200 * time.Millisecond
	detectFirstProbeMax = 400 * time.Millisecond

	f := newFakeROS(t, nil)
	f.silent.Store(true)
	ep := f.endpoint()
	ep.Timeout = 0 // DefaultTimeout (10s) would apply to the second probe without a total budget
	start := time.Now()
	_, _, err := Detect(context.Background(), ep)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrUnreachable) || err.Error() != ErrUnreachable.Error() {
		t.Fatalf("want bare ErrUnreachable, got %v", err)
	}
	if elapsed > detectTotalBudget+500*time.Millisecond {
		t.Fatalf("Detect took %v, budget %v", elapsed, detectTotalBudget)
	}
}

// TestDetectBudgetGrowsWithConfiguredTimeout: an explicitly larger endpoint
// timeout raises the total budget instead of being cut to the default.
func TestDetectBudgetGrowsWithConfiguredTimeout(t *testing.T) {
	if got := detectBudget(Endpoint{}); got != detectTotalBudget {
		t.Fatalf("default budget = %v, want %v", got, detectTotalBudget)
	}
	if got := detectBudget(Endpoint{Timeout: 30 * time.Second}); got != 30*time.Second {
		t.Fatalf("budget = %v, want 30s", got)
	}
	if got := detectBudget(Endpoint{Timeout: time.Second}); got != detectTotalBudget {
		t.Fatalf("budget = %v, want %v", got, detectTotalBudget)
	}
}

func TestValidateHostDNSBudget(t *testing.T) {
	if dnsLookupBudget != 3*time.Second {
		t.Fatalf("dnsLookupBudget = %v, want 3s", dnsLookupBudget)
	}
	origLookup, origBudget := lookupNetIP, dnsLookupBudget
	t.Cleanup(func() { lookupNetIP, dnsLookupBudget = origLookup, origBudget })
	dnsLookupBudget = 200 * time.Millisecond
	lookupNetIP = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		<-ctx.Done() // a blackholed resolver
		return nil, ctx.Err()
	}
	start := time.Now()
	_, _, err := ValidateHost(context.Background(), "slow.example.com", nil)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("lookup took %v, budget %v", elapsed, dnsLookupBudget)
	}
	if !errors.Is(err, ErrUnreachable) || errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("lookup timeout must be unreachable-class, got %v", err)
	}
}

// TestBinaryDialAndOperationShareOneDeadline: a slow connect followed by a
// silent server must fail after about one endpoint timeout, not dial time plus
// a fresh full timeout.
func TestBinaryDialAndOperationShareOneDeadline(t *testing.T) {
	f := newFakeROS(t, nil)
	f.silent.Store(true)
	ep := f.endpoint()
	ep.Timeout = 600 * time.Millisecond

	orig := binaryDial
	t.Cleanup(func() { binaryDial = orig })
	binaryDial = func(ctx context.Context, d *net.Dialer, address string) (net.Conn, error) {
		time.Sleep(400 * time.Millisecond) // slow TCP connect
		return orig(ctx, d, address)
	}
	start := time.Now()
	_, err := newBinaryClient(ep).Version(context.Background())
	elapsed := time.Since(start)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want ErrUnreachable, got %v", err)
	}
	if elapsed > ep.Timeout+250*time.Millisecond {
		t.Fatalf("took %v, want about one timeout (%v)", elapsed, ep.Timeout)
	}
}
