package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const defaultInvitationLifetime = 72 * time.Hour

type MembershipStore interface {
	Invite(context.Context, repository.InviteMembershipParams) (model.Membership, error)
	Accept(context.Context, repository.AcceptMembershipParams) (model.Membership, error)
	ChangeStatus(context.Context, repository.ChangeMembershipStatusParams) (model.Membership, error)
	SyncRoles(context.Context, repository.SyncMembershipRolesParams) (model.Membership, error)
	TransferOwnership(context.Context, repository.TransferMembershipOwnershipParams) error
	ListByOrganization(
		context.Context,
		string,
		repository.MembershipListFilter,
	) ([]model.Membership, int64, error)
}

type InviteMemberInput struct {
	OrganizationID string
	Email          string
	RoleIDs        []string
	ExpiresAt      *time.Time
	ActorUserID    string
}

type InviteMemberResult struct {
	Membership model.Membership
	Token      string
}

type AcceptInvitationInput struct {
	UserID string
	Token  string
}

type ChangeMembershipStatusInput struct {
	OrganizationID string
	MembershipID   string
	Status         model.MembershipStatus
	Reason         string
	ActorUserID    string
}

type SyncMembershipRolesInput struct {
	OrganizationID string
	MembershipID   string
	RoleIDs        []string
	ActorUserID    string
}

type TransferOwnershipInput struct {
	OrganizationID   string
	FromMembershipID string
	ToMembershipID   string
	ActorUserID      string
}

type MembershipService struct {
	store         MembershipStore
	now           func() time.Time
	generateToken func(int) (string, error)
}

func NewMembershipService(store MembershipStore) *MembershipService {
	return &MembershipService{
		store:         store,
		now:           time.Now,
		generateToken: coreauth.NewRandomToken,
	}
}

func (s *MembershipService) Invite(
	ctx context.Context,
	input InviteMemberInput,
) (InviteMemberResult, error) {
	now := s.now().UTC()
	email := strings.ToLower(strings.TrimSpace(input.Email))
	roleIDs, err := normalizeRoleIDs(input.RoleIDs)
	if err != nil {
		return InviteMemberResult{}, err
	}
	if !validUUID(input.OrganizationID) || !validUUID(input.ActorUserID) {
		return InviteMemberResult{}, validationError(
			"organization_id and actor_user_id must be valid UUIDs",
		)
	}
	if email == "" || !strings.Contains(email, "@") {
		return InviteMemberResult{}, validationError("email is invalid")
	}
	expiresAt := now.Add(defaultInvitationLifetime)
	if input.ExpiresAt != nil {
		expiresAt = input.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return InviteMemberResult{}, validationError("expires_at must be in the future")
	}
	token, err := s.generateToken(32)
	if err != nil {
		return InviteMemberResult{}, coreerrors.Wrap(
			"MEMBERSHIP_TOKEN_GENERATION_FAILED",
			"failed to generate invitation token",
			http.StatusInternalServerError,
			err,
		)
	}
	membership, err := s.store.Invite(ctx, repository.InviteMembershipParams{
		OrganizationID:      strings.TrimSpace(input.OrganizationID),
		Email:               email,
		RoleIDs:             roleIDs,
		InvitationTokenHash: coreauth.HashToken(token),
		InvitationExpiresAt: expiresAt,
		ActorUserID:         strings.TrimSpace(input.ActorUserID),
		InvitedAt:           now,
	})
	if err != nil {
		return InviteMemberResult{}, mapMembershipError(
			"MEMBERSHIP_INVITE_FAILED", "failed to invite member", err,
		)
	}
	return InviteMemberResult{Membership: membership, Token: token}, nil
}

func (s *MembershipService) Accept(
	ctx context.Context,
	input AcceptInvitationInput,
) (model.Membership, error) {
	if !validUUID(input.UserID) {
		return model.Membership{}, validationError("user_id must be a valid UUID")
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return model.Membership{}, validationError("invitation token is required")
	}
	membership, err := s.store.Accept(ctx, repository.AcceptMembershipParams{
		UserID:              strings.TrimSpace(input.UserID),
		InvitationTokenHash: coreauth.HashToken(token),
		AcceptedAt:          s.now().UTC(),
	})
	if err != nil {
		return model.Membership{}, mapMembershipError(
			"MEMBERSHIP_ACCEPT_FAILED", "failed to accept invitation", err,
		)
	}
	return membership, nil
}

func (s *MembershipService) List(
	ctx context.Context,
	organizationID string,
	filter repository.MembershipListFilter,
) ([]model.Membership, int64, error) {
	if !validUUID(organizationID) {
		return nil, 0, validationError("organization_id must be a valid UUID")
	}
	if filter.Status != "" && !filter.Status.IsValid() {
		return nil, 0, validationError("membership status is invalid")
	}
	memberships, total, err := s.store.ListByOrganization(
		ctx, strings.TrimSpace(organizationID), filter,
	)
	if err != nil {
		return nil, 0, mapMembershipError(
			"MEMBERSHIP_LIST_FAILED", "failed to list members", err,
		)
	}
	return memberships, total, nil
}

