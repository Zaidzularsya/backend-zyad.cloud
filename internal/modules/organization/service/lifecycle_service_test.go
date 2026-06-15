package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakeLifecycleStore struct {
	bundle       repository.OrganizationBundle
	createParams repository.CreateOrganizationBundleParams
	createErr    error
	statusResult model.Organization
	statusErr    error
}

func (f *fakeLifecycleStore) CreateBundle(
	_ context.Context,
	params repository.CreateOrganizationBundleParams,
) (repository.OrganizationBundle, error) {
	f.createParams = params
	return f.bundle, f.createErr
}

func (f *fakeLifecycleStore) ChangeStatus(
	_ context.Context,
	_ string,
	_ coretenant.OrganizationStatus,
	_ coretenant.OrganizationStatus,
	_ string,
	_ string,
	_ time.Time,
) (model.Organization, error) {
	return f.statusResult, f.statusErr
}

func TestLifecycleServiceCreateNormalizesProvisioningBundle(t *testing.T) {
	store := &fakeLifecycleStore{
		bundle: repository.OrganizationBundle{
			Organization: model.Organization{ID: "organization-1"},
		},
	}
	service := NewLifecycleService(store)
	service.now = func() time.Time {
		return time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	}

	_, err := service.Create(context.Background(), CreateOrganizationInput{
		Type:        coretenant.OrganizationTypeCustomer,
		Slug:        "  ACME  ",
		Name:        " Acme ",
		OwnerUserID: "11111111-1111-1111-1111-111111111111",
		Plan: []repository.UpsertEntitlementParams{
			{FeatureKey: " Landing.Enabled "},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.createParams.Organization.Status != coretenant.OrganizationStatusProvisioning ||
		store.createParams.Organization.Slug != "acme" ||
		store.createParams.Plan[0].Source != model.EntitlementSourcePlan ||
		store.createParams.Plan[0].FeatureKey != "landing.enabled" {
		t.Fatalf("Create() params = %#v", store.createParams)
	}
}

func TestLifecycleServiceProtectsPlatformOrganization(t *testing.T) {
	service := NewLifecycleService(&fakeLifecycleStore{})
	_, err := service.ChangeStatus(context.Background(), model.Organization{
		ID:     "organization-1",
		Type:   coretenant.OrganizationTypePlatform,
		Status: coretenant.OrganizationStatusActive,
	}, ChangeOrganizationStatusInput{
		OrganizationID: "organization-1",
		Status:         coretenant.OrganizationStatusArchived,
		Reason:         "test",
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "PLATFORM_ORGANIZATION_PROTECTED" {
		t.Fatalf("ChangeStatus() error = %v", err)
	}
}

func TestCanTransitionOrganizationStatus(t *testing.T) {
	if !canTransitionOrganizationStatus(
		coretenant.OrganizationStatusProvisioning,
		coretenant.OrganizationStatusActive,
	) {
		t.Fatal("expected provisioning to active transition")
	}
	if canTransitionOrganizationStatus(
		coretenant.OrganizationStatusArchived,
		coretenant.OrganizationStatusActive,
	) {
		t.Fatal("archived organization must not be restored through normal lifecycle")
	}
}
