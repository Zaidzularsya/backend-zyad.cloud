//go:build integration

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestPlatformServiceProvisionAndSummaryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("platform-api"), ".", "-")

	var ownerUserID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Platform API Owner', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("platform-api-%s@example.test", suffix)).Scan(&ownerUserID); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	organizationRepository := repository.NewOrganizationRepository(db)
	lifecycleService := NewLifecycleService(repository.NewLifecycleRepository(db))
	service := NewPlatformService(organizationRepository, lifecycleService)

	created, err := service.Create(ctx, dto.CreateOrganizationRequest{
		Type:          string(coretenant.OrganizationTypeCustomer),
		Slug:          "platform-api-" + suffix,
		Name:          "Platform API Organization",
		DataPlacement: string(coretenant.DataPlacementShared),
		OwnerUserID:   ownerUserID,
	}, ownerUserID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1`, created.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, created.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, ownerUserID)
	})

	detail, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.Organization.Status != string(coretenant.OrganizationStatusProvisioning) ||
		detail.Placement.Status != "ready" ||
		detail.Health.Status != "degraded" {
		t.Fatalf("Get() detail = %#v", detail)
	}

	provisioned, err := service.Provision(ctx, created.ID, ownerUserID)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if provisioned.Organization.Status != string(coretenant.OrganizationStatusActive) ||
		provisioned.Health.Status != "healthy" {
		t.Fatalf("Provision() detail = %#v", provisioned)
	}

	var statusAuditCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE organization_id = $1
			AND event = 'organization_status_changed'
	`, created.ID).Scan(&statusAuditCount); err != nil {
		t.Fatalf("count provisioning status audit: %v", err)
	}
	if statusAuditCount != 1 {
		t.Fatalf("provisioning status audit count = %d, want 1", statusAuditCount)
	}
}
