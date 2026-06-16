package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	coreerrors "zyad.cloud/internal/core/errors"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const organizationSecurityStatusChangedEvent = "organization.security_status_changed"

type LifecycleStore interface {
	CreateBundle(
		ctx context.Context,
		params repository.CreateOrganizationBundleParams,
	) (repository.OrganizationBundle, error)
	ChangeStatus(
		ctx context.Context,
		organizationID string,
		expectedStatus coretenant.OrganizationStatus,
		status coretenant.OrganizationStatus,
		actorUserID string,
		reason string,
		changedAt time.Time,
	) (model.Organization, error)
}

type LifecycleNotificationPublisher interface {
	Publish(
		ctx context.Context,
		event notificationpublisher.Event,
	) (notificationdomain.OutboxEvent, error)
}

type CreateOrganizationInput struct {
	Type          coretenant.OrganizationType
	Slug          string
	Name          string
	Timezone      string
	Locale        string
	Region        string
	DataPlacement coretenant.DataPlacement
	Metadata      map[string]any
	OwnerUserID   string
	ActorUserID   string
	Plan          []repository.UpsertEntitlementParams
}

type ChangeOrganizationStatusInput struct {
	OrganizationID string
	Status         coretenant.OrganizationStatus
	Reason         string
	ActorUserID    string
}

type LifecycleService struct {
	store                 LifecycleStore
	notificationPublisher LifecycleNotificationPublisher
	now                   func() time.Time
}

func NewLifecycleService(store LifecycleStore) *LifecycleService {
	return &LifecycleService{
		store: store,
		now:   time.Now,
	}
}

func (s *LifecycleService) SetNotificationPublisher(publisher LifecycleNotificationPublisher) {
	s.notificationPublisher = publisher
}

func (s *LifecycleService) Create(
	ctx context.Context,
	input CreateOrganizationInput,
) (repository.OrganizationBundle, error) {
	normalized, err := normalizeCreateOrganization(input)
	if err != nil {
		return repository.OrganizationBundle{}, err
	}
	if s.store == nil {
		return repository.OrganizationBundle{}, coreerrors.New(
			"ORGANIZATION_STORE_REQUIRED",
			"organization lifecycle store is required",
			http.StatusInternalServerError,
		)
	}

	params := repository.CreateOrganizationBundleParams{
		Organization: repository.CreateOrganizationParams{
			Type:          normalized.Type,
			Slug:          normalized.Slug,
			Name:          normalized.Name,
			Status:        coretenant.OrganizationStatusProvisioning,
			Timezone:      normalized.Timezone,
			Locale:        normalized.Locale,
			Region:        normalized.Region,
			DataPlacement: normalized.DataPlacement,
			Metadata:      normalized.Metadata,
		},
		OwnerUserID: normalized.OwnerUserID,
		ActorUserID: normalized.ActorUserID,
		Plan:        normalized.Plan,
		CreatedAt:   s.now().UTC(),
	}
	bundle, err := s.store.CreateBundle(ctx, params)
	if err != nil {
		return repository.OrganizationBundle{}, mapLifecycleError(
			"ORGANIZATION_CREATE_FAILED",
			"failed to create organization",
			err,
		)
	}
	return bundle, nil
}

func (s *LifecycleService) ChangeStatus(
	ctx context.Context,
	current model.Organization,
	input ChangeOrganizationStatusInput,
) (model.Organization, error) {
	target := input.Status
	if !target.IsValid() {
		return model.Organization{}, validationError("organization status is invalid")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return model.Organization{}, validationError("reason is required")
	}
	if current.ID == "" || current.ID != strings.TrimSpace(input.OrganizationID) {
		return model.Organization{}, coreerrors.New(
			"ORGANIZATION_NOT_FOUND",
			"organization not found",
			http.StatusNotFound,
		)
	}
	if current.IsPlatform() &&
		(target == coretenant.OrganizationStatusDisabled ||
			target == coretenant.OrganizationStatusArchived) {
		return model.Organization{}, coreerrors.New(
			"PLATFORM_ORGANIZATION_PROTECTED",
			"platform organization cannot be disabled or archived",
			http.StatusConflict,
		)
	}
	if !canTransitionOrganizationStatus(current.Status, target) {
		return model.Organization{}, coreerrors.New(
			"ORGANIZATION_STATUS_TRANSITION_INVALID",
			"organization status transition is invalid",
			http.StatusConflict,
		)
	}

	organization, err := s.store.ChangeStatus(
		ctx,
		current.ID,
		current.Status,
		target,
		strings.TrimSpace(input.ActorUserID),
		strings.TrimSpace(input.Reason),
		s.now().UTC(),
	)
	if err != nil {
		return model.Organization{}, mapLifecycleError(
			"ORGANIZATION_STATUS_UPDATE_FAILED",
			"failed to update organization status",
			err,
		)
	}
	s.publishSecurityStatusChange(ctx, current, organization, input)
	return organization, nil
}

func (s *LifecycleService) publishSecurityStatusChange(
	ctx context.Context,
	previous model.Organization,
	organization model.Organization,
	input ChangeOrganizationStatusInput,
) {
	if s.notificationPublisher == nil || !isSecurityIncidentStatus(input.Status) {
		return
	}
	_, _ = s.notificationPublisher.Publish(ctx, notificationpublisher.Event{
		Type:   organizationSecurityStatusChangedEvent,
		UserID: strings.TrimSpace(input.ActorUserID),
		Payload: map[string]any{
			"organization_id":   organization.ID,
			"organization_slug": organization.Slug,
			"organization_name": organization.Name,
			"from_status":       string(previous.Status),
			"to_status":         string(input.Status),
			"reason":            strings.TrimSpace(input.Reason),
			"actor_user_id":     strings.TrimSpace(input.ActorUserID),
			"changed_at":        organization.UpdatedAt.UTC().Format(time.RFC3339),
		},
	})
}

