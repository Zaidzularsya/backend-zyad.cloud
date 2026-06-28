package service

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakeOnboardingStore struct {
	params repository.CreateWorkspaceParams
	result repository.MembershipOrganization
	err    error
}

func (s *fakeOnboardingStore) CreateWorkspace(
	_ context.Context,
	params repository.CreateWorkspaceParams,
) (repository.MembershipOrganization, error) {
	s.params = params
	return s.result, s.err
}

func TestOnboardingServiceCreateWorkspaceDerivesSlugAndContext(t *testing.T) {
	store := &fakeOnboardingStore{
		result: repository.MembershipOrganization{
			Organization: model.Organization{
				ID:        "11111111-1111-1111-1111-111111111111",
				Slug:      "hey-digital-solution",
				Name:      "Hey Digital Solution",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Membership: model.Membership{
				ID:             "22222222-2222-2222-2222-222222222222",
				OrganizationID: "11111111-1111-1111-1111-111111111111",
				UserID:         "33333333-3333-3333-3333-333333333333",
				Status:         model.MembershipStatusActive,
				IsOwner:        true,
				Version:        2,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			RoleSlugs: []string{"organization_owner"},
		},
	}
	service := NewOnboardingService(store)
	service.now = func() time.Time {
		return time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
	}

	response, err := service.CreateWorkspace(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		dto.CreateWorkspaceRequest{Name: " Hey Digital Solution "},
		OnboardingMetadata{RequestID: "req-1", IPAddress: "127.0.0.1", UserAgent: "test"},
	)
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if store.params.Slug != "hey-digital-solution" ||
		store.params.Name != "Hey Digital Solution" ||
		store.params.RequestID != "req-1" ||
		!store.params.CorrelationAt.Equal(time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("CreateWorkspace() params = %#v", store.params)
	}
	if !response.CurrentOrganization.Membership.IsOwner ||
		response.CurrentOrganization.Membership.RoleSlugs[0] != "organization_owner" {
		t.Fatalf("CreateWorkspace() response = %#v", response)
	}
}

func TestOnboardingServiceCreateWorkspaceRejectsInvalidSlug(t *testing.T) {
	service := NewOnboardingService(&fakeOnboardingStore{})
	_, err := service.CreateWorkspace(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		dto.CreateWorkspaceRequest{Name: "!!!"},
		OnboardingMetadata{},
	)
	if err == nil {
		t.Fatal("CreateWorkspace() error = nil, want invalid slug error")
	}
}
