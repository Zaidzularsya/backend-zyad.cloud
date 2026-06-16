//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestDomainRepositoryVerificationAndResolutionIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	organizationRepo := NewOrganizationRepository(db)
	domainRepo := NewDomainRepository(db)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("domain"), ".", "-")

	organizationA, err := organizationRepo.Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "domain-a-" + suffix,
		Name:          "Domain Organization A",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization A: %v", err)
	}
	organizationB, err := organizationRepo.Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "domain-b-" + suffix,
		Name:          "Domain Organization B",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization B: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM audit_logs
			WHERE organization_id = ANY($1::uuid[])
		`, []string{organizationA.ID, organizationB.ID})
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM organization_domains
			WHERE organization_id = ANY($1::uuid[])
		`, []string{organizationA.ID, organizationB.ID})
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM organizations
			WHERE id = ANY($1::uuid[])
		`, []string{organizationA.ID, organizationB.ID})
	})

	hostA := "www-" + suffix + ".example.test"
	challengeA := strings.Repeat("a", 64)
	domain, err := domainRepo.Create(ctx, CreateDomainParams{
		OrganizationID:            organizationA.ID,
		Type:                      model.DomainTypeCustom,
		CanonicalHost:             "  " + strings.ToUpper(hostA) + ". ",
		VerificationChallengeHash: challengeA,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if domain.CanonicalHost != hostA || domain.Status != model.DomainStatusPending {
		t.Fatalf("Create() domain = %#v", domain)
	}

	if _, err := domainRepo.ResolveActiveHost(ctx, hostA); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ResolveActiveHost() pending error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := domainRepo.Activate(ctx, organizationA.ID, domain.ID, true, time.Now().UTC(), ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Activate() unverified error = %v, want pgx.ErrNoRows", err)
	}

	_, err = domainRepo.Create(ctx, CreateDomainParams{
		OrganizationID:            organizationB.ID,
		Type:                      model.DomainTypeCustom,
		CanonicalHost:             strings.ToUpper(hostA),
		VerificationChallengeHash: strings.Repeat("b", 64),
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "idx_organization_domains_canonical_host_unique" {
		t.Fatalf("Create() duplicate error = %v, want host unique violation", err)
	}

	verifiedAt := time.Now().UTC()
	domain, err = domainRepo.UpdateVerification(ctx, organizationA.ID, domain.ID, UpdateDomainVerificationParams{
		Status:      model.DomainStatusVerified,
		VerifiedAt:  &verifiedAt,
		AttemptedAt: verifiedAt,
	})
	if err != nil {
		t.Fatalf("UpdateVerification() error = %v", err)
	}
	if domain.VerifiedAt == nil || domain.VerificationAttempts != 1 {
		t.Fatalf("UpdateVerification() domain = %#v", domain)
	}

	domain, err = domainRepo.SetPrimary(ctx, organizationA.ID, domain.ID, verifiedAt.Add(time.Second), "")
	if err != nil {
		t.Fatalf("SetPrimary() error = %v", err)
	}
	if domain.Status != model.DomainStatusActive || !domain.IsPrimary {
		t.Fatalf("SetPrimary() domain = %#v", domain)
	}

	resolved, err := domainRepo.ResolveActiveHost(ctx, strings.ToUpper(hostA)+".")
	if err != nil {
		t.Fatalf("ResolveActiveHost() error = %v", err)
	}
	if resolved.Organization.ID != organizationA.ID || resolved.Domain.ID != domain.ID {
		t.Fatalf("ResolveActiveHost() result = %#v", resolved)
	}

	if _, err := organizationRepo.UpdateStatus(
		ctx,
		organizationA.ID,
		coretenant.OrganizationStatusSuspended,
	); err != nil {
		t.Fatalf("suspend organization: %v", err)
	}
	if _, err := domainRepo.ResolveActiveHost(ctx, hostA); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ResolveActiveHost() suspended error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := organizationRepo.UpdateStatus(
		ctx,
		organizationA.ID,
		coretenant.OrganizationStatusActive,
	); err != nil {
		t.Fatalf("restore organization: %v", err)
	}

	hostB := "new-" + suffix + ".example.test"
	challengeB := strings.Repeat("c", 64)
	domain, err = domainRepo.Reassign(
		ctx,
		domain.ID,
		organizationA.ID,
		organizationB.ID,
		model.DomainTypeCustom,
		hostB,
		challengeB,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("Reassign() error = %v", err)
	}
	if domain.OrganizationID != organizationB.ID ||
		domain.Status != model.DomainStatusPending ||
		domain.VerifiedAt != nil ||
		domain.IsPrimary ||
		domain.VerificationChallengeHash != challengeB ||
		domain.VerificationAttempts != 0 ||
		domain.SSLStatus != model.DomainSSLStatusPending {
		t.Fatalf("Reassign() retained verification state: %#v", domain)
	}
	if _, err := domainRepo.ResolveActiveHost(ctx, hostA); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ResolveActiveHost() old host error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := domainRepo.ResolveActiveHost(ctx, hostB); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ResolveActiveHost() reassigned pending error = %v, want pgx.ErrNoRows", err)
	}

	domains, total, err := domainRepo.ListByOrganization(ctx, organizationB.ID, DomainListFilter{})
	if err != nil {
		t.Fatalf("ListByOrganization() error = %v", err)
	}
	if total != 1 || len(domains) != 1 || domains[0].ID != domain.ID {
		t.Fatalf("ListByOrganization() total = %d, domains = %#v", total, domains)
	}

	if err := domainRepo.Delete(ctx, organizationB.ID, domain.ID, time.Now().UTC(), ""); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := domainRepo.FindByID(ctx, organizationB.ID, domain.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("FindByID() deleted error = %v, want pgx.ErrNoRows", err)
	}
}

func TestDomainRepositoryRejectsReservedSubdomainIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	organizationRepo := NewOrganizationRepository(db)
	domainRepo := NewDomainRepository(db)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("reserved"), ".", "-")
	label := "reserved-" + suffix

	organization, err := organizationRepo.Create(ctx, CreateOrganizationParams{
		Type:   coretenant.OrganizationTypeCustomer,
		Slug:   "reserved-org-" + suffix,
		Name:   "Reserved Domain Organization",
		Status: coretenant.OrganizationStatusActive,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO reserved_subdomains (label, reason)
		VALUES ($1, 'integration test')
	`, label); err != nil {
		t.Fatalf("create reserved subdomain: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_domains WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM reserved_subdomains WHERE label = $1`, label)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	_, err = domainRepo.Create(ctx, CreateDomainParams{
		OrganizationID:            organization.ID,
		Type:                      model.DomainTypeSubdomain,
		CanonicalHost:             label + ".example.test",
		VerificationChallengeHash: strings.Repeat("d", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "is reserved") {
		t.Fatalf("Create() reserved subdomain error = %v", err)
	}
}
