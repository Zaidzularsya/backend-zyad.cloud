package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type PlatformOrganizationStore interface {
	List(
		context.Context,
		repository.OrganizationListFilter,
	) ([]model.Organization, int64, error)
	FindByIDIncludingArchived(context.Context, string) (model.Organization, error)
	ControlPlaneSummary(
		context.Context,
		string,
	) (repository.OrganizationControlPlaneSummary, error)
	Update(
		context.Context,
		string,
		repository.UpdateOrganizationParams,
	) (model.Organization, error)
}

type PlatformService struct {
	store     PlatformOrganizationStore
	lifecycle *LifecycleService
}

func NewPlatformService(
	store PlatformOrganizationStore,
	lifecycle *LifecycleService,
) *PlatformService {
	return &PlatformService{store: store, lifecycle: lifecycle}
}

func (s *PlatformService) List(
	ctx context.Context,
	query dto.OrganizationListQuery,
) ([]dto.OrganizationResponse, dto.PaginationMeta, error) {
	filter, page, perPage, err := platformOrganizationFilter(query)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}
	if s.store == nil {
		return nil, dto.PaginationMeta{}, platformStoreRequiredError()
	}
	organizations, total, err := s.store.List(ctx, filter)
	if err != nil {
		return nil, dto.PaginationMeta{}, mapPlatformOrganizationError(
			"ORGANIZATION_LIST_FAILED",
			"failed to list organizations",
			err,
		)
	}
	items := make([]dto.OrganizationResponse, 0, len(organizations))
	for _, organization := range organizations {
		items = append(items, organizationResponse(organization))
	}
	return items, dto.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: int(math.Ceil(float64(total) / float64(perPage))),
	}, nil
}

func (s *PlatformService) Get(
	ctx context.Context,
	organizationID string,
) (dto.PlatformOrganizationDetailResponse, error) {
	if !validUUID(organizationID) {
		return dto.PlatformOrganizationDetailResponse{},
			validationError("organization_id must be a valid UUID")
	}
	if s.store == nil {
		return dto.PlatformOrganizationDetailResponse{}, platformStoreRequiredError()
	}
	organization, err := s.store.FindByIDIncludingArchived(
		ctx,
		strings.TrimSpace(organizationID),
	)
	if err != nil {
		return dto.PlatformOrganizationDetailResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_QUERY_FAILED",
			"failed to get organization",
			err,
		)
	}
	return s.detail(ctx, organization)
}

func (s *PlatformService) Create(
	ctx context.Context,
	request dto.CreateOrganizationRequest,
	actorUserID string,
) (dto.OrganizationResponse, error) {
	if s.lifecycle == nil {
		return dto.OrganizationResponse{}, coreerrors.New(
			"ORGANIZATION_LIFECYCLE_REQUIRED",
			"organization lifecycle service is required",
			http.StatusInternalServerError,
		)
	}
	bundle, err := s.lifecycle.Create(ctx, CreateOrganizationInput{
		Type:          coretenant.OrganizationType(request.Type),
		Slug:          request.Slug,
		Name:          request.Name,
		Timezone:      request.Timezone,
		Locale:        request.Locale,
		Region:        request.Region,
		DataPlacement: coretenant.DataPlacement(request.DataPlacement),
		Metadata:      request.Metadata,
		OwnerUserID:   request.OwnerUserID,
		ActorUserID:   actorUserID,
	})
	if err != nil {
		return dto.OrganizationResponse{}, err
	}
	return organizationResponse(bundle.Organization), nil
}

func (s *PlatformService) Update(
	ctx context.Context,
	organizationID string,
	request dto.UpdateOrganizationRequest,
) (dto.OrganizationResponse, error) {
	if !validUUID(organizationID) {
		return dto.OrganizationResponse{}, validationError("organization_id must be a valid UUID")
	}
	params, err := platformUpdateParams(request)
	if err != nil {
		return dto.OrganizationResponse{}, err
	}
	if s.store == nil {
		return dto.OrganizationResponse{}, platformStoreRequiredError()
	}
	current, err := s.store.FindByIDIncludingArchived(ctx, organizationID)
	if err != nil {
		return dto.OrganizationResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_QUERY_FAILED",
			"failed to get organization",
			err,
		)
	}
	if current.Status == coretenant.OrganizationStatusArchived {
		return dto.OrganizationResponse{}, coreerrors.New(
			"ORGANIZATION_ARCHIVED",
			"archived organization cannot be updated",
			http.StatusConflict,
		)
	}
	if current.IsPlatform() && params.DataPlacement != nil {
		return dto.OrganizationResponse{}, coreerrors.New(
			"PLATFORM_ORGANIZATION_PROTECTED",
			"platform organization data placement cannot be changed",
			http.StatusConflict,
		)
	}
	organization, err := s.store.Update(ctx, organizationID, params)
	if err != nil {
		return dto.OrganizationResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_UPDATE_FAILED",
			"failed to update organization",
			err,
		)
	}
	return organizationResponse(organization), nil
}

