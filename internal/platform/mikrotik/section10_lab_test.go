//go:build mikrotiklab

package mikrotik

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// TestLabSection10_ProbeProfileInUseDelete records what RouterOS itself does when
// a PPP profile that a secret still uses is removed (runbook section 10, item 11).
// It is a probe: the outcome is logged for the runbook, the only assertions are
// that the test objects are gone afterwards.
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run TestLabSection10_ProbeProfileInUseDelete -v -count=1 ./internal/platform/mikrotik/
//
// Only zyad-labtest-* objects with comment=zyad-managed are created and removed.
func TestLabSection10_ProbeProfileInUseDelete(t *testing.T) {
	requireLabWrite(t)
	c := labBinary(t)
	ctx := context.Background()
	const profile, secret = "zyad-labtest-inuse", "zyad-labtest-inuse-user"

	byName := func(path, name string) []Row {
		rows, err := c.Print(ctx, path, Row{"name": name})
		if err != nil {
			t.Fatalf("print %s: %v", path, err)
		}
		var out []Row
		for _, r := range rows {
			if r["name"] == name {
				out = append(out, r)
			}
		}
		return out
	}
	if len(byName("/ppp/profile", profile)) != 0 || len(byName("/ppp/secret", secret)) != 0 {
		t.Skip("test objects already exist; refusing to run")
	}

	profID, err := c.Add(ctx, "/ppp/profile", Row{"name": profile, "comment": labManaged})
	if err != nil {
		t.Fatalf("add profile: %v", err)
	}
	t.Cleanup(func() {
		for _, r := range byName("/ppp/secret", secret) {
			if err := c.Remove(ctx, "/ppp/secret", r[".id"]); err != nil {
				t.Errorf("cleanup secret: %v", err)
			}
		}
		for _, r := range byName("/ppp/profile", profile) {
			if err := c.Remove(ctx, "/ppp/profile", r[".id"]); err != nil {
				t.Errorf("cleanup profile: %v", err)
			}
		}
		if n := len(byName("/ppp/profile", profile)) + len(byName("/ppp/secret", secret)); n != 0 {
			t.Errorf("%d test objects left after cleanup", n)
		}
	})

	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Add(ctx, "/ppp/secret", Row{
		"name": secret, "password": hex.EncodeToString(buf), "service": "pppoe",
		"profile": profile, "disabled": "yes", "comment": labManaged,
	}); err != nil {
		t.Fatalf("add secret: %v", err)
	}

	rmErr := c.Remove(ctx, "/ppp/profile", profID)
	if rmErr != nil {
		t.Logf("RouterOS REFUSES to remove a profile that a secret uses: %v", rmErr)
		return
	}
	rows := byName("/ppp/secret", secret)
	if len(rows) == 1 {
		t.Logf("RouterOS ALLOWS removing a profile that a secret uses; the secret now shows profile=%q", rows[0]["profile"])
	} else {
		t.Logf("RouterOS removed the profile and %d secret rows remain", len(rows))
	}
}
