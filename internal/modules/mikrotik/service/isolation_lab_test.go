//go:build mikrotiklab

package service

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	corecrypto "zyad.cloud/internal/core/crypto"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Contract test for the isolation Executor against a real RouterOS router
// (Fase 3, Task 10). It exercises Isolate/Restore through every transport the
// router supports with a DISABLED throwaway secret that cannot log in.
//
// Env: MIKROTIK_LAB_HOST/USER/PASS (required), MIKROTIK_LAB_PORT (binary API port,
// optional, default 8728), MIKROTIK_LAB_REST_PORT (optional, default 80),
// MIKROTIK_LAB_WRITE=1 (required, the test writes to the router) and
// MIKROTIK_LAB_ISOLIR_PROFILE (optional, default zyad-isolir: the profile
// created by the Fase 1 setup, which must already exist on the router).
//
//	MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run TestLab_IsolateRestoreRoundTrip -v -count=1 ./internal/modules/mikrotik/service/
//
// Only counts and profile names are logged, never credentials.

const (
	labTestSecretName = "zyad-labtest-user"
	labNormalProfile  = "default"
)

func TestLab_IsolateRestoreRoundTrip(t *testing.T) {
	host, user, pass := os.Getenv("MIKROTIK_LAB_HOST"), os.Getenv("MIKROTIK_LAB_USER"), os.Getenv("MIKROTIK_LAB_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("MIKROTIK_LAB_HOST/USER/PASS not set")
	}
	if os.Getenv("MIKROTIK_LAB_WRITE") != "1" {
		t.Skip("MIKROTIK_LAB_WRITE=1 not set; this test writes to the lab router")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatal("MIKROTIK_LAB_HOST must be an IP address")
	}
	allowed := []netip.Prefix{netip.PrefixFrom(addr, addr.BitLen())}
	isolirProfile := os.Getenv("MIKROTIK_LAB_ISOLIR_PROFILE")
	if isolirProfile == "" {
		isolirProfile = setupDefaultProf
	}
	for _, apiType := range []string{pmk.APITypeBinary, pmk.APITypeREST} {
		apiType := apiType
		t.Run(apiType, func(t *testing.T) {
			labRoundTrip(t, apiType, host, user, pass, isolirProfile, allowed)
		})
	}
}

