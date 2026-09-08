package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type IntegrationListFilter struct {
	Provider       domain.IntegrationProvider
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreateIntegrationParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope. SecretEncrypted is already
// ciphertext by the time it reaches the repository — encryption happens in
// IntegrationService.
type CreateIntegrationParams struct {
	Provider        domain.IntegrationProvider
	Name            string
	Config          map[string]any
	SecretEncrypted string
	CreatedBy       string
}

type UpdateIntegrationParams struct {
	Name            *string
	Config          map[string]any
	SecretEncrypted *string
	IsActive        *bool
	UpdatedBy       string
}

// IntegrationRepository is the tenant-owned data contract. Every method
// requires a verified immutable scope and row lookups include both scope and
// resource ID.
type IntegrationRepository interface {
	Create(context.Context, coretenant.Scope, CreateIntegrationParams) (domain.Integration, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Integration, error)
	List(context.Context, coretenant.Scope, IntegrationListFilter) ([]domain.Integration, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateIntegrationParams) (domain.Integration, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Connect(context.Context, coretenant.Scope, string, string) (domain.Integration, error)
	UpdateSecret(context.Context, coretenant.Scope, string, string, string) (domain.Integration, error)
}
