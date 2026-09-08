package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type LeadListFilter struct {
	Search         string
	Status         domain.LeadStatus
	OwnerUserID    string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreateLeadParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateLeadParams struct {
	ContactName string
	CompanyName string
	Email       string
	Phone       string
	Source      string
	Score       int
	OwnerUserID string
	Notes       string
	CreatedBy   string
}

type UpdateLeadParams struct {
	ContactName *string
	CompanyName *string
	Email       *string
	Phone       *string
	Source      *string
	Status      *domain.LeadStatus
	Score       *int
	OwnerUserID *string
	Notes       *string
	UpdatedBy   string
}

// MarkConvertedParams records the result of converting a lead into working
// CRM records. Called after the Contact/Company (and, from Fase 2 onward,
// Deal) rows already exist.
type MarkConvertedParams struct {
	ConvertedContactID string
	ConvertedCompanyID string
	UpdatedBy          string
}

// LeadRepository is the tenant-owned data contract. Every method requires a
// verified immutable scope and row lookups include both scope and resource ID.
type LeadRepository interface {
	Create(context.Context, coretenant.Scope, CreateLeadParams) (domain.Lead, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Lead, error)
	List(context.Context, coretenant.Scope, LeadListFilter) ([]domain.Lead, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateLeadParams) (domain.Lead, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	Assign(context.Context, coretenant.Scope, string, string, string) (domain.Lead, error)
	MarkConverted(context.Context, coretenant.Scope, string, MarkConvertedParams) (domain.Lead, error)
}
