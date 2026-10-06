package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

var (
	ErrInvalidProduct  = errors.New("name (max 200) and unit (max 30) are required; base_price must be a non-negative number with at most 2 decimals; tax_percent must be between 0 and 100; sku max 64; " + pricing.ErrInvalidAttributes.Error())
	ErrInvalidCategory = errors.New("category name is required (max 100)")

	ErrPlatformOnly   = errors.New("Fitur dan publikasi hanya untuk katalog platform")
	ErrInvalidListing = errors.New("listing_code wajib (huruf kecil, angka, tanda -, maks 50) dan kategori wajib bila produk dipublikasikan")
	ErrInvalidFeature = errors.New("fitur tidak dikenal/nonaktif, nilai tidak sesuai tipe, label > 200, atau lebih dari 30 fitur")
)

// FeatureRegistry membaca registry fitur (modul product); dirangkai di internal/app.
type FeatureRegistry interface {
	ListActive(ctx context.Context) ([]domain.FeatureDef, error)
	FindByKeys(ctx context.Context, keys []string) (map[string]domain.FeatureDef, error) // termasuk yang nonaktif
}

type ProductInput struct {
	CategoryID  string
	SKU         string
	Name        string
	Description string
	Unit        string
	BasePrice   string
	TaxPercent  string
	Pricing     pricing.Attributes
	IsActive    bool

	Platform bool // org pemanggil adalah org platform; fitur & listing hanya untuk platform
	Listing  repository.ListingParams
	Features []repository.FeatureValue
}

type ProductService interface {
	Create(ctx context.Context, scope coretenant.Scope, in ProductInput, userID string) (domain.Product, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Product, error)
	List(ctx context.Context, scope coretenant.Scope, f repository.ProductListFilter) ([]domain.Product, int64, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, in repository.UpdateProductParams, platform bool) (domain.Product, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, userID string) error
	// FindByIDs memenuhi interface katalog CRM; label fitur terisi.
	FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]domain.Product, error)
	ListFeatures(ctx context.Context) ([]domain.FeatureDef, error)
}

type CategoryService interface {
	Create(ctx context.Context, scope coretenant.Scope, name string, position int, userID string) (domain.Category, error)
	List(ctx context.Context, scope coretenant.Scope) ([]domain.Category, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, name *string, position *int, userID string) (domain.Category, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, userID string) error
}
