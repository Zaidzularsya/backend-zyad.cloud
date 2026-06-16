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

type EntitlementAPIStore interface {
	ListEffective(
		context.Context,
		string,
		repository.EntitlementListFilter,
		time.Time,
	) ([]model.Entitlement, int64, error)
	Upsert(context.Context, repository.UpsertEntitlementParams) (model.Entitlement, error)
}

type EntitlementAPIService struct {
	store       EntitlementAPIStore
	entitlement *EntitlementService
	now         func() time.Time
}

func NewEntitlementAPIService(
	store EntitlementAPIStore,
	entitlement *EntitlementService,
) *EntitlementAPIService {
	return &EntitlementAPIService{
		store:       store,
		entitlement: entitlement,
		now:         time.Now,
	}
}

func (s *EntitlementAPIService) ListFeatures(
	ctx context.Context,
	organizationID string,
	query dto.EntitlementListQuery,
) ([]dto.EffectiveFeatureResponse, dto.PaginationMeta, error) {
	if !validUUID(organizationID) {
		return nil, dto.PaginationMeta{},
			validationError("organization_id must be a valid UUID")
	}
	filter, page, perPage, err := entitlementListFilter(query)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}
	if s.store == nil {
		return nil, dto.PaginationMeta{}, entitlementStoreRequiredError()
	}
	entitlements, total, err := s.store.ListEffective(
		ctx,
		strings.TrimSpace(organizationID),
		filter,
		s.now().UTC(),
	)
	if err != nil {
		return nil, dto.PaginationMeta{}, mapEntitlementAPIError(
			"ORGANIZATION_FEATURE_LIST_FAILED",
			"failed to list organization features",
			err,
		)
	}
	items := make([]dto.EffectiveFeatureResponse, 0, len(entitlements))
	for _, entitlement := range entitlements {
		items = append(items, effectiveFeatureResponse(entitlement, true))
	}
	return items, dto.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: int(math.Ceil(float64(total) / float64(perPage))),
	}, nil
}

func (s *EntitlementAPIService) CheckUsage(
	ctx context.Context,
	organizationID string,
	query dto.UsageQuery,
) (dto.UsageResponse, error) {
	if s.entitlement == nil {
		return dto.UsageResponse{}, entitlementStoreRequiredError()
	}
	periodStart, err := parseEntitlementTime(query.PeriodStart, "period_start")
	if err != nil {
		return dto.UsageResponse{}, err
	}
	periodEnd, err := parseEntitlementTime(query.PeriodEnd, "period_end")
	if err != nil {
		return dto.UsageResponse{}, err
	}
	usage, err := s.entitlement.CheckUsage(ctx, UsageInput{
		OrganizationID: strings.TrimSpace(organizationID),
		FeatureKey:     query.FeatureKey,
		MetricKey:      query.MetricKey,
		LimitKey:       query.LimitKey,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
	})
	if err != nil {
		return dto.UsageResponse{}, err
	}
	return usageResponse(usage), nil
}

func (s *EntitlementAPIService) UpsertPlatformOverride(
	ctx context.Context,
	organizationID string,
	request dto.UpsertEntitlementRequest,
	actorUserID string,
) (dto.EntitlementResponse, error) {
	if !validUUID(organizationID) {
		return dto.EntitlementResponse{},
			validationError("organization_id must be a valid UUID")
	}
	if !validUUID(actorUserID) {
		return dto.EntitlementResponse{},
			validationError("actor_user_id must be a valid UUID")
	}
	params, err := platformOverrideParams(organizationID, request, actorUserID)
	if err != nil {
		return dto.EntitlementResponse{}, err
	}
	if s.store == nil {
		return dto.EntitlementResponse{}, entitlementStoreRequiredError()
	}
	entitlement, err := s.store.Upsert(ctx, params)
	if err != nil {
		return dto.EntitlementResponse{}, mapEntitlementAPIError(
			"ORGANIZATION_ENTITLEMENT_UPSERT_FAILED",
			"failed to upsert organization entitlement override",
			err,
		)
	}
	return entitlementResponse(entitlement), nil
}

func entitlementListFilter(
	query dto.EntitlementListQuery,
) (repository.EntitlementListFilter, int, int, error) {
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
	return repository.EntitlementListFilter{
		FeatureKey: strings.TrimSpace(query.FeatureKey),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	}, page, perPage, nil
}

