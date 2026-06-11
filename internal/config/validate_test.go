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
	}

	if err := cfg.ValidateForApp(); err == nil {
		t.Fatal("expected matching production auth secrets to be invalid")
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
