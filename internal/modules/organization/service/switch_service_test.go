package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakeSwitchStore struct {
	results      []repository.MembershipOrganization
	snapshot     repository.SessionOrganizationSnapshot
	switchResult repository.MembershipOrganization
	switchParams repository.SwitchOrganizationParams
	switchErr    error
}

func (f *fakeSwitchStore) ListActiveByUser(
	context.Context,
	string,
) ([]repository.MembershipOrganization, error) {
	return f.results, nil
}

func (f *fakeSwitchStore) FindSessionSnapshot(
	context.Context,
	string,
	string,
) (repository.SessionOrganizationSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeSwitchStore) Switch(
	_ context.Context,
	params repository.SwitchOrganizationParams,
) (repository.MembershipOrganization, error) {
	f.switchParams = params
	return f.switchResult, f.switchErr
}

func TestSwitchServiceListMarksCurrentOrganization(t *testing.T) {
	result := switchTestMembershipOrganization()
	store := &fakeSwitchStore{
		results: []repository.MembershipOrganization{result},
		snapshot: repository.SessionOrganizationSnapshot{
			OrganizationID: result.Organization.ID,
		},
	}
	switchService := NewSwitchService(store)

	responses, err := switchService.List(
		context.Background(),
		resolverUserID,
		resolverSessionID,
	)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(responses) != 1 || !responses[0].IsCurrent ||
		responses[0].Organization.ID != result.Organization.ID ||
		len(responses[0].Membership.RoleIDs) != 1 ||
		responses[0].Membership.RoleIDs[0] != result.RoleIDs[0] ||
		responses[0].Membership.RoleSlugs[0] != result.RoleSlugs[0] {
		t.Fatalf("List() responses = %#v", responses)
	}
}

func TestSwitchServiceSwitchPersistsMetadata(t *testing.T) {
	result := switchTestMembershipOrganization()
	store := &fakeSwitchStore{switchResult: result}
	switchService := NewSwitchService(store)
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	switchService.now = func() time.Time { return now }

	response, err := switchService.Switch(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		dto.SwitchOrganizationRequest{OrganizationID: result.Organization.ID},
		SwitchMetadata{
			RequestID: "request-1",
			IPAddress: "192.0.2.10",
			UserAgent: "test-agent",
		},
	)
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if !response.CurrentOrganization.IsCurrent ||
		store.switchParams.SwitchedAt != now ||
		store.switchParams.RequestID != "request-1" {
		t.Fatalf("Switch() response/params = %#v / %#v", response, store.switchParams)
	}
}

func TestSwitchServiceMapsMembershipDenied(t *testing.T) {
	store := &fakeSwitchStore{switchErr: pgx.ErrNoRows}
	switchService := NewSwitchService(store)

	_, err := switchService.Switch(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		dto.SwitchOrganizationRequest{OrganizationID: resolverOrganizationID},
		SwitchMetadata{},
	)
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "ORGANIZATION_ACCESS_DENIED" {
		t.Fatalf("Switch() error = %v", err)
	}
}

func switchTestMembershipOrganization() repository.MembershipOrganization {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	return repository.MembershipOrganization{
		Organization: model.Organization{
			ID:            resolverOrganizationID,
			Type:          coretenant.OrganizationTypeCustomer,
			Slug:          "acme",
			Name:          "Acme",
			Status:        coretenant.OrganizationStatusActive,
			Timezone:      "Asia/Jakarta",
			Locale:        "id-ID",
			DataPlacement: coretenant.DataPlacementShared,
			Metadata:      map[string]any{},
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		Membership: model.Membership{
			ID:             resolverMembershipID,
			OrganizationID: resolverOrganizationID,
			UserID:         resolverUserID,
			Status:         model.MembershipStatusActive,
			Version:        2,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		RoleIDs:   []string{"55555555-5555-5555-5555-555555555555"},
		RoleSlugs: []string{"organization_admin"},
	}
}