func (s *PlatformService) ChangeStatus(
	ctx context.Context,
	organizationID string,
	request dto.UpdateOrganizationStatusRequest,
	actorUserID string,
) (dto.OrganizationResponse, error) {
	if !validUUID(organizationID) {
		return dto.OrganizationResponse{}, validationError("organization_id must be a valid UUID")
	}
	if s.store == nil {
		return dto.OrganizationResponse{}, platformStoreRequiredError()
	}
	if s.lifecycle == nil {
		return dto.OrganizationResponse{}, coreerrors.New(
			"ORGANIZATION_LIFECYCLE_REQUIRED",
			"organization lifecycle service is required",
			http.StatusInternalServerError,
		)
	}
	current, err := s.store.FindByIDIncludingArchived(ctx, organizationID)
	if err != nil {
		return dto.OrganizationResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_QUERY_FAILED",
			"failed to get organization",
			err,
		)
	}
	organization, err := s.lifecycle.ChangeStatus(ctx, current, ChangeOrganizationStatusInput{
		OrganizationID: organizationID,
		Status:         coretenant.OrganizationStatus(request.Status),
		Reason:         request.Reason,
		ActorUserID:    actorUserID,
	})
	if err != nil {
		return dto.OrganizationResponse{}, err
	}
	return organizationResponse(organization), nil
}

func (s *PlatformService) Provision(
	ctx context.Context,
	organizationID string,
	actorUserID string,
) (dto.PlatformOrganizationDetailResponse, error) {
	if !validUUID(organizationID) {
		return dto.PlatformOrganizationDetailResponse{},
			validationError("organization_id must be a valid UUID")
	}
	if !validUUID(actorUserID) {
		return dto.PlatformOrganizationDetailResponse{},
			validationError("actor_user_id must be a valid UUID")
	}
	if s.store == nil {
		return dto.PlatformOrganizationDetailResponse{}, platformStoreRequiredError()
	}
	if s.lifecycle == nil {
		return dto.PlatformOrganizationDetailResponse{}, coreerrors.New(
			"ORGANIZATION_LIFECYCLE_REQUIRED",
			"organization lifecycle service is required",
			http.StatusInternalServerError,
		)
	}
	current, err := s.store.FindByIDIncludingArchived(ctx, organizationID)
	if err != nil {
		return dto.PlatformOrganizationDetailResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_QUERY_FAILED",
			"failed to get organization",
			err,
		)
	}
	if current.DataPlacement == coretenant.DataPlacementDedicated {
		return dto.PlatformOrganizationDetailResponse{}, coreerrors.New(
			"DEDICATED_PROVISIONING_NOT_AVAILABLE",
			"dedicated organization provisioning is not available",
			http.StatusServiceUnavailable,
		)
	}
	if current.Status != coretenant.OrganizationStatusProvisioning &&
		current.Status != coretenant.OrganizationStatusProvisioningFailed {
		return dto.PlatformOrganizationDetailResponse{}, coreerrors.New(
			"ORGANIZATION_PROVISIONING_STATE_INVALID",
			"organization is not waiting for provisioning",
			http.StatusConflict,
		)
	}
	summary, err := s.store.ControlPlaneSummary(ctx, current.ID)
	if err != nil {
		return dto.PlatformOrganizationDetailResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_HEALTH_QUERY_FAILED",
			"failed to inspect organization provisioning readiness",
			err,
		)
	}
	if summary.ActiveOwnerCount == 0 {
		return dto.PlatformOrganizationDetailResponse{}, coreerrors.New(
			"ORGANIZATION_PROVISIONING_NOT_READY",
			"organization requires an active owner before provisioning",
			http.StatusConflict,
		)
	}

	if current.Status == coretenant.OrganizationStatusProvisioningFailed {
		current, err = s.lifecycle.ChangeStatus(ctx, current, ChangeOrganizationStatusInput{
			OrganizationID: current.ID,
			Status:         coretenant.OrganizationStatusProvisioning,
			Reason:         "shared organization provisioning retry started",
			ActorUserID:    actorUserID,
		})
		if err != nil {
			return dto.PlatformOrganizationDetailResponse{}, err
		}
	}
	organization, err := s.lifecycle.ChangeStatus(ctx, current, ChangeOrganizationStatusInput{
		OrganizationID: current.ID,
		Status:         coretenant.OrganizationStatusActive,
		Reason:         "shared organization provisioning completed",
		ActorUserID:    actorUserID,
	})
	if err != nil {
		return dto.PlatformOrganizationDetailResponse{}, err
	}
	return s.detail(ctx, organization)
}

