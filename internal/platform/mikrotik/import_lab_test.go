//go:build mikrotiklab

package mikrotik

import (
	"context"
	"testing"
)

// Contract check for the secret import (Fase 2): the fields the import
// service reads from /ppp/secret and /ppp/profile must be listable through
// every transport the router supports. Only counts and key presence are
// logged, never secret names, passwords or credentials.
//
// Env: MIKROTIK_LAB_HOST/USER/PASS (required), MIKROTIK_LAB_PORT (optional).
// Run: go test -tags mikrotiklab -run TestLab_Import -v ./internal/platform/mikrotik/

func TestLab_ImportListsPPPSecretsAndProfiles(t *testing.T) {
	ep := labEndpoint(t)
	transports := []string{APITypeREST, APITypeBinary}
	for _, apiType := range transports {
		apiType := apiType
		t.Run(apiType, func(t *testing.T) {
			c, err := NewClient(apiType, ep)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			secrets, err := c.Print(ctx, "/ppp/secret", nil)
			if err != nil {
				t.Skipf("%s transport cannot list /ppp/secret on this router: %v", apiType, err)
			}
			profiles, err := c.Print(ctx, "/ppp/profile", nil)
			if err != nil {
				t.Fatalf("/ppp/profile: %v", err)
			}
			t.Logf("%s: ppp secrets=%d profiles=%d", apiType, len(secrets), len(profiles))
			if len(profiles) == 0 {
				t.Fatal("expected at least the default ppp profiles")
			}
			for _, p := range profiles {
				if p["name"] == "" {
					t.Fatal("profile row without name")
				}
			}
			for _, s := range secrets {
				if s["name"] == "" {
					t.Fatal("secret row without name")
				}
				if _, ok := s["profile"]; !ok {
					t.Fatal("secret row without profile key")
				}
				if _, ok := s["disabled"]; !ok {
					t.Fatal("secret row without disabled key")
				}
			}
		})
	}
}
