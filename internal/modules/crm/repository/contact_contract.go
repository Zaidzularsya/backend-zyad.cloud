package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type ContactListFilter struct {
	Search         string
	CompanyID      string
	OwnerUserID    string
	LifecycleStage domain.ContactLifecycleStage
	IsCustomer     *bool
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreateContactParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateContactParams struct {
	CompanyID      string
	FirstName      string
	LastName       string
	Email          string
	Phone          string
	JobTitle       string
	Address        map[string]any
	Tags           []string
	Source         string
	OwnerUserID    string
	IsCustomer     bool
	LifecycleStage domain.ContactLifecycleStage
	CreatedBy      string
}

type UpdateContactParams struct {
	CompanyID      *string
	FirstName      *string
	LastName       *string
	Email          *string
	Phone          *string
	JobTitle       *string
	Address        map[string]any
	Tags           []string
	Source         *string
	OwnerUserID    *string
	IsCustomer     *bool
	LifecycleStage *domain.ContactLifecycleStage
	UpdatedBy      string
}

// ContactRepository is the tenant-owned data contract. Every method requires
// a verified immutable scope and row lookups include both scope and resource ID.
type ContactRepository interface {
	Create(context.Context, coretenant.Scope, CreateContactParams) (domain.Contact, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Contact, error)
	List(context.Context, coretenant.Scope, ContactListFilter) ([]domain.Contact, int64, error)
	Count(context.Context, coretenant.Scope, ContactListFilter) (int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateContactParams) (domain.Contact, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