func (s *PlatformService) detail(
	ctx context.Context,
	organization model.Organization,
) (dto.PlatformOrganizationDetailResponse, error) {
	summary, err := s.store.ControlPlaneSummary(ctx, organization.ID)
	if err != nil {
		return dto.PlatformOrganizationDetailResponse{}, mapPlatformOrganizationError(
			"ORGANIZATION_SUMMARY_FAILED",
			"failed to get organization summary",
			err,
		)
	}
	placement := dto.OrganizationPlacementSummary{
		Type:      string(organization.DataPlacement),
		Supported: organization.DataPlacement == coretenant.DataPlacementShared,
		Status:    "ready",
	}
	if !placement.Supported {
		placement.Status = "unavailable"
	}
	issues := organizationHealthIssues(organization, summary, placement.Supported)
	healthStatus := "healthy"
	if len(issues) > 0 {
		healthStatus = "degraded"
	}
	if !placement.Supported ||
		summary.ActiveOwnerCount == 0 ||
		organization.Status == coretenant.OrganizationStatusDisabled ||
		organization.Status == coretenant.OrganizationStatusArchived {
		healthStatus = "unavailable"
	}
	return dto.PlatformOrganizationDetailResponse{
		Organization: organizationResponse(organization),
		Placement:    placement,
		Domains: dto.OrganizationDomainSummary{
			Total:         summary.DomainCount,
			Active:        summary.ActiveDomainCount,
			Pending:       summary.PendingDomainCount,
			Failed:        summary.FailedDomainCount,
			SSLFailed:     summary.SSLFailedDomainCount,
			PrimaryActive: summary.PrimaryActiveCount,
		},
		Entitlements: dto.OrganizationEntitlementSummary{
			Total:      summary.EntitlementCount,
			Active:     summary.ActiveEntitlementCount,
			MaxVersion: summary.MaxEntitlementVersion,
		},
		Health: dto.OrganizationHealthSummary{
			Status: healthStatus,
			Issues: issues,
		},
	}, nil
}

func organizationHealthIssues(
	organization model.Organization,
	summary repository.OrganizationControlPlaneSummary,
	placementSupported bool,
) []string {
	issues := make([]string, 0, 6)
	if !placementSupported {
		issues = append(issues, "data placement is not available")
	}
	if summary.ActiveOwnerCount == 0 {
		issues = append(issues, "active organization owner is missing")
	}
	if organization.Status != coretenant.OrganizationStatusActive {
		issues = append(issues, "organization is not active")
	}
	if summary.FailedDomainCount > 0 {
		issues = append(issues, "one or more domains failed verification")
	}
	if summary.SSLFailedDomainCount > 0 {
		issues = append(issues, "one or more domains failed SSL provisioning")
	}
	return issues
}

func platformOrganizationFilter(
	query dto.OrganizationListQuery,
) (repository.OrganizationListFilter, int, int, error) {
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
	filter := repository.OrganizationListFilter{
		Type:            coretenant.OrganizationType(strings.TrimSpace(query.Type)),
		Status:          coretenant.OrganizationStatus(strings.TrimSpace(query.Status)),
		DataPlacement:   coretenant.DataPlacement(strings.TrimSpace(query.DataPlacement)),
		Search:          strings.TrimSpace(query.Search),
		IncludeArchived: query.IncludeDeleted,
		Limit:           perPage,
		Offset:          (page - 1) * perPage,
	}
	if filter.Type != "" && !filter.Type.IsValid() {
		return repository.OrganizationListFilter{}, 0, 0, validationError("type is invalid")
	}
	if filter.Status != "" && !filter.Status.IsValid() {
		return repository.OrganizationListFilter{}, 0, 0, validationError("status is invalid")
	}
	if filter.DataPlacement != "" && !filter.DataPlacement.IsValid() {
		return repository.OrganizationListFilter{}, 0, 0, validationError("data_placement is invalid")
	}
	return filter, page, perPage, nil
}

func platformUpdateParams(
	request dto.UpdateOrganizationRequest,
) (repository.UpdateOrganizationParams, error) {
	params := repository.UpdateOrganizationParams{
		Slug:     request.Slug,
		Name:     request.Name,
		Timezone: request.Timezone,
		Locale:   request.Locale,
		Region:   request.Region,
		Metadata: request.Metadata,
	}
	if request.DataPlacement != nil {
		placement := coretenant.DataPlacement(strings.TrimSpace(*request.DataPlacement))
		if !placement.IsValid() {
			return repository.UpdateOrganizationParams{}, validationError("data_placement is invalid")
		}
		params.DataPlacement = &placement
	}
	for name, value := range map[string]*string{
		"slug": request.Slug, "name": request.Name,
		"timezone": request.Timezone, "locale": request.Locale,
	} {
		if value != nil && strings.TrimSpace(*value) == "" {
			return repository.UpdateOrganizationParams{}, validationError(name + " cannot be blank")
		}
	}
	return params, nil
}

func mapPlatformOrganizationError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New("ORGANIZATION_NOT_FOUND", "organization not found", http.StatusNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return coreerrors.New(
			"ORGANIZATION_SLUG_ALREADY_EXISTS",
			"organization slug already exists",
			http.StatusConflict,
		)
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}

func platformStoreRequiredError() error {
	return coreerrors.New(
		"ORGANIZATION_STORE_REQUIRED",
		"organization store is required",
		http.StatusInternalServerError,
	)
}
