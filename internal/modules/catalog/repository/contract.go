package repository

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/shared/pricing"
)

var (
	ErrSKUExists          = errors.New("sku already used by another product")
	ErrCategoryNameExists = errors.New("category name already exists")
)

type ProductListFilter struct {
	Search     string // nama atau SKU
	CategoryID string
	IsActive   *bool
	Limit      int
	Offset     int
}

type CreateProductParams struct {
	CategoryID  string
	SKU         string
	Name        string
	Description string
	Unit        string
	BasePrice   string
	TaxPercent  string
	Currency    string
	Pricing     pricing.Attributes
	IsActive    bool
	CreatedBy   string
}

// UpdateProductParams: field nil tidak diubah; string kosong pada field
// nullable (CategoryID, SKU, Description) mengosongkan kolom.
type UpdateProductParams struct {
	CategoryID  *string
	SKU         *string
	Name        *string
	Description *string
	Unit        *string
	BasePrice   *string
	TaxPercent  *string
	Pricing     *pricing.Attributes // dikirim utuh (tiga field); nil = tidak diubah
	IsActive    *bool
	UpdatedBy   string
}

type ProductRepository interface {
	Create(ctx context.Context, scope coretenant.Scope, p CreateProductParams) (domain.Product, error)
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Product, error)
	// FindByIDs mengembalikan produk aktif maupun nonaktif, tetapi tidak yang terhapus.
	FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]domain.Product, error)
	List(ctx context.Context, scope coretenant.Scope, f ProductListFilter) ([]domain.Product, int64, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, p UpdateProductParams) (domain.Product, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, deletedBy string) error
}

type CategoryRepository interface {
	Create(ctx context.Context, scope coretenant.Scope, name string, position int, createdBy string) (domain.Category, error)
	List(ctx context.Context, scope coretenant.Scope) ([]domain.Category, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, name *string, position *int, updatedBy string) (domain.Category, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, deletedBy string) error
}
