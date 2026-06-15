//go:build integration

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestWorkerResolverStatusAndPlacementIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("worker-resolver"), ".", "-")
	organizationRepo := repository.NewOrganizationRepository(db)
	resolver := NewWorkerResolver(organizationRepo, "notification-worker")

	organization, err := organizationRepo.Create(ctx, repository.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "worker-resolver-" + suffix,
		Name:          "Worker Resolver Integration Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM organizations WHERE id = $1`,
			organization.ID,
		)
	})

	tenantContext, err := resolver.ResolveWorkerOrganization(
		ctx,
		organization.ID,
		"notification-worker",
	)
	if err != nil {
		t.Fatalf("ResolveWorkerOrganization() error = %v", err)
	}
	if tenantContext.OrganizationID() != organization.ID ||
		tenantContext.ResolutionSource() != coretenant.ResolutionSourceWorker {
		t.Fatalf("tenant context = %#v", tenantContext)
	}

	if _, err := organizationRepo.UpdateStatus(
		ctx,
		organization.ID,
		coretenant.OrganizationStatusSuspended,
	); err != nil {
		t.Fatalf("suspend organization: %v", err)
	}
	_, err = resolver.ResolveWorkerOrganization(
		ctx,
		organization.ID,
		"notification-worker",
	)
	assertWorkerResolverIntegrationErrorCode(t, err, "WORKER_ORGANIZATION_INACTIVE")

	if _, err := organizationRepo.UpdateStatus(
		ctx,
		organization.ID,
		coretenant.OrganizationStatusActive,
	); err != nil {
		t.Fatalf("restore organization: %v", err)
	}
	dedicated := coretenant.DataPlacementDedicated
	if _, err := organizationRepo.Update(
		ctx,
		organization.ID,
		repository.UpdateOrganizationParams{DataPlacement: &dedicated},
	); err != nil {
		t.Fatalf("set dedicated placement: %v", err)
	}
	_, err = resolver.ResolveWorkerOrganization(
		ctx,
		organization.ID,
		"notification-worker",
	)
	assertWorkerResolverIntegrationErrorCode(t, err, "WORKER_DATA_PLACEMENT_UNAVAILABLE")

	_, err = resolver.ResolveWorkerOrganization(
		ctx,
		"11111111-1111-1111-1111-111111111111",
		"notification-worker",
	)
	assertWorkerResolverIntegrationErrorCode(t, err, "WORKER_ORGANIZATION_NOT_FOUND")
}

func assertWorkerResolverIntegrationErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}
