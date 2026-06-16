package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type SelfOrganizationStore interface {
	FindByID(context.Context, string) (model.Organization, error)
	Update(
		context.Context,
		string,
		repository.UpdateCurrentOrganizationParams,
	) (model.Organization, error)
}

type MembershipDetailStore interface {
	ListDetailsByOrganization(
		context.Context,
		string,
		repository.MembershipListFilter,
	) ([]repository.MembershipDetail, int64, error)
}

type SelfService struct {
	store       SelfOrganizationStore
	memberships *MembershipService
	details     MembershipDetailStore
	now         func() time.Time
}

func NewSelfService(
	store SelfOrganizationStore,
	memberships *MembershipService,
	details MembershipDetailStore,
) *SelfService {
	return &SelfService{
		store:       store,
		memberships: memberships,
		details:     details,
		now:         time.Now,
	}
}

func (s *SelfService) Get(
	ctx context.Context,
	organizationID string,
) (dto.OrganizationResponse, error) {
	if !validUUID(organizationID) {
		return dto.OrganizationResponse{}, validationError(
			"organization_id must be a valid UUID",
		)
	}
	organization, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		return dto.OrganizationResponse{}, mapSelfError(
			"ORGANIZATION_QUERY_FAILED",
			"failed to get current organization",
			err,
		)
	}
	return organizationResponse(organization), nil
}

func (s *SelfService) Update(
	ctx context.Context,
	organizationID string,
	membershipID string,
	actorUserID string,
	request dto.UpdateCurrentOrganizationRequest,
) (dto.OrganizationResponse, error) {
	if !validUUID(organizationID) || !validUUID(membershipID) ||
		!validUUID(actorUserID) {
		return dto.OrganizationResponse{}, validationError(
			"organization_id, membership_id, and actor_user_id must be valid UUIDs",
		)
	}
	if request.Name == nil && request.Timezone == nil && request.Locale == nil &&
		request.Region == nil && request.Metadata == nil {
		return dto.OrganizationResponse{}, validationError(
			"at least one organization field is required",
		)
	}
	for name, value := range map[string]*string{
		"name": request.Name, "timezone": request.Timezone, "locale": request.Locale,
	} {
		if value != nil && strings.TrimSpace(*value) == "" {
			return dto.OrganizationResponse{}, validationError(name + " cannot be blank")
		}
	}
	if request.Timezone != nil {
		if _, err := time.LoadLocation(strings.TrimSpace(*request.Timezone)); err != nil {
			return dto.OrganizationResponse{}, validationError("timezone is invalid")
		}
	}
	organization, err := s.store.Update(
		ctx,
		strings.TrimSpace(organizationID),
		repository.UpdateCurrentOrganizationParams{
			Name:         request.Name,
			Timezone:     request.Timezone,
			Locale:       request.Locale,
			Region:       request.Region,
			Metadata:     request.Metadata,
			ActorUserID:  strings.TrimSpace(actorUserID),
			MembershipID: strings.TrimSpace(membershipID),
			ChangedAt:    s.now().UTC(),
		},
	)
	if err != nil {
		return dto.OrganizationResponse{}, mapSelfError(
			"ORGANIZATION_UPDATE_FAILED",
			"failed to update current organization",
			err,
		)
	}
	return organizationResponse(organization), nil
}

func (s *SelfService) ListMembers(
	ctx context.Context,
	organizationID string,
	query dto.MembershipListQuery,
) ([]dto.MembershipResponse, dto.PaginationMeta, error) {
	if !validUUID(organizationID) {
		return nil, dto.PaginationMeta{}, validationError(
			"organization_id must be a valid UUID",
		)
	}
	filter, page, perPage, err := membershipListFilter(query)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}
	details, total, err := s.details.ListDetailsByOrganization(
		ctx,
		strings.TrimSpace(organizationID),
		filter,
	)
	if err != nil {
		return nil, dto.PaginationMeta{}, mapSelfError(
			"MEMBERSHIP_LIST_FAILED",
			"failed to list organization members",
			err,
		)
	}
	items := make([]dto.MembershipResponse, 0, len(details))
	for _, detail := range details {
		items = append(items, membershipDetailResponse(detail))
	}
	return items, dto.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: int(math.Ceil(float64(total) / float64(perPage))),
	}, nil
}

