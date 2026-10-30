package app

import (
	"context"

	"zyad.cloud/internal/modules/catalog/domain"
	productmodel "zyad.cloud/internal/modules/product/model"
	productrepo "zyad.cloud/internal/modules/product/repository"
)

// registryPageSize sama dengan batas atas FeatureRepository.List; registry dibaca per halaman.
const registryPageSize = 100

// catalogFeatureRegistry menghubungkan modul catalog ke registry fitur modul
// product tanpa membuat catalog mengimpor product.
type catalogFeatureRegistry struct {
	features *productrepo.FeatureRepository
}

func (r catalogFeatureRegistry) ListActive(ctx context.Context) ([]domain.FeatureDef, error) {
	active := true
	rows, err := r.all(ctx, productrepo.FeatureListFilter{IsActive: &active})
	if err != nil {
		return nil, err
	}
	out := make([]domain.FeatureDef, 0, len(rows))
	for _, f := range rows {
		out = append(out, toFeatureDef(f))
	}
	return out, nil
}

func (r catalogFeatureRegistry) FindByKeys(ctx context.Context, keys []string) (map[string]domain.FeatureDef, error) {
	rows, err := r.all(ctx, productrepo.FeatureListFilter{})
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	out := make(map[string]domain.FeatureDef, len(keys))
	for _, f := range rows {
		if want[f.Key] {
			out[f.Key] = toFeatureDef(f)
		}
	}
	return out, nil
}

// all membaca seluruh halaman registry (kecil, puluhan baris).
func (r catalogFeatureRegistry) all(ctx context.Context, f productrepo.FeatureListFilter) ([]productmodel.Feature, error) {
	f.Limit = registryPageSize
	var out []productmodel.Feature
	for {
		f.Offset = len(out)
		page, total, err := r.features.List(ctx, f)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) == 0 || int64(len(out)) >= total {
			return out, nil
		}
	}
}

func toFeatureDef(f productmodel.Feature) domain.FeatureDef {
	return domain.FeatureDef{
		Key: f.Key, Module: f.Module, Name: f.Name, ValueType: string(f.ValueType), Unit: f.Unit,
		ResetStrategy: string(f.ResetStrategy), IsActive: f.IsActive,
		TenantOfferable: f.TenantOfferable, TenantMax: f.TenantMax,
	}
}
