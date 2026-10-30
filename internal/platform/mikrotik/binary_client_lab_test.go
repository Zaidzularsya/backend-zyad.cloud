//go:build mikrotiklab

package mikrotik

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"
)

// Runs against a real RouterOS router. Required env: MIKROTIK_LAB_HOST,
// MIKROTIK_LAB_USER, MIKROTIK_LAB_PASS. Optional: MIKROTIK_LAB_PORT,
// MIKROTIK_LAB_TLS=1 (TLS pin is not set: use only with a trusted cert).
// Run: go test -tags mikrotiklab -run TestLabBinary -v ./internal/platform/mikrotik/
func labClient(t *testing.T) RouterClient {
	t.Helper()
	host, user, pass := os.Getenv("MIKROTIK_LAB_HOST"), os.Getenv("MIKROTIK_LAB_USER"), os.Getenv("MIKROTIK_LAB_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("MIKROTIK_LAB_HOST/USER/PASS not set")
	}
	ep := Endpoint{
		Host: host, Username: user, Password: pass,
		UseTLS:         os.Getenv("MIKROTIK_LAB_TLS") == "1",
		Timeout:        10 * time.Second,
		AllowedPrivate: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12")},
	}
	if p := os.Getenv("MIKROTIK_LAB_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("bad MIKROTIK_LAB_PORT: %v", err)
		}
		ep.Port = n
	}
	c, err := NewClient(APITypeBinary, ep)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLabBinaryVersionAndPrint(t *testing.T) {
	c := labClient(t)
	ctx := context.Background()
	v, err := c.Version(ctx)
	if err != nil || v == "" {
		t.Fatalf("version: %q %v", v, err)
	}
	t.Logf("RouterOS %s", v)
	rows, err := c.Print(ctx, "/ppp/profile", nil)
	if err != nil || len(rows) == 0 {
		t.Fatalf("print: %v rows=%d", err, len(rows))
	}
}

func TestLabBinaryProfileLifecycle(t *testing.T) {
	c := labClient(t)
	ctx := context.Background()
	const name = "zyad-labtest"
	id, err := c.Add(ctx, "/ppp/profile", Row{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Remove(context.Background(), "/ppp/profile", id) })
	if err := c.Set(ctx, "/ppp/profile", id, Row{"comment": "zyad lab test"}); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Print(ctx, "/ppp/profile", Row{"name": name})
	if err != nil || len(rows) != 1 || rows[0]["comment"] != "zyad lab test" {
		t.Fatalf("print: %v %#v", err, rows)
	}
	if err := c.Remove(ctx, "/ppp/profile", id); err != nil {
		t.Fatal(err)
	}
}
