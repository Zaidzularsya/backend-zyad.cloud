package service

import (
	"context"
	"log/slog"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// TriggerPreview is what enabling a product trigger would affect. It is
// computed without writing anything.
type TriggerPreview struct {
	Product        ProductInfo
	Usage          ProductUsage
	EffectiveFrom  time.Time
	AlreadyEnabled bool
}

// PreviewTrigger reports the impact of marking the product as a PPPoE service.
// The product is resolved through ProductCatalog first, so a product of
// another organization yields ErrProductNotFound.
func (s *PolicyService) PreviewTrigger(ctx context.Context, scope coretenant.Scope, productID string) (TriggerPreview, error) {
	product, err := s.products.Get(ctx, scope, productID)
	if err != nil {
		return TriggerPreview{}, err
	}
	effective := businesstime.DayOf(s.now())
	existing, found, err := s.repo.GetProductTrigger(ctx, scope, productID)
	if err != nil {
		return TriggerPreview{}, err
	}
	already := found && existing.Enabled
	if already {
		effective = existing.EffectiveFrom
	}
	usage, err := s.usage.ForProduct(ctx, scope, productID, effective)
	if err != nil {
		return TriggerPreview{}, err
	}
	return TriggerPreview{Product: product, Usage: usage, EffectiveFrom: effective, AlreadyEnabled: already}, nil
}

// SetTrigger marks (or unmarks) a product as a PPPoE service. Enabling an
// already-enabled product keeps its effective_from; re-enabling after a
// disable resets it to today (WIB). The repository does this in one atomic
// upsert. There is no direct effect on any router.
func (s *PolicyService) SetTrigger(ctx context.Context, scope coretenant.Scope, productID string, enabled bool, actor string) (domain.ProductTrigger, error) {
	if _, err := s.products.Get(ctx, scope, productID); err != nil {
		return domain.ProductTrigger{}, err
	}
	t, err := s.repo.UpsertProductTrigger(ctx, scope, productID, enabled, businesstime.DayOf(s.now()), actor)
	if err != nil {
		return domain.ProductTrigger{}, err
	}
	// The write is committed; a failed audit row must not fail it.
	if err := s.audit.InsertAudit(ctx, scope, repository.AuditEntry{
		Event: "product_trigger.set", ActorUserID: actor, TargetID: productID,
		Metadata: map[string]any{
			"target": "product_trigger", "product_id": productID, "enabled": t.Enabled,
			"effective_from": t.EffectiveFrom.Format("2006-01-02"),
		},
	}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "product_id", productID, "event", "product_trigger.set", "error", scrub(err.Error()))
	}
	return t, nil
}

func (s *PolicyService) ListTriggers(ctx context.Context, scope coretenant.Scope) ([]domain.ProductTrigger, error) {
	return s.repo.ListProductTriggers(ctx, scope)
}

// maxTriggerEnrich bounds the per-product lookups of ListTriggersWithProducts
// (ProductCatalog has no batch Get). Triggers beyond it are returned with ids only.
const maxTriggerEnrich = 200

// TriggerItem is a product trigger with the product's display fields. Product
// is nil when the product could not be resolved or the enrich cap was hit.
type TriggerItem struct {
	Trigger domain.ProductTrigger
	Product *ProductInfo
}

// ListTriggersWithProducts returns the triggers of the organization enriched
// with product name/sku, at most maxTriggerEnrich lookups.
func (s *PolicyService) ListTriggersWithProducts(ctx context.Context, scope coretenant.Scope) ([]TriggerItem, error) {
	triggers, err := s.repo.ListProductTriggers(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]TriggerItem, 0, len(triggers))
	for i, t := range triggers {
		item := TriggerItem{Trigger: t}
		if i < maxTriggerEnrich {
			if p, err := s.products.Get(ctx, scope, t.ProductID); err == nil {
				item.Product = &p
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// UnmarkedProducts lists recurring products used by active contracts that are
// not (or no longer) enabled as PPPoE services.
func (s *PolicyService) UnmarkedProducts(ctx context.Context, scope coretenant.Scope) ([]ProductInfo, error) {
	inUse, err := s.products.ListRecurringInUse(ctx, scope)
	if err != nil {
		return nil, err
	}
	triggers, err := s.repo.ListProductTriggers(ctx, scope)
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]bool, len(triggers))
	for _, t := range triggers {
		if t.Enabled {
			enabled[t.ProductID] = true
		}
	}
	out := make([]ProductInfo, 0, len(inUse))
	for _, p := range inUse {
		if !enabled[p.ID] {
			out = append(out, p)
		}
	}
	return out, nil
}