func platformOverrideParams(
	organizationID string,
	request dto.UpsertEntitlementRequest,
	actorUserID string,
) (repository.UpsertEntitlementParams, error) {
	source := model.EntitlementSource(strings.TrimSpace(request.Source))
	if source != model.EntitlementSourcePlatformOverride {
		return repository.UpsertEntitlementParams{},
			validationError("source must be platform_override")
	}
	status := model.EntitlementStatus(strings.TrimSpace(request.Status))
	if status == "" {
		status = model.EntitlementStatusActive
	}
	if !status.IsValid() {
		return repository.UpsertEntitlementParams{},
			validationError("entitlement status is invalid")
	}
	effectiveFrom := time.Now().UTC()
	if request.EffectiveFrom != nil {
		parsed, err := parseEntitlementTime(*request.EffectiveFrom, "effective_from")
		if err != nil {
			return repository.UpsertEntitlementParams{}, err
		}
		effectiveFrom = parsed
	}
	var effectiveUntil *time.Time
	if request.EffectiveUntil != nil {
		parsed, err := parseEntitlementTime(*request.EffectiveUntil, "effective_until")
		if err != nil {
			return repository.UpsertEntitlementParams{}, err
		}
		effectiveUntil = &parsed
	}
	if effectiveUntil != nil && !effectiveUntil.After(effectiveFrom) {
		return repository.UpsertEntitlementParams{},
			validationError("effective_until must be after effective_from")
	}
	return repository.UpsertEntitlementParams{
		OrganizationID:  strings.TrimSpace(organizationID),
		FeatureKey:      request.FeatureKey,
		Source:          source,
		SourceReference: request.SourceReference,
		Status:          status,
		Limits:          request.Limits,
		EffectiveFrom:   effectiveFrom,
		EffectiveUntil:  effectiveUntil,
		Reason:          request.Reason,
		ActorUserID:     strings.TrimSpace(actorUserID),
	}, nil
}

func effectiveFeatureResponse(
	entitlement model.Entitlement,
	enabled bool,
) dto.EffectiveFeatureResponse {
	return dto.EffectiveFeatureResponse{
		FeatureKey:     entitlement.FeatureKey,
		Enabled:        enabled,
		Limits:         nonNilLimits(entitlement.Limits),
		EffectiveUntil: formatOrganizationTime(entitlement.EffectiveUntil),
		Version:        entitlement.Version,
	}
}

func entitlementResponse(entitlement model.Entitlement) dto.EntitlementResponse {
	return dto.EntitlementResponse{
		ID:              entitlement.ID,
		OrganizationID:  entitlement.OrganizationID,
		FeatureKey:      entitlement.FeatureKey,
		Source:          string(entitlement.Source),
		SourceReference: entitlement.SourceReference,
		Status:          string(entitlement.Status),
		Limits:          nonNilLimits(entitlement.Limits),
		Version:         entitlement.Version,
		EffectiveFrom:   entitlement.EffectiveFrom.UTC().Format(time.RFC3339),
		EffectiveUntil:  formatOrganizationTime(entitlement.EffectiveUntil),
		Reason:          entitlement.Reason,
		CreatedAt:       entitlement.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       entitlement.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func usageResponse(usage UsageEvaluation) dto.UsageResponse {
	return dto.UsageResponse{
		FeatureKey:     usage.Counter.FeatureKey,
		MetricKey:      usage.Counter.MetricKey,
		PeriodStart:    usage.Counter.PeriodStart.UTC().Format(time.RFC3339),
		PeriodEnd:      usage.Counter.PeriodEnd.UTC().Format(time.RFC3339),
		UsageValue:     usage.Counter.UsageValue,
		LimitValue:     usage.Limit,
		RemainingValue: usage.Remaining,
		Version:        usage.Counter.Version,
		LastRecordedAt: formatOrganizationTime(usage.Counter.LastRecordedAt),
	}
}

func nonNilLimits(limits map[string]any) map[string]any {
	if limits == nil {
		return map[string]any{}
	}
	return limits
}

func parseEntitlementTime(value string, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, validationError(field + " must use RFC3339 format")
	}
	return parsed.UTC(), nil
}

func mapEntitlementAPIError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"ORGANIZATION_ENTITLEMENT_NOT_FOUND",
			"organization entitlement was not found",
			http.StatusNotFound,
		)
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}