func (s *SelfService) Invite(
	ctx context.Context,
	organizationID string,
	actorUserID string,
	request dto.InviteMemberRequest,
) (dto.InvitationResponse, error) {
	var expiresAt *time.Time
	if request.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*request.ExpiresAt))
		if err != nil {
			return dto.InvitationResponse{}, validationError(
				"expires_at must use RFC3339 format",
			)
		}
		expiresAt = &parsed
	}
	result, err := s.memberships.Invite(ctx, InviteMemberInput{
		OrganizationID: organizationID,
		Email:          request.Email,
		RoleIDs:        request.RoleIDs,
		ExpiresAt:      expiresAt,
		ActorUserID:    actorUserID,
	})
	if err != nil {
		return dto.InvitationResponse{}, err
	}
	response := dto.InvitationResponse{
		Membership:      membershipResponse(result.Membership),
		InvitationToken: result.Token,
	}
	if result.Membership.InvitationExpiresAt != nil {
		response.ExpiresAt = result.Membership.InvitationExpiresAt.UTC().Format(time.RFC3339)
	}
	return response, nil
}

func (s *SelfService) ChangeMemberStatus(
	ctx context.Context,
	organizationID string,
	membershipID string,
	actorUserID string,
	request dto.UpdateMembershipStatusRequest,
) (dto.MembershipResponse, error) {
	membership, err := s.memberships.ChangeStatus(ctx, ChangeMembershipStatusInput{
		OrganizationID: organizationID,
		MembershipID:   membershipID,
		Status:         model.MembershipStatus(strings.TrimSpace(request.Status)),
		Reason:         request.Reason,
		ActorUserID:    actorUserID,
	})
	if err != nil {
		return dto.MembershipResponse{}, err
	}
	return membershipResponse(membership), nil
}

func (s *SelfService) RemoveMember(
	ctx context.Context,
	organizationID string,
	membershipID string,
	actorUserID string,
	reason string,
) (dto.MembershipResponse, error) {
	return s.ChangeMemberStatus(
		ctx,
		organizationID,
		membershipID,
		actorUserID,
		dto.UpdateMembershipStatusRequest{
			Status: string(model.MembershipStatusRemoved),
			Reason: reason,
		},
	)
}

func membershipListFilter(
	query dto.MembershipListQuery,
) (repository.MembershipListFilter, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	status := model.MembershipStatus(strings.TrimSpace(query.Status))
	if status != "" && !status.IsValid() {
		return repository.MembershipListFilter{}, 0, 0,
			validationError("membership status is invalid")
	}
	return repository.MembershipListFilter{
		Status:         status,
		IncludeRemoved: query.IncludeRemoved,
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}, page, perPage, nil
}

func membershipDetailResponse(
	detail repository.MembershipDetail,
) dto.MembershipResponse {
	response := membershipResponse(detail.Membership)
	response.UserName = detail.UserName
	response.UserEmail = detail.UserEmail
	response.RoleIDs = nonNilOrganizationStrings(detail.RoleIDs)
	response.RoleSlugs = nonNilOrganizationStrings(detail.RoleSlugs)
	return response
}

func membershipResponse(membership model.Membership) dto.MembershipResponse {
	return dto.MembershipResponse{
		ID:             membership.ID,
		OrganizationID: membership.OrganizationID,
		UserID:         membership.UserID,
		Status:         string(membership.Status),
		IsOwner:        membership.IsOwner,
		Version:        membership.Version,
		RoleIDs:        []string{},
		RoleSlugs:      []string{},
		InvitedBy:      optionalOrganizationString(membership.InvitedBy),
		InvitedEmail:   membership.InvitedEmail,
		InvitedAt:      formatOrganizationTime(membership.InvitedAt),
		AcceptedAt:     formatOrganizationTime(membership.AcceptedAt),
		SuspendedAt:    formatOrganizationTime(membership.SuspendedAt),
		RemovedAt:      formatOrganizationTime(membership.RemovedAt),
		CreatedAt:      membership.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      membership.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func mapSelfError(code, message string, err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return coreerrors.New(
			"ORGANIZATION_OR_RESOURCE_NOT_FOUND",
			"organization or related resource was not found",
			http.StatusNotFound,
		)
	case errors.Is(err, repository.ErrNoOrganizationChanges):
		return validationError("at least one organization field is required")
	default:
		return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
	}
}
