package config

import "testing"

func TestValidateForAppAllowsMissingAuthSecretsOutsideProduction(t *testing.T) {
	cfg := Config{
		App: AppConfig{Env: "development"},
		Auth: AuthConfig{
			Secret:        "",
			RefreshSecret: "",
		},
	}

	if err := cfg.ValidateForApp(); err != nil {
		t.Fatalf("expected development config to be valid, got %v", err)
	}
}

func TestValidateForAppRequiresProductionAuthSecrets(t *testing.T) {
	cfg := Config{
		App: AppConfig{Env: "production"},
	}

	if err := cfg.ValidateForApp(); err == nil {
		t.Fatal("expected missing production auth secrets to be invalid")
	}
}

func TestValidateForAppRejectsMatchingProductionAuthSecrets(t *testing.T) {
	secret := "12345678901234567890123456789012"
	cfg := Config{
		App: AppConfig{Env: "production"},
		Auth: AuthConfig{
			Secret:        secret,
			RefreshSecret: secret,
		},
		MultiTenant: validProductionMultiTenantConfig(),
	}

	if err := cfg.ValidateForApp(); err == nil {
		t.Fatal("expected matching production auth secrets to be invalid")
	}
}

func TestValidateForAppRequiresProductionMultiTenantConfig(t *testing.T) {
	cfg := Config{
		App: AppConfig{Env: "production"},
		Auth: AuthConfig{
			Secret:        "12345678901234567890123456789012",
			RefreshSecret: "abcdefghijklmnopqrstuvwxyz123456",
		},
	}

	if err := cfg.ValidateForApp(); err == nil {
		t.Fatal("expected missing production multi-tenant config to be invalid")
	}
}

func TestValidateForAppAllowsMissingAppSecretOutsideProduction(t *testing.T) {
	cfg := Config{
		App: AppConfig{Env: "development", Secret: ""},
	}

	if err := cfg.App.validate(cfg.App.Env); err != nil {
		t.Fatalf("expected development app config to be valid, got %v", err)
	}
}

func TestAppConfigRequiresProductionSecret(t *testing.T) {
	cfg := AppConfig{Env: "production", Secret: ""}

	if err := cfg.validate("production"); err == nil {
		t.Fatal("expected missing production app secret to be invalid")
	}
}

func TestAppConfigRejectsShortProductionSecret(t *testing.T) {
	cfg := AppConfig{Env: "production", Secret: "too-short"}

	if err := cfg.validate("production"); err == nil {
		t.Fatal("expected short production app secret to be invalid")
	}
}

func TestAppConfigAcceptsValidProductionSecret(t *testing.T) {
	cfg := AppConfig{Env: "production", Secret: "12345678901234567890123456789012"}

	if err := cfg.validate("production"); err != nil {
		t.Fatalf("expected valid production app secret to pass, got %v", err)
	}
}

func TestMultiTenantConfigRejectsForwardedHostWithoutTrustedProxy(t *testing.T) {
	cfg := MultiTenantConfig{
		DefaultDataPlacement: "shared",
		TrustForwardedHost:   true,
	}

	if err := cfg.validate("development"); err == nil {
		t.Fatal("expected forwarded host without trusted proxy to be invalid")
	}
}

func TestMultiTenantConfigRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	cfg := MultiTenantConfig{
		DefaultDataPlacement: "shared",
		TrustedProxyCIDRs:    []string{"not-a-cidr"},
	}

	if err := cfg.validate("development"); err == nil {
		t.Fatal("expected invalid trusted proxy CIDR to be rejected")
	}
}

func TestMultiTenantConfigAcceptsValidProductionConfig(t *testing.T) {
	cfg := validProductionMultiTenantConfig()
	cfg.TrustForwardedHost = true
	cfg.TrustedProxyCIDRs = []string{"10.0.0.0/8", "2001:db8::/32"}

	if err := cfg.validate("production"); err != nil {
		t.Fatalf("expected production multi-tenant config to be valid, got %v", err)
	}
}

func TestGoogleAuthConfigRequiresClientIDWhenEnabled(t *testing.T) {
	cfg := GoogleAuthConfig{
		Enabled:       true,
		AutoRegister:  true,
		DefaultRole:   "member",
		DefaultStatus: "active",
	}

	if err := cfg.validate("development"); err == nil {
		t.Fatal("expected enabled google auth without client id to be invalid")
	}
}

func TestGoogleAuthConfigAcceptsActiveAutoRegister(t *testing.T) {
	cfg := GoogleAuthConfig{
		Enabled:               true,
		ClientIDs:             []string{"google-client-id.apps.googleusercontent.com"},
		AutoRegister:          true,
		AutoLinkVerifiedEmail: true,
		DefaultRole:           "member",
		DefaultStatus:         "active",
	}

	if err := cfg.validate("production"); err != nil {
		t.Fatalf("expected google auth config to be valid, got %v", err)
	}
}

func validProductionMultiTenantConfig() MultiTenantConfig {
	return MultiTenantConfig{
		PlatformOrganizationID:   "00000000-0000-0000-0000-000000000001",
		PlatformOrganizationSlug: "zyad-cloud",
		PlatformOrganizationName: "Zyad Cloud",
		PlatformPrimaryDomain:    "zyad.cloud",
		ReservedSubdomains:       []string{"www", "api", "app", "admin"},
		DefaultDataPlacement:     "shared",
	}
}

func TestSeedAdminConfigValidateRequiresCoreFields(t *testing.T) {
	cfg := SeedAdminConfig{
		Name:     "Super Admin",
		Username: "superadmin",
		Email:    "admin@example.com",
		Password: "secret",
		Role:     "super_admin",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected seed admin config to be valid, got %v", err)
	}
}

func TestSeedAdminConfigValidateRejectsMissingCoreFields(t *testing.T) {
	var cfg SeedAdminConfig

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing seed admin config to be invalid")
	}
}
