package service

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/repository"
)

var workspaceSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var workspaceSlugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type OnboardingStore interface {
	CreateWorkspace(context.Context, repository.CreateWorkspaceParams) (repository.MembershipOrganization, error)
}

type OnboardingService struct {
	store OnboardingStore
	now   func() time.Time
}

type OnboardingMetadata struct {
	RequestID string
	IPAddress string
	UserAgent string
}

func NewOnboardingService(store OnboardingStore) *OnboardingService {
	return &OnboardingService{store: store, now: time.Now}
}

func (s *OnboardingService) CreateWorkspace(
	ctx context.Context,
	userID string,
	sessionID string,
	request dto.CreateWorkspaceRequest,
	metadata OnboardingMetadata,
) (dto.CreateWorkspaceResponse, error) {
	if !validUUID(userID) || !validUUID(sessionID) {
		return dto.CreateWorkspaceResponse{}, validationError("user_id and session_id must be valid UUIDs")
	}
	if s.store == nil {
		return dto.CreateWorkspaceResponse{}, coreerrors.New(
			"WORKSPACE_ONBOARDING_STORE_REQUIRED",
			"workspace onboarding store is required",
			http.StatusInternalServerError,
		)
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		return dto.CreateWorkspaceResponse{}, validationError("workspace name is required")
	}
	slug := normalizeWorkspaceSlug(request.Slug)
	if slug == "" {
		slug = normalizeWorkspaceSlug(name)
	}
	if !workspaceSlugPattern.MatchString(slug) {
		return dto.CreateWorkspaceResponse{}, validationError("workspace slug must be a valid DNS label")
	}

	result, err := s.store.CreateWorkspace(ctx, repository.CreateWorkspaceParams{
		UserID:        strings.TrimSpace(userID),
		SessionID:     strings.TrimSpace(sessionID),
		Slug:          slug,
		Name:          name,
		Timezone:      strings.TrimSpace(request.Timezone),
		Locale:        strings.TrimSpace(request.Locale),
		Region:        strings.TrimSpace(request.Region),
		Metadata:      request.Metadata,
		RequestID:     strings.TrimSpace(metadata.RequestID),
		IPAddress:     strings.TrimSpace(metadata.IPAddress),
		UserAgent:     strings.TrimSpace(metadata.UserAgent),
		CorrelationAt: s.now().UTC(),
	})
	if err != nil {
		return dto.CreateWorkspaceResponse{}, mapOnboardingError(err)
	}
	return dto.CreateWorkspaceResponse{
		CurrentOrganization: userOrganizationResponse(result, result.Organization.ID),
	}, nil
}

func normalizeWorkspaceSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = workspaceSlugCleaner.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func mapOnboardingError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"UNAUTHORIZED",
			"authenticated session was not found",
			http.StatusUnauthorized,
		)
	}
	if errors.Is(err, repository.ErrOrganizationOwnerRoleNotFound) {
		return coreerrors.New(
			"WORKSPACE_OWNER_ROLE_NOT_FOUND",
			"organization owner role is not configured",
			http.StatusInternalServerError,
		)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return coreerrors.New(
				"WORKSPACE_SLUG_ALREADY_EXISTS",
				"workspace slug already exists",
				http.StatusConflict,
			)
		case "23503":
			return coreerrors.New(
				"WORKSPACE_OWNER_NOT_FOUND",
				"workspace owner was not found",
				http.StatusUnprocessableEntity,
			)
		}
	}
	return coreerrors.Wrap(
		"WORKSPACE_ONBOARDING_FAILED",
		"failed to create workspace",
		http.StatusInternalServerError,
		err,
	)
}
