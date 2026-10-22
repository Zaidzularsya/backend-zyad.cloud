package service

import (
	"context"
	"sort"
	"strings"
	"time"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const (
	SummaryStateUnlimited = "unlimited"
	SummaryStateQuota     = "quota"
	SummaryStateHidden    = "hidden"
)

// EntitlementLister lists the effective entitlements of an organization
// (satisfied by repository.EntitlementRepository).
type EntitlementLister interface {
	ListEffective(ctx context.Context, organizationID string, filter repository.EntitlementListFilter, at time.Time) ([]model.Entitlement, int64, error)
}

// FeatureDefLister lists active registry features.
type FeatureDefLister interface {
	ListActive(ctx context.Context) ([]catalogdomain.FeatureDef, error)
}

type EntitlementSummaryService struct {
	entitlements EntitlementLister
	features     FeatureDefLister
	meters       *QuotaMeterRegistry
	orgs         OrganizationTypeResolver
	now          func() time.Time
}

func NewEntitlementSummaryService(
	entitlements EntitlementLister,
	features FeatureDefLister,
	meters *QuotaMeterRegistry,
	orgs OrganizationTypeResolver,
	now func() time.Time,
) *EntitlementSummaryService {
	if now == nil {
		now = time.Now
	}
	return &EntitlementSummaryService{entitlements: entitlements, features: features, meters: meters, orgs: orgs, now: now}
}

const summaryPageSize = 100

func (s *EntitlementSummaryService) Summary(ctx context.Context, organizationID string) ([]dto.EntitlementSummaryItem, error) {
	organizationID = strings.TrimSpace(organizationID)
	isPlatform := false
	if s.orgs != nil {
		org, err := s.orgs.FindByID(ctx, organizationID)
		if err != nil {
			return nil, err
		}
		isPlatform = org.IsPlatform()
	}
	defs, err := s.features.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Module != defs[j].Module {
			return defs[i].Module < defs[j].Module
		}
		return defs[i].Key < defs[j].Key
	})

	byKey := map[string]model.Entitlement{}
	if !isPlatform {
		now := s.now().UTC()
		for offset := 0; ; {
			page, total, err := s.entitlements.ListEffective(ctx, organizationID,
				repository.EntitlementListFilter{Limit: summaryPageSize, Offset: offset}, now)
			if err != nil {
				return nil, err
			}
			for _, e := range page {
				byKey[e.FeatureKey] = e
			}
			offset += len(page)
			if len(page) == 0 || int64(offset) >= total {
				break
			}
		}
	}

	items := make([]dto.EntitlementSummaryItem, 0, len(defs))
	for _, def := range defs {
		item := dto.EntitlementSummaryItem{
			FeatureKey: def.Key, Module: def.Module, Name: def.Name, Unit: def.Unit,
			ResetStrategy: def.ResetStrategy, State: SummaryStateHidden,
		}
		if isPlatform {
			item.State = SummaryStateUnlimited
			items = append(items, item)
			continue
		}
		if ent, ok := byKey[def.Key]; ok {
			if err := s.fill(ctx, organizationID, def, ent, &item); err != nil {
				return nil, err
			}
		} else if IsUncappedWhenAbsent(def.Key) {
			// Same rule as QuotaEnforcer.RequireRoom: no row for a new quota key = uncapped.
			item.State = SummaryStateUnlimited
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *EntitlementSummaryService) fill(ctx context.Context, organizationID string, def catalogdomain.FeatureDef, ent model.Entitlement, item *dto.EntitlementSummaryItem) error {
	if def.ValueType == "boolean" {
		if enabled, _ := ent.Limits["enabled"].(bool); enabled {
			item.State = SummaryStateUnlimited
		}
		return nil
	}
	if def.ValueType != "integer" {
		return nil
	}
	// An explicit enabled=false row means "not in the plan" (see EvaluateFeature).
	if enabled, ok := ent.Limits["enabled"].(bool); ok && !enabled {
		return nil
	}
	limit, err := guardLimit(ent.Limits, "limit")
	if err != nil {
		return err
	}
	if limit == nil {
		item.State = SummaryStateUnlimited
		return nil
	}
	item.State = SummaryStateQuota
	item.Limit = limit
	meter, ok := s.meters.Get(def.Key)
	if !ok {
		return nil
	}
	start, err := PeriodStart(def.ResetStrategy, s.now())
	if err != nil {
		return err
	}
	used, err := meter.Used(ctx, organizationID, start)
	if err != nil {
		return err
	}
	remaining := *limit - used
	if remaining < 0 {
		remaining = 0
	}
	item.Used = &used
	item.Remaining = &remaining
	item.AtLimit = used >= *limit
	return nil
}
