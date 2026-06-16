package service

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type selfStoreStub struct {
	organization model.Organization
	updateParams repository.UpdateCurrentOrganizationParams
}

func (s *selfStoreStub) FindByID(
	context.Context,
	string,
) (model.Organization, error) {
	return s.organization, nil
}

func (s *selfStoreStub) Update(
	_ context.Context,
	_ string,
	params repository.UpdateCurrentOrganizationParams,
) (model.Organization, error) {
	s.updateParams = params
	return s.organization, nil
}

type membershipDetailStoreStub struct {
	details []repository.MembershipDetail
	total   int64
	filter  repository.MembershipListFilter
}

func (s *membershipDetailStoreStub) ListDetailsByOrganization(
	_ context.Context,
	_ string,
	filter repository.MembershipListFilter,
) ([]repository.MembershipDetail, int64, error) {
	s.filter = filter
	return s.details, s.total, nil
}

func TestSelfServiceUpdateValidatesTimezone(t *testing.T) {
	store := &selfStoreStub{}
	service := NewSelfService(store, nil, nil)
	timezone := "Mars/Olympus"

	_, err := service.Update(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		dto.UpdateCurrentOrganizationRequest{Timezone: &timezone},
	)
	if err == nil {
		t.Fatal("Update() error = nil, want invalid timezone")
	}
}

func TestSelfServiceUpdatePassesVerifiedActorContext(t *testing.T) {
	store := &selfStoreStub{organization: model.Organization{
		ID:        "11111111-1111-1111-1111-111111111111",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}
	service := NewSelfService(store, nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	}
	name := "Updated Organization"

	_, err := service.Update(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		dto.UpdateCurrentOrganizationRequest{Name: &name},
	)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if store.updateParams.ActorUserID != "33333333-3333-3333-3333-333333333333" ||
		store.updateParams.MembershipID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("Update() params = %#v", store.updateParams)
	}
}

func TestSelfServiceListMembersBuildsPaginationAndDetails(t *testing.T) {
	details := &membershipDetailStoreStub{
		total: 21,
		details: []repository.MembershipDetail{{
			Membership: model.Membership{
				ID:             "44444444-4444-4444-4444-444444444444",
				OrganizationID: "11111111-1111-1111-1111-111111111111",
				UserID:         "33333333-3333-3333-3333-333333333333",
				Status:         model.MembershipStatusActive,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			UserName:  "Member",
			UserEmail: "member@example.test",
			RoleIDs:   []string{"55555555-5555-5555-5555-555555555555"},
			RoleSlugs: []string{"tenant_admin"},
		}},
	}
	service := NewSelfService(nil, nil, details)

	items, meta, err := service.ListMembers(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		dto.MembershipListQuery{Page: 2, PerPage: 10, Status: "active"},
	)
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if meta.TotalPages != 3 || details.filter.Offset != 10 ||
		items[0].UserEmail != "member@example.test" ||
		len(items[0].RoleIDs) != 1 {
		t.Fatalf("ListMembers() items=%#v meta=%#v filter=%#v", items, meta, details.filter)
	}
}
