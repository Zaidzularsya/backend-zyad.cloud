package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var hostnameLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (c Config) ValidateForApp() error {
	if err := c.Auth.validate(c.App.Env); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}
	if err := c.MultiTenant.validate(c.App.Env); err != nil {
		return fmt.Errorf("multi-tenant config: %w", err)
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

func (c MultiTenantConfig) validate(env string) error {
	if c.DefaultDataPlacement != "" &&
		c.DefaultDataPlacement != "shared" &&
		c.DefaultDataPlacement != "dedicated" {
		return errors.New("TENANT_DEFAULT_DATA_PLACEMENT must be shared or dedicated")
	}

	if c.PlatformOrganizationSlug != "" && !isValidHostnameLabel(c.PlatformOrganizationSlug) {
		return errors.New("PLATFORM_ORGANIZATION_SLUG must be a lowercase DNS label")
	}
	if c.PlatformPrimaryDomain != "" && !isValidHostname(c.PlatformPrimaryDomain) {
		return errors.New("PLATFORM_PRIMARY_DOMAIN must be a hostname without scheme, port, or path")
	}

	for _, subdomain := range c.ReservedSubdomains {
		if !isValidHostnameLabel(subdomain) {
			return fmt.Errorf("PLATFORM_RESERVED_SUBDOMAINS contains invalid label %q", subdomain)
		}
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR %q", cidr)
		}
	}
	if c.TrustForwardedHost && len(c.TrustedProxyCIDRs) == 0 {
		return errors.New("TRUST_FORWARDED_HOST requires at least one TRUSTED_PROXY_CIDRS value")
	}

	if !isProduction(env) {
		return nil
	}

	var missing []string
	if c.PlatformOrganizationID == "" {
		missing = append(missing, "PLATFORM_ORGANIZATION_ID")
	}
	if c.PlatformOrganizationSlug == "" {
		missing = append(missing, "PLATFORM_ORGANIZATION_SLUG")
	}
	if c.PlatformOrganizationName == "" {
		missing = append(missing, "PLATFORM_ORGANIZATION_NAME")
	}
	if c.PlatformPrimaryDomain == "" {
		missing = append(missing, "PLATFORM_PRIMARY_DOMAIN")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required production config: %s", strings.Join(missing, ", "))
	}

	return nil
}

func isValidHostname(value string) bool {
	if len(value) > 253 || strings.ContainsAny(value, "/:") {
		return false
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !isValidHostnameLabel(label) {
			return false
		}
	}
	return true
}

func isValidHostnameLabel(value string) bool {
	return hostnameLabelPattern.MatchString(value)
}

func isProduction(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}
