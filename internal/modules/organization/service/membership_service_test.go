package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakeMembershipStore struct {
	inviteParams   repository.InviteMembershipParams
	inviteResult   model.Membership
	inviteErr      error
	acceptParams   repository.AcceptMembershipParams
	acceptResult   model.Membership
	statusParams   repository.ChangeMembershipStatusParams
	statusResult   model.Membership
	statusErr      error
	roleParams     repository.SyncMembershipRolesParams
	transferParams repository.TransferMembershipOwnershipParams
}

func (f *fakeMembershipStore) Invite(
	_ context.Context,
	params repository.InviteMembershipParams,
) (model.Membership, error) {
	f.inviteParams = params
	return f.inviteResult, f.inviteErr
}

func (f *fakeMembershipStore) Accept(
	_ context.Context,
	params repository.AcceptMembershipParams,
) (model.Membership, error) {
	f.acceptParams = params
	return f.acceptResult, nil
}

func (f *fakeMembershipStore) ChangeStatus(
	_ context.Context,
	params repository.ChangeMembershipStatusParams,
) (model.Membership, error) {
	f.statusParams = params
	return f.statusResult, f.statusErr
}

func (f *fakeMembershipStore) SyncRoles(
	_ context.Context,
	params repository.SyncMembershipRolesParams,
) (model.Membership, error) {
	f.roleParams = params
	return model.Membership{}, nil
}

func (f *fakeMembershipStore) TransferOwnership(
	_ context.Context,
	params repository.TransferMembershipOwnershipParams,
) error {
	f.transferParams = params
	return nil
}

func (f *fakeMembershipStore) ListByOrganization(
	_ context.Context,
	_ string,
	_ repository.MembershipListFilter,
) ([]model.Membership, int64, error) {
	return nil, 0, nil
}

func TestMembershipServiceInviteHashesTokenAndNormalizesRoles(t *testing.T) {
	store := &fakeMembershipStore{
		inviteResult: model.Membership{ID: "membership-1"},
	}
	service := NewMembershipService(store)
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.generateToken = func(int) (string, error) { return "plain-token", nil }
	roleID := "33333333-3333-3333-3333-333333333333"

	result, err := service.Invite(context.Background(), InviteMemberInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111",
		Email:          " MEMBER@EXAMPLE.TEST ",
		RoleIDs:        []string{roleID, roleID},
		ActorUserID:    "22222222-2222-2222-2222-222222222222",
	})
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	if result.Token != "plain-token" ||
		store.inviteParams.InvitationTokenHash == result.Token {
		t.Fatalf("Invite() token result = %#v, params = %#v", result, store.inviteParams)
	}
	if store.inviteParams.Email != "member@example.test" ||
		len(store.inviteParams.RoleIDs) != 1 ||
		!store.inviteParams.InvitationExpiresAt.Equal(now.Add(defaultInvitationLifetime)) {
		t.Fatalf("Invite() params = %#v", store.inviteParams)
	}
}

func TestMembershipServiceRequiresReasonForSuspension(t *testing.T) {
	service := NewMembershipService(&fakeMembershipStore{})
	_, err := service.ChangeStatus(context.Background(), ChangeMembershipStatusInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111",
		MembershipID:   "22222222-2222-2222-2222-222222222222",
		Status:         model.MembershipStatusSuspended,
		ActorUserID:    "33333333-3333-3333-3333-333333333333",
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("ChangeStatus() error = %v", err)
	}
}

func TestMembershipServiceMapsLastOwnerConflict(t *testing.T) {
	store := &fakeMembershipStore{statusErr: repository.ErrLastActiveOwner}
	service := NewMembershipService(store)
	_, err := service.ChangeStatus(context.Background(), ChangeMembershipStatusInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111",
		MembershipID:   "22222222-2222-2222-2222-222222222222",
		Status:         model.MembershipStatusRemoved,
		Reason:         "account closure",
		ActorUserID:    "33333333-3333-3333-3333-333333333333",
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "MEMBERSHIP_LAST_OWNER_REQUIRED" {
		t.Fatalf("ChangeStatus() error = %v", err)
	}
}
