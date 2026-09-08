package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type CreateIntegrationInput struct {
	Provider  domain.IntegrationProvider
	Name      string
	Config    map[string]any
	Secret    string
	CreatedBy string
}

type UpdateIntegrationInput struct {
	Name      *string
	Config    map[string]any
	IsActive  *bool
	UpdatedBy string
}

type IntegrationService interface {
	Create(context.Context, coretenant.Scope, CreateIntegrationInput) (domain.Integration, error)
	Get(context.Context, coretenant.Scope, string) (domain.Integration, error)
	List(context.Context, coretenant.Scope, repository.IntegrationListFilter) ([]domain.Integration, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateIntegrationInput) (domain.Integration, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Connect(context.Context, coretenant.Scope, string, string) (domain.Integration, error)
	// RevealSecret decrypts and returns the plaintext secret — callers must
	// gate this behind the integration.view_secret permission (see
	// handler.IntegrationHandler.RevealSecret).
	RevealSecret(context.Context, coretenant.Scope, string) (string, error)
	// UpdateSecret encrypts and replaces the stored secret — callers must
	// gate this behind integration.update_secret.
	UpdateSecret(context.Context, coretenant.Scope, string, string, string) (domain.Integration, error)
}