func isSecurityIncidentStatus(status coretenant.OrganizationStatus) bool {
	return status == coretenant.OrganizationStatusSuspended ||
		status == coretenant.OrganizationStatusDisabled ||
		status == coretenant.OrganizationStatusArchived
}

func normalizeCreateOrganization(input CreateOrganizationInput) (CreateOrganizationInput, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.Locale = strings.TrimSpace(input.Locale)
	input.Region = strings.TrimSpace(input.Region)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.ActorUserID = strings.TrimSpace(input.ActorUserID)

	if !input.Type.IsValid() {
		return CreateOrganizationInput{}, validationError("organization type is invalid")
	}
	if input.Type == coretenant.OrganizationTypePlatform {
		return CreateOrganizationInput{}, validationError("platform organization must be created by the platform seed")
	}
	if input.Slug == "" || input.Name == "" {
		return CreateOrganizationInput{}, validationError("organization slug and name are required")
	}
	if !validUUID(input.OwnerUserID) {
		return CreateOrganizationInput{}, validationError("owner_user_id must be a valid UUID")
	}
	if input.ActorUserID != "" && !validUUID(input.ActorUserID) {
		return CreateOrganizationInput{}, validationError("actor_user_id must be a valid UUID")
	}
	if input.DataPlacement == "" {
		input.DataPlacement = coretenant.DataPlacementShared
	}
	if !input.DataPlacement.IsValid() {
		return CreateOrganizationInput{}, validationError("data_placement is invalid")
	}
	if input.Timezone == "" {
		input.Timezone = "Asia/Jakarta"
	}
	if input.Locale == "" {
		input.Locale = "id-ID"
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	for index := range input.Plan {
		input.Plan[index].FeatureKey = strings.ToLower(strings.TrimSpace(input.Plan[index].FeatureKey))
		input.Plan[index].Source = model.EntitlementSourcePlan
		if input.Plan[index].FeatureKey == "" {
			return CreateOrganizationInput{}, validationError("plan feature_key is required")
		}
		if input.Plan[index].Status == "" {
			input.Plan[index].Status = model.EntitlementStatusActive
		}
		if !input.Plan[index].Status.IsValid() {
			return CreateOrganizationInput{}, validationError("plan status is invalid")
		}
	}
	return input, nil
}

func canTransitionOrganizationStatus(from, to coretenant.OrganizationStatus) bool {
	if from == to {
		return false
	}
	allowed := map[coretenant.OrganizationStatus]map[coretenant.OrganizationStatus]bool{
		coretenant.OrganizationStatusPending: {
			coretenant.OrganizationStatusProvisioning: true,
			coretenant.OrganizationStatusActive:       true,
			coretenant.OrganizationStatusDisabled:     true,
			coretenant.OrganizationStatusArchived:     true,
		},
		coretenant.OrganizationStatusProvisioning: {
			coretenant.OrganizationStatusActive:             true,
			coretenant.OrganizationStatusProvisioningFailed: true,
			coretenant.OrganizationStatusDisabled:           true,
		},
		coretenant.OrganizationStatusProvisioningFailed: {
			coretenant.OrganizationStatusProvisioning: true,
			coretenant.OrganizationStatusDisabled:     true,
			coretenant.OrganizationStatusArchived:     true,
		},
		coretenant.OrganizationStatusActive: {
			coretenant.OrganizationStatusSuspended: true,
			coretenant.OrganizationStatusDisabled:  true,
			coretenant.OrganizationStatusArchived:  true,
		},
		coretenant.OrganizationStatusSuspended: {
			coretenant.OrganizationStatusActive:   true,
			coretenant.OrganizationStatusDisabled: true,
			coretenant.OrganizationStatusArchived: true,
		},
		coretenant.OrganizationStatusDisabled: {
			coretenant.OrganizationStatusActive:   true,
			coretenant.OrganizationStatusArchived: true,
		},
	}
	return allowed[from][to]
}

func mapLifecycleError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New("ORGANIZATION_NOT_FOUND", "organization not found", http.StatusNotFound)
	}
	if errors.Is(err, repository.ErrOrganizationStatusStale) {
		return coreerrors.New(
			"ORGANIZATION_STATUS_STALE",
			"organization status changed; reload and retry",
			http.StatusConflict,
		)
	}
	if errors.Is(err, repository.ErrPlatformOrganizationProtected) {
		return coreerrors.New(
			"PLATFORM_ORGANIZATION_PROTECTED",
			"platform organization cannot be disabled or archived",
			http.StatusConflict,
		)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return coreerrors.New(
				"ORGANIZATION_SLUG_ALREADY_EXISTS",
				"organization slug already exists",
				http.StatusConflict,
			)
		case "23503":
			return coreerrors.New(
				"ORGANIZATION_OWNER_NOT_FOUND",
				"organization owner was not found",
				http.StatusUnprocessableEntity,
			)
		}
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(strings.TrimSpace(value)) == nil && id.Valid
}
