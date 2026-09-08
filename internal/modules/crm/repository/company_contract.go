package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type CompanyListFilter struct {
	Search         string
	OwnerUserID    string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreateCompanyParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateCompanyParams struct {
	Name        string
	Industry    string
	Website     string
	Phone       string
	Email       string
	Address     map[string]any
	SizeRange   string
	Notes       string
	Tags        []string
	OwnerUserID string
	CreatedBy   string
}

type UpdateCompanyParams struct {
	Name        *string
	Industry    *string
	Website     *string
	Phone       *string
	Email       *string
	Address     map[string]any
	SizeRange   *string
	Notes       *string
	Tags        []string
	OwnerUserID *string
	UpdatedBy   string
}

// CompanyRepository is the tenant-owned data contract. Every method requires
// a verified immutable scope and row lookups include both scope and resource ID.
type CompanyRepository interface {
	Create(context.Context, coretenant.Scope, CreateCompanyParams) (domain.Company, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Company, error)
	List(context.Context, coretenant.Scope, CompanyListFilter) ([]domain.Company, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateCompanyParams) (domain.Company, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
