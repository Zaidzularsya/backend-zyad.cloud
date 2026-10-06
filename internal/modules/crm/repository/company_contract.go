package repository

import (
	"context"
	"errors"

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
	// FindCandidatesByName: kandidat kasar untuk pengecekan duplikat
	// (lower(name) mengandung needle). needle sudah lowercase.
	FindCandidatesByName(ctx context.Context, scope coretenant.Scope, needle string, limit int) ([]domain.Company, error)
	Create(context.Context, coretenant.Scope, CreateCompanyParams) (domain.Company, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Company, error)
	List(context.Context, coretenant.Scope, CompanyListFilter) ([]domain.Company, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateCompanyParams) (domain.Company, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	// FindByTenantOrganization mengembalikan company yang tertaut ke workspace
	// tenantOrgID, atau pgx.ErrNoRows.
	FindByTenantOrganization(ctx context.Context, scope coretenant.Scope, tenantOrgID string) (domain.Company, error)
	// SetTenantOrganization menautkan company ke workspace; ErrWorkspaceAlreadyLinked
	// bila workspace itu sudah tertaut ke company lain di org yang sama.
	SetTenantOrganization(ctx context.Context, scope coretenant.Scope, companyID, tenantOrgID, updatedBy string) (domain.Company, error)
}

var ErrWorkspaceAlreadyLinked = errors.New("workspace already linked to another company")
