package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c Config) ValidateForApp() error {
	if err := c.Auth.validate(c.App.Env); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}
	return nil
}

func (c SeedAdminConfig) Validate() error {
	var missing []string
	if c.Name == "" {
		missing = append(missing, "SEED_ADMIN_NAME")
	}
	if c.Username == "" {
		missing = append(missing, "SEED_ADMIN_USERNAME")
	}
	if c.Email == "" {
		missing = append(missing, "SEED_ADMIN_EMAIL")
	}
	if c.Password == "" {
		missing = append(missing, "SEED_ADMIN_PASSWORD")
	}
	if c.Role == "" {
		missing = append(missing, "SEED_ADMIN_ROLE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required seed admin config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c AuthConfig) validate(env string) error {
	if !isProduction(env) {
		return nil
	}

	var missing []string
	if c.Secret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if c.RefreshSecret == "" {
		missing = append(missing, "JWT_REFRESH_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required production auth config: %s", strings.Join(missing, ", "))
	}

	if len(c.Secret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters in production")
	}
	if len(c.RefreshSecret) < 32 {
		return errors.New("JWT_REFRESH_SECRET must be at least 32 characters in production")
	}
	if c.Secret == c.RefreshSecret {
		return errors.New("JWT_SECRET and JWT_REFRESH_SECRET must be different in production")
	}

	return nil
}

func isProduction(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}
