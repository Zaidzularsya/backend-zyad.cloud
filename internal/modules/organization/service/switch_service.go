package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/repository"
)

type SwitchStore interface {
	ListActiveByUser(context.Context, string) ([]repository.MembershipOrganization, error)
	FindSessionSnapshot(
		context.Context,
		string,
		string,
	) (repository.SessionOrganizationSnapshot, error)
	Switch(
		context.Context,
		repository.SwitchOrganizationParams,
	) (repository.MembershipOrganization, error)
}

type SwitchMetadata struct {
	RequestID string
	IPAddress string
	UserAgent string
}

type SwitchService struct {
	store SwitchStore
	now   func() time.Time
}

func NewSwitchService(store SwitchStore) *SwitchService {
	return &SwitchService{store: store, now: time.Now}
}

func (s *SwitchService) List(
	ctx context.Context,
	userID string,
	sessionID string,
) ([]dto.UserOrganizationResponse, error) {
	if !validUUID(userID) || !validUUID(sessionID) {
		return nil, validationError("user_id and session_id must be valid UUIDs")
	}
	if s.store == nil {
		return nil, switchStoreRequiredError()
	}
	snapshot, err := s.store.FindSessionSnapshot(ctx, userID, sessionID)
	if err != nil {
		return nil, mapSwitchError("ORGANIZATION_LIST_FAILED", "failed to list organizations", err)
	}
	results, err := s.store.ListActiveByUser(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, mapSwitchError("ORGANIZATION_LIST_FAILED", "failed to list organizations", err)
	}
	responses := make([]dto.UserOrganizationResponse, 0, len(results))
	for _, result := range results {
		responses = append(
			responses,
			userOrganizationResponse(result, snapshot.OrganizationID),
		)
	}
	return responses, nil
}

func (s *SwitchService) Switch(
	ctx context.Context,
	userID string,
	sessionID string,
	request dto.SwitchOrganizationRequest,
	metadata SwitchMetadata,
) (dto.SwitchOrganizationResponse, error) {
	if !validUUID(userID) || !validUUID(sessionID) ||
		!validUUID(request.OrganizationID) {
		return dto.SwitchOrganizationResponse{},
			validationError("user_id, session_id, and organization_id must be valid UUIDs")
	}
	if s.store == nil {
		return dto.SwitchOrganizationResponse{}, switchStoreRequiredError()
	}
	result, err := s.store.Switch(ctx, repository.SwitchOrganizationParams{
		UserID:         strings.TrimSpace(userID),
		SessionID:      strings.TrimSpace(sessionID),
		OrganizationID: strings.TrimSpace(request.OrganizationID),
		RequestID:      strings.TrimSpace(metadata.RequestID),
		IPAddress:      strings.TrimSpace(metadata.IPAddress),
		UserAgent:      strings.TrimSpace(metadata.UserAgent),
		SwitchedAt:     s.now().UTC(),
	})
	if err != nil {
		return dto.SwitchOrganizationResponse{}, mapSwitchError(
			"ORGANIZATION_SWITCH_FAILED",
			"failed to switch organization",
			err,
		)
	}
	return dto.SwitchOrganizationResponse{
		CurrentOrganization: userOrganizationResponse(
			result,
			result.Organization.ID,
		),
	}, nil
}

func userOrganizationResponse(
	result repository.MembershipOrganization,
	currentOrganizationID string,
) dto.UserOrganizationResponse {
	return dto.UserOrganizationResponse{
		Organization: dto.OrganizationResponse{
			ID:            result.Organization.ID,
			Type:          string(result.Organization.Type),
			Slug:          result.Organization.Slug,
			Name:          result.Organization.Name,
			Status:        string(result.Organization.Status),
			Timezone:      result.Organization.Timezone,
			Locale:        result.Organization.Locale,
			Region:        result.Organization.Region,
			DataPlacement: string(result.Organization.DataPlacement),
			Metadata:      result.Organization.Metadata,
			CreatedAt:     result.Organization.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:     result.Organization.UpdatedAt.UTC().Format(time.RFC3339),
			DeletedAt:     formatOrganizationTime(result.Organization.DeletedAt),
		},
		Membership: dto.MembershipResponse{
			ID:             result.Membership.ID,
			OrganizationID: result.Membership.OrganizationID,
			UserID:         result.Membership.UserID,
			Status:         string(result.Membership.Status),
			IsOwner:        result.Membership.IsOwner,
			Version:        result.Membership.Version,
			RoleIDs:        nonNilOrganizationStrings(result.RoleIDs),
			RoleSlugs:      nonNilOrganizationStrings(result.RoleSlugs),
			InvitedBy:      optionalOrganizationString(result.Membership.InvitedBy),
			InvitedEmail:   result.Membership.InvitedEmail,
			InvitedAt:      formatOrganizationTime(result.Membership.InvitedAt),
			AcceptedAt:     formatOrganizationTime(result.Membership.AcceptedAt),
			SuspendedAt:    formatOrganizationTime(result.Membership.SuspendedAt),
			RemovedAt:      formatOrganizationTime(result.Membership.RemovedAt),
			CreatedAt:      result.Membership.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      result.Membership.UpdatedAt.UTC().Format(time.RFC3339),
		},
		IsCurrent: result.Organization.ID == strings.TrimSpace(currentOrganizationID),
	}
}

func mapSwitchError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"ORGANIZATION_ACCESS_DENIED",
			"organization membership or active session was not found",
			http.StatusForbidden,
		)
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}

func switchStoreRequiredError() error {
	return coreerrors.New(
		"ORGANIZATION_SWITCH_STORE_REQUIRED",
		"organization switch store is required",
		http.StatusInternalServerError,
	)
}

func formatOrganizationTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func optionalOrganizationString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nonNilOrganizationStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
