package cache

import (
	"errors"
	"fmt"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
)

const DefaultPrefix = "zyad"

var ErrInvalidKeyInput = errors.New("invalid cache key input")

type KeyBuilder struct {
	prefix string
}

type VersionToken struct {
	MembershipVersion  int64
	DomainVersion      int64
	EntitlementVersion int64
	PageVersion        int64
}

func NewKeyBuilder(prefix string) KeyBuilder {
	prefix = canonicalSegment(prefix)
	if prefix == "" {
		prefix = DefaultPrefix
	}
	return KeyBuilder{prefix: prefix}
}

func (b KeyBuilder) TenantKey(
	scope coretenant.Scope,
	namespace string,
	segments ...string,
) (string, error) {
	if !scope.IsValid() {
		return "", ErrInvalidKeyInput
	}
	namespace = canonicalSegment(namespace)
	if namespace == "" {
		return "", ErrInvalidKeyInput
	}
	parts := []string{
		b.prefix,
		"org",
		scope.OrganizationID(),
		namespace,
	}
	for _, segment := range segments {
		segment = canonicalSegment(segment)
		if segment == "" {
			return "", ErrInvalidKeyInput
		}
		parts = append(parts, segment)
	}
	return strings.Join(parts, ":"), nil
}

func (b KeyBuilder) DomainKey(
	scope coretenant.Scope,
	host string,
	domainVersion int64,
) (string, error) {
	if domainVersion <= 0 {
		return "", ErrInvalidKeyInput
	}
	return b.TenantKey(
		scope,
		"domain",
		host,
		versionSegment("v", domainVersion),
	)
}

func (b KeyBuilder) PermissionKey(
	scope coretenant.Scope,
	userID string,
	membershipID string,
	membershipVersion int64,
) (string, error) {
	if membershipVersion <= 0 {
		return "", ErrInvalidKeyInput
	}
	return b.TenantKey(
		scope,
		"permission",
		"user",
		userID,
		"membership",
		membershipID,
		versionSegment("v", membershipVersion),
	)
}

func (b KeyBuilder) EntitlementKey(
	scope coretenant.Scope,
	featureKey string,
	entitlementVersion int64,
) (string, error) {
	if entitlementVersion <= 0 {
		return "", ErrInvalidKeyInput
	}
	return b.TenantKey(
		scope,
		"entitlement",
		featureKey,
		versionSegment("v", entitlementVersion),
	)
}

func (b KeyBuilder) LandingPublicKey(
	scope coretenant.Scope,
	host string,
	slug string,
	pageVersion int64,
) (string, error) {
	if pageVersion <= 0 {
		return "", ErrInvalidKeyInput
	}
	return b.TenantKey(
		scope,
		"landing_public",
		host,
		slug,
		versionSegment("page", pageVersion),
	)
}

func (b KeyBuilder) InvalidationPrefix(
	scope coretenant.Scope,
	namespace string,
) (string, error) {
	return b.TenantKey(scope, namespace)
}

func canonicalSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func versionSegment(prefix string, version int64) string {
	return fmt.Sprintf("%s%d", prefix, version)
}
