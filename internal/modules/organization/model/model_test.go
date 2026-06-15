package model

import (
	"testing"
	"time"
)

func TestOrganizationModelValues(t *testing.T) {
	if !MembershipStatusActive.IsValid() {
		t.Fatal("expected active membership status")
	}
	if !DomainTypeCustom.IsValid() {
		t.Fatal("expected custom domain type")
	}
	if !DomainStatusVerified.IsValid() {
		t.Fatal("expected verified domain status")
	}
	if !DomainSSLStatusProvisioning.IsValid() {
		t.Fatal("expected provisioning SSL status")
	}
	if !EntitlementSourcePlatformOverride.IsValid() {
		t.Fatal("expected platform override source")
	}
	if !EntitlementStatusActive.IsValid() {
		t.Fatal("expected active entitlement status")
	}
}

func TestEntitlementIsEffective(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	entitlement := Entitlement{
		Status:         EntitlementStatusActive,
		EffectiveFrom:  now.Add(-time.Hour),
		EffectiveUntil: &expiresAt,
	}

	if !entitlement.IsEffective(now) {
		t.Fatal("expected entitlement to be effective")
	}
	if entitlement.IsEffective(expiresAt) {
		t.Fatal("expected entitlement to expire at effective_until")
	}
}

func TestOrganizationDomainCanResolvePublicly(t *testing.T) {
	domain := OrganizationDomain{
		Status:    DomainStatusActive,
		SSLStatus: DomainSSLStatusActive,
	}
	if !domain.CanResolvePublicly() {
		t.Fatal("expected active domain to resolve publicly")
	}

	deletedAt := time.Now()
	domain.DeletedAt = &deletedAt
	if domain.CanResolvePublicly() {
		t.Fatal("expected deleted domain not to resolve publicly")
	}
}

func TestUsageCounterContains(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	counter := UsageCounter{PeriodStart: start, PeriodEnd: end}

	if !counter.Contains(start) {
		t.Fatal("expected period start to be included")
	}
	if counter.Contains(end) {
		t.Fatal("expected period end to be excluded")
	}
}

func TestImpersonationSessionIsActive(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	session := ImpersonationSession{ExpiresAt: now.Add(time.Hour)}

	if !session.IsActive(now) {
		t.Fatal("expected impersonation session to be active")
	}
	if session.IsActive(session.ExpiresAt) {
		t.Fatal("expected impersonation session to expire at expires_at")
	}

	stoppedAt := now.Add(time.Minute)
	session.StoppedAt = &stoppedAt
	if session.IsActive(now) {
		t.Fatal("expected stopped impersonation session to be inactive")
	}
}