func labRoundTrip(t *testing.T, apiType, host, user, pass, isolirProfile string, allowed []netip.Prefix) {
	rest := apiType == pmk.APITypeREST
	fail := func(format string, args ...any) {
		t.Helper()
		if rest {
			t.Skipf("rest transport not usable on this router: "+format, args...)
		}
		t.Fatalf(format, args...)
	}

	port := 0
	portEnv := "MIKROTIK_LAB_PORT"
	if rest {
		portEnv = "MIKROTIK_LAB_REST_PORT" // the REST (www) port differs from the binary API port
	}
	if p := os.Getenv(portEnv); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			t.Fatal("bad " + portEnv)
		}
		port = n
	}
	if port == 0 {
		// Standard RouterOS defaults: binary API 8728, REST (www) 80.
		port = 8728
		if rest {
			port = 80
		}
	}
	ep := pmk.Endpoint{Host: host, Port: port, Username: user, Password: pass, AllowedPrivate: allowed, Timeout: 20 * time.Second}
	raw, err := pmk.NewClient(apiType, ep)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	profiles, err := raw.Print(ctx, pppProfilePath, nil)
	if err != nil {
		fail("print /ppp/profile: %v", err)
	}
	have := map[string]bool{}
	for _, p := range profiles {
		have[p["name"]] = true
	}
	if !have[isolirProfile] {
		t.Skipf("isolir profile %q not on the router (run the Fase 1 setup first, or set MIKROTIK_LAB_ISOLIR_PROFILE)", isolirProfile)
	}
	if !have[labNormalProfile] {
		t.Skipf("profile %q not on the router", labNormalProfile)
	}
	if rows, err := raw.Print(ctx, secretPath, pmk.Row{"name": labTestSecretName}); err != nil {
		fail("print /ppp/secret: %v", err)
	} else if len(rows) != 0 {
		t.Skipf("test secret %s already exists on the router; refusing to touch it", labTestSecretName)
	}

	// Disabled + random password: it can never authenticate.
	if _, err := raw.Add(ctx, secretPath, pmk.Row{
		"name": labTestSecretName, "password": "labtest-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"profile": labNormalProfile, "disabled": "yes", "comment": "zyad-labtest",
	}); err != nil {
		fail("create test secret: %v", err)
	}
	t.Cleanup(func() {
		// Removes by name so cleanup works even when the add id is not reusable.
		rows, err := raw.Print(context.Background(), secretPath, pmk.Row{"name": labTestSecretName})
		if err != nil {
			t.Errorf("cleanup list: %v (remove %s manually)", err, labTestSecretName)
			return
		}
		for _, r := range rows {
			if r["name"] == labTestSecretName {
				if err := raw.Remove(context.Background(), secretPath, r[rowIDKey]); err != nil {
					t.Errorf("cleanup remove: %v (remove %s manually)", err, labTestSecretName)
				}
			}
		}
	})

	enc, err := corecrypto.EncryptSecret(testSecret, pass)
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	scope := testScope(t)
	routerID := "00000000-0000-0000-0000-0000000000b1"
	repo.routers[routerID] = domain.Router{
		ID: routerID, OrganizationID: scope.OrganizationID(), Name: "lab", Host: host, Port: port,
		APIType: domain.APIType(apiType), Username: user, PasswordEncrypted: enc,
		IsolirReady: true, IsolirProfileName: isolirProfile,
	}
	ex := NewExecutor(NewRouterService(repo, NewPlatformClientFactory(), testSecret, allowed))
	sec := domain.PPPSecret{RouterID: routerID, Name: labTestSecretName, OriginalProfile: labNormalProfile}

	profileOnRouter := func() string {
		t.Helper()
		rows, err := raw.Print(ctx, secretPath, pmk.Row{"name": labTestSecretName})
		if err != nil || len(rows) != 1 {
			t.Fatalf("read back secret: rows=%d err=%v", len(rows), err)
		}
		return rows[0]["profile"]
	}

	orig, err := ex.Isolate(ctx, scope, sec)
	if err != nil {
		fail("Isolate: %v", err)
	}
	if orig != labNormalProfile {
		t.Fatalf("Isolate returned original %q, want %q", orig, labNormalProfile)
	}
	if got := profileOnRouter(); got != isolirProfile {
		t.Fatalf("after Isolate profile = %q, want %q", got, isolirProfile)
	}

	// Idempotent: second Isolate keeps the stored original profile.
	orig, err = ex.Isolate(ctx, scope, sec)
	if err != nil || orig != labNormalProfile {
		t.Fatalf("second Isolate: original=%q err=%v", orig, err)
	}
	if got := profileOnRouter(); got != isolirProfile {
		t.Fatalf("after second Isolate profile = %q, want %q", got, isolirProfile)
	}

	// Documents the query behaviour: a disabled secret has no session, and
	// filtering /ppp/active by name must return zero rows without error.
	active, err := raw.Print(ctx, pppActivePath, pmk.Row{"name": labTestSecretName})
	if err != nil {
		t.Fatalf("print /ppp/active filtered by name: %v", err)
	}
	t.Logf("%s: /ppp/active rows for test user = %d", apiType, len(active))
	if len(active) != 0 {
		t.Fatalf("expected 0 active rows for the disabled test secret, got %d", len(active))
	}

	if err := ex.Restore(ctx, scope, sec); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := profileOnRouter(); got != labNormalProfile {
		t.Fatalf("after Restore profile = %q, want %q", got, labNormalProfile)
	}
	t.Logf("%s: isolate -> isolate -> restore round trip OK (isolir profile %q)", apiType, isolirProfile)
}
