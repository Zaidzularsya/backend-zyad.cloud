package app

import (
	"context"
	"errors"
	"fmt"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

// ErrDefaultProductNotFound: produk paket gratis (SELF_SERVE_FREE_PRODUCT_SKU) belum ada atau nonaktif.
var ErrDefaultProductNotFound = errors.New("DEFAULT_PRODUCT_NOT_FOUND")

type defaultAccessScopes interface {
	PlatformScope(ctx context.Context) (coretenant.Scope, error)
}

type defaultAccessProducts interface {
	FindBySKU(ctx context.Context, scope coretenant.Scope, sku string) (catalogdomain.Product, error)
}

type defaultAccessFeatures interface {
	FindByKeys(ctx context.Context, keys []string) (map[string]catalogdomain.FeatureDef, error)
}

type defaultAccessWriter interface {
	GrantDefault(ctx context.Context, organizationID string, grants []organizationservice.FeatureGrant, actorUserID string) error
}

// defaultAccessProvisioner memberi workspace paket gratis dari fitur produk katalog platform ber-SKU
// SELF_SERVE_FREE_PRODUCT_SKU (default "FREE"). Aman dipanggil ulang: produk yang baru dibuat memperbaiki
// workspace yang onboarding-nya terlanjur gagal.
type defaultAccessProvisioner struct {
	scopes   defaultAccessScopes
	products defaultAccessProducts
	features defaultAccessFeatures
	writer   defaultAccessWriter
	sku      string
}

func (p defaultAccessProvisioner) ProvisionDefaultAccess(ctx context.Context, organizationID string) error {
	scope, err := p.scopes.PlatformScope(ctx)
	if err != nil {
		return fmt.Errorf("resolve platform scope: %w", err)
	}
	product, err := p.products.FindBySKU(ctx, scope, p.sku)
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) && appErr.Code == "PRODUCT_NOT_FOUND" {
			return fmt.Errorf("%w: sku %q", ErrDefaultProductNotFound, p.sku)
		}
		return fmt.Errorf("find default product %q: %w", p.sku, err)
	}
	snapshots := make([]catalogdomain.ProductFeature, len(product.Features))
	copy(snapshots, product.Features)
	keys := make([]string, len(snapshots))
	for i, f := range snapshots {
		keys[i] = f.FeatureKey
	}
	defs, err := p.features.FindByKeys(ctx, keys)
	if err != nil {
		return fmt.Errorf("read feature registry: %w", err)
	}
	grants := make([]organizationservice.FeatureGrant, 0, len(snapshots))
	for _, f := range snapshots {
		if def, ok := defs[f.FeatureKey]; ok {
			grants = append(grants, organizationservice.FeatureGrant{Key: f.FeatureKey, ValueType: def.ValueType, Value: f.Value})
		}
	}
	return p.writer.GrantDefault(ctx, organizationID, grants, "")
}
