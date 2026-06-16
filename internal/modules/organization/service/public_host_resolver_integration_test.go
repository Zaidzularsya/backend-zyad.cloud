//go:build integration

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestPublicHostResolverCustomDomainAndStatusIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("public-host"), ".", "-")

	organizationRepo := repository.NewOrganizationRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	resolver := NewPublicHostResolver(
		repository.NewPublicHostResolverRepository(db),
		"",
		"",
	)

	organization, err := organizationRepo.Create(ctx, repository.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "public-host-" + suffix,
		Name:          "Public Host Resolver Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	host := "landing-" + suffix + ".example.test"
	domain, err := domainRepo.Create(ctx, repository.CreateDomainParams{
		OrganizationID:            organization.ID,
		Type:                      model.DomainTypeCustom,
		CanonicalHost:             host,
		VerificationChallengeHash: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	verifiedAt := time.Now().UTC()
	if _, err := domainRepo.UpdateVerification(
		ctx,
		organization.ID,
		domain.ID,
		repository.UpdateDomainVerificationParams{
			Status:      model.DomainStatusVerified,
			VerifiedAt:  &verifiedAt,
			AttemptedAt: verifiedAt,
		},
	); err != nil {
		t.Fatalf("verify domain: %v", err)
	}
	if _, err := domainRepo.Activate(
		ctx,
		organization.ID,
		domain.ID,
		true,
		verifiedAt,
		"",
	); err != nil {
		t.Fatalf("activate domain: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_domains WHERE id = $1`, domain.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	tenantContext, ok, err := resolver.ResolvePublicHost(ctx, strings.ToUpper(host)+".")
	if err != nil {
		t.Fatalf("ResolvePublicHost() error = %v", err)
	}
	if !ok || tenantContext.OrganizationID() != organization.ID ||
		tenantContext.ResolutionSource() != coretenant.ResolutionSourceCustomDomain {
		t.Fatalf("resolved context = %#v, %v", tenantContext, ok)
	}

	if _, err := organizationRepo.UpdateStatus(
		ctx,
		organization.ID,
		coretenant.OrganizationStatusSuspended,
	); err != nil {
		t.Fatalf("suspend organization: %v", err)
	}
	_, ok, err = resolver.ResolvePublicHost(ctx, host)
	if err != nil || ok {
		t.Fatalf("suspended organization resolution = %v, %v", ok, err)
	}
}
