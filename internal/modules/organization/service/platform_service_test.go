package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakePlatformOrganizationStore struct {
	organizations []model.Organization
	total         int64
	filter        repository.OrganizationListFilter
	current       model.Organization
	summary       repository.OrganizationControlPlaneSummary
	updateParams  repository.UpdateOrganizationParams
}

func (f *fakePlatformOrganizationStore) List(
	_ context.Context,
	filter repository.OrganizationListFilter,
) ([]model.Organization, int64, error) {
	f.filter = filter
	return f.organizations, f.total, nil
}

func (f *fakePlatformOrganizationStore) FindByIDIncludingArchived(
	context.Context,
	string,
) (model.Organization, error) {
	return f.current, nil
}

func (f *fakePlatformOrganizationStore) ControlPlaneSummary(
	context.Context,
	string,
) (repository.OrganizationControlPlaneSummary, error) {
	return f.summary, nil
}

func (f *fakePlatformOrganizationStore) Update(
	_ context.Context,
	_ string,
	params repository.UpdateOrganizationParams,
) (model.Organization, error) {
	f.updateParams = params
	return f.current, nil
}

func TestPlatformServiceListNormalizesPagination(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		organizations: []model.Organization{{ID: "organization-1"}},
		total:         101,
	}
	service := NewPlatformService(store, nil)
	items, meta, err := service.List(context.Background(), dto.OrganizationListQuery{
		Page:    2,
		PerPage: 500,
		Type:    "customer",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || meta.PerPage != 100 || meta.TotalPages != 2 {
		t.Fatalf("List() items=%#v meta=%#v", items, meta)
	}
	if store.filter.Offset != 100 || store.filter.Type != coretenant.OrganizationTypeCustomer {
		t.Fatalf("List() filter = %#v", store.filter)
	}
}

func TestPlatformServiceRejectsPlatformPlacementUpdate(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		current: model.Organization{
			ID:     "11111111-1111-1111-1111-111111111111",
			Type:   coretenant.OrganizationTypePlatform,
			Status: coretenant.OrganizationStatusActive,
		},
	}
	service := NewPlatformService(store, nil)
	placement := "dedicated"
	_, err := service.Update(
		context.Background(),
		store.current.ID,
		dto.UpdateOrganizationRequest{DataPlacement: &placement},
	)
	if err == nil {
		t.Fatal("Update() unexpectedly succeeded")
	}
}

func TestPlatformServiceProvisionSharedOrganization(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		current: model.Organization{
			ID:            "11111111-1111-1111-1111-111111111111",
			Type:          coretenant.OrganizationTypeCustomer,
			Status:        coretenant.OrganizationStatusProvisioning,
			DataPlacement: coretenant.DataPlacementShared,
		},
		summary: repository.OrganizationControlPlaneSummary{ActiveOwnerCount: 1},
	}
	lifecycleStore := &fakeLifecycleStore{
		statusResult: model.Organization{
			ID:            store.current.ID,
			Type:          store.current.Type,
			Status:        coretenant.OrganizationStatusActive,
			DataPlacement: coretenant.DataPlacementShared,
		},
	}
	service := NewPlatformService(store, NewLifecycleService(lifecycleStore))
	result, err := service.Provision(
		context.Background(),
		store.current.ID,
		"22222222-2222-2222-2222-222222222222",
	)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if result.Organization.Status != string(coretenant.OrganizationStatusActive) ||
		result.Health.Status != "healthy" {
		t.Fatalf("Provision() result = %#v", result)
	}
}

func TestPlatformServiceRejectsDedicatedProvisioning(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		current: model.Organization{
			ID:            "11111111-1111-1111-1111-111111111111",
			Status:        coretenant.OrganizationStatusProvisioning,
			DataPlacement: coretenant.DataPlacementDedicated,
		},
	}
	service := NewPlatformService(store, NewLifecycleService(&fakeLifecycleStore{}))
	_, err := service.Provision(
		context.Background(),
		store.current.ID,
		"22222222-2222-2222-2222-222222222222",
	)
	if err == nil {
		t.Fatal("Provision() unexpectedly succeeded")
	}
}

func TestPlatformServiceProvisionRequiresActiveOwner(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		current: model.Organization{
			ID:            "11111111-1111-1111-1111-111111111111",
			Status:        coretenant.OrganizationStatusProvisioning,
			DataPlacement: coretenant.DataPlacementShared,
		},
	}
	service := NewPlatformService(store, NewLifecycleService(&fakeLifecycleStore{}))
	_, err := service.Provision(
		context.Background(),
		store.current.ID,
		"22222222-2222-2222-2222-222222222222",
	)
	if err == nil {
		t.Fatal("Provision() unexpectedly succeeded")
	}
}

func TestPlatformServiceRetriesFailedProvisioningThroughProvisioningState(t *testing.T) {
	store := &fakePlatformOrganizationStore{
		current: model.Organization{
			ID:            "11111111-1111-1111-1111-111111111111",
			Type:          coretenant.OrganizationTypeCustomer,
			Status:        coretenant.OrganizationStatusProvisioningFailed,
			DataPlacement: coretenant.DataPlacementShared,
		},
		summary: repository.OrganizationControlPlaneSummary{ActiveOwnerCount: 1},
	}
	lifecycleStore := &fakeLifecycleStore{
		statusResults: []model.Organization{
			{
				ID:            store.current.ID,
				Status:        coretenant.OrganizationStatusProvisioning,
				DataPlacement: coretenant.DataPlacementShared,
			},
			{
				ID:            store.current.ID,
				Status:        coretenant.OrganizationStatusActive,
				DataPlacement: coretenant.DataPlacementShared,
			},
		},
	}
	service := NewPlatformService(store, NewLifecycleService(lifecycleStore))
	result, err := service.Provision(
		context.Background(),
		store.current.ID,
		"22222222-2222-2222-2222-222222222222",
	)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if lifecycleStore.statusCalls != 2 ||
		result.Organization.Status != string(coretenant.OrganizationStatusActive) {
		t.Fatalf("statusCalls=%d result=%#v", lifecycleStore.statusCalls, result)
	}
}

func TestOrganizationHealthIssuesAreDeterministic(t *testing.T) {
	issues := organizationHealthIssues(
		model.Organization{Status: coretenant.OrganizationStatusSuspended},
		repository.OrganizationControlPlaneSummary{
			FailedDomainCount:    1,
			SSLFailedDomainCount: 1,
		},
		false,
	)
	want := []string{
		"data placement is not available",
		"active organization owner is missing",
		"organization is not active",
		"one or more domains failed verification",
		"one or more domains failed SSL provisioning",
	}
	if len(issues) != len(want) {
		t.Fatalf("issues = %#v", issues)
	}
	for index := range want {
		if issues[index] != want[index] {
			t.Fatalf("issues = %#v, want %#v", issues, want)
		}
	}
}