func (s *MembershipService) ChangeStatus(
	ctx context.Context,
	input ChangeMembershipStatusInput,
) (model.Membership, error) {
	if !validUUID(input.OrganizationID) || !validUUID(input.MembershipID) ||
		!validUUID(input.ActorUserID) {
		return model.Membership{}, validationError(
			"organization_id, membership_id, and actor_user_id must be valid UUIDs",
		)
	}
	if input.Status != model.MembershipStatusActive &&
		input.Status != model.MembershipStatusSuspended &&
		input.Status != model.MembershipStatusRemoved {
		return model.Membership{}, validationError("membership status is invalid")
	}
	if (input.Status == model.MembershipStatusSuspended ||
		input.Status == model.MembershipStatusRemoved) &&
		strings.TrimSpace(input.Reason) == "" {
		return model.Membership{}, validationError("reason is required")
	}
	membership, err := s.store.ChangeStatus(ctx, repository.ChangeMembershipStatusParams{
		OrganizationID: strings.TrimSpace(input.OrganizationID),
		MembershipID:   strings.TrimSpace(input.MembershipID),
		Status:         input.Status,
		Reason:         strings.TrimSpace(input.Reason),
		ActorUserID:    strings.TrimSpace(input.ActorUserID),
		ChangedAt:      s.now().UTC(),
	})
	if err != nil {
		return model.Membership{}, mapMembershipError(
			"MEMBERSHIP_STATUS_UPDATE_FAILED", "failed to update membership status", err,
		)
	}
	return membership, nil
}

func (s *MembershipService) SyncRoles(
	ctx context.Context,
	input SyncMembershipRolesInput,
) (model.Membership, error) {
	if !validUUID(input.OrganizationID) || !validUUID(input.MembershipID) ||
		!validUUID(input.ActorUserID) {
		return model.Membership{}, validationError(
			"organization_id, membership_id, and actor_user_id must be valid UUIDs",
		)
	}
	roleIDs, err := normalizeRoleIDs(input.RoleIDs)
	if err != nil {
		return model.Membership{}, err
	}
	membership, err := s.store.SyncRoles(ctx, repository.SyncMembershipRolesParams{
		OrganizationID: strings.TrimSpace(input.OrganizationID),
		MembershipID:   strings.TrimSpace(input.MembershipID),
		RoleIDs:        roleIDs,
		ActorUserID:    strings.TrimSpace(input.ActorUserID),
		ChangedAt:      s.now().UTC(),
	})
	if err != nil {
		return model.Membership{}, mapMembershipError(
			"MEMBERSHIP_ROLE_UPDATE_FAILED", "failed to update membership roles", err,
		)
	}
	return membership, nil
}

func (s *MembershipService) TransferOwnership(
	ctx context.Context,
	input TransferOwnershipInput,
) error {
	if !validUUID(input.OrganizationID) || !validUUID(input.FromMembershipID) ||
		!validUUID(input.ToMembershipID) || !validUUID(input.ActorUserID) {
		return validationError(
			"organization_id, membership IDs, and actor_user_id must be valid UUIDs",
		)
	}
	if input.FromMembershipID == input.ToMembershipID {
		return validationError("ownership target must be another membership")
	}
	err := s.store.TransferOwnership(ctx, repository.TransferMembershipOwnershipParams{
		OrganizationID:   strings.TrimSpace(input.OrganizationID),
		FromMembershipID: strings.TrimSpace(input.FromMembershipID),
		ToMembershipID:   strings.TrimSpace(input.ToMembershipID),
		ActorUserID:      strings.TrimSpace(input.ActorUserID),
		ChangedAt:        s.now().UTC(),
	})
	if err != nil {
		return mapMembershipError(
			"MEMBERSHIP_OWNERSHIP_TRANSFER_FAILED",
			"failed to transfer organization ownership",
			err,
		)
	}
	return nil
}

func normalizeRoleIDs(roleIDs []string) ([]string, error) {
	normalized := make([]string, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		roleID = strings.TrimSpace(roleID)
		if !validUUID(roleID) {
			return nil, validationError("role_ids must contain valid UUIDs")
		}
		if _, exists := seen[roleID]; exists {
			continue
		}
		seen[roleID] = struct{}{}
		normalized = append(normalized, roleID)
	}
	return normalized, nil
}

func mapMembershipError(code, message string, err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return coreerrors.New(
			"MEMBERSHIP_OR_RESOURCE_NOT_FOUND",
			"membership or related resource was not found",
			http.StatusNotFound,
		)
	case errors.Is(err, repository.ErrInvitationExpired):
		return coreerrors.New(
			"MEMBERSHIP_INVITATION_EXPIRED",
			"membership invitation has expired",
			http.StatusConflict,
		)
	case errors.Is(err, repository.ErrMembershipAlreadyExists):
		return coreerrors.New(
			"MEMBERSHIP_ALREADY_EXISTS",
			"user is already a member of the organization",
			http.StatusConflict,
		)
	case errors.Is(err, repository.ErrMembershipStatusInvalid):
		return coreerrors.New(
			"MEMBERSHIP_STATUS_TRANSITION_INVALID",
			"membership status transition is invalid",
			http.StatusConflict,
		)
	case errors.Is(err, repository.ErrLastActiveOwner):
		return coreerrors.New(
			"MEMBERSHIP_LAST_OWNER_REQUIRED",
			"organization must retain at least one active owner",
			http.StatusConflict,
		)
	case errors.Is(err, repository.ErrRoleNotFound):
		return coreerrors.New(
			"MEMBERSHIP_ROLE_NOT_FOUND",
			"one or more roles were not found",
			http.StatusUnprocessableEntity,
		)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return coreerrors.New(
			"MEMBERSHIP_REFERENCE_NOT_FOUND",
			"membership reference was not found",
			http.StatusUnprocessableEntity,
		)
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}
