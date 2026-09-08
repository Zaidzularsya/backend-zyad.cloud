package service

import (
	"context"
	"errors"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var ErrInvalidIntegrationProvider = errors.New("invalid integration provider")

type integrationService struct {
	repo          repository.IntegrationRepository
	encryptionKey string
}

// NewIntegrationService takes encryptionKey — the application secret used to
// derive an AES-256-GCM key for secret_encrypted (see
// internal/core/crypto.EncryptSecret's doc comment on the single-key
// no-rotation trade-off this implies).
//
// Get/List/Update/Connect/UpdateSecret all return domain.Integration with
// SecretEncrypted still set (the repository always loads it) — that's safe
// because dto.IntegrationFromDomain (internal/modules/crm/dto/response.go)
// never serializes the ciphertext, only a derived HasSecret bool. Only
// RevealSecret decrypts and returns plaintext, and only the handler route
// gated by integration.view_secret calls it.
func NewIntegrationService(repo repository.IntegrationRepository, encryptionKey string) IntegrationService {
	return &integrationService{repo: repo, encryptionKey: encryptionKey}
}

func (s *integrationService) Create(ctx context.Context, scope coretenant.Scope, input CreateIntegrationInput) (domain.Integration, error) {
	if !input.Provider.IsValid() {
		return domain.Integration{}, ErrInvalidIntegrationProvider
	}

	var secretEncrypted string
	if input.Secret != "" {
		encrypted, err := corecrypto.EncryptSecret(s.encryptionKey, input.Secret)
		if err != nil {
			return domain.Integration{}, err
		}
		secretEncrypted = encrypted
	}

	return s.repo.Create(ctx, scope, repository.CreateIntegrationParams{
		Provider:        input.Provider,
		Name:            input.Name,
		Config:          input.Config,
		SecretEncrypted: secretEncrypted,
		CreatedBy:       input.CreatedBy,
	})
}

func (s *integrationService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Integration, error) {
	integ, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Integration{}, crmmodule.MapNotFound(err, "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
	}
	return integ, nil
}

func (s *integrationService) List(ctx context.Context, scope coretenant.Scope, filter repository.IntegrationListFilter) ([]domain.Integration, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *integrationService) Update(ctx context.Context, scope coretenant.Scope, id string, input UpdateIntegrationInput) (domain.Integration, error) {
	integ, err := s.repo.Update(ctx, scope, id, repository.UpdateIntegrationParams{
		Name:      input.Name,
		Config:    input.Config,
		IsActive:  input.IsActive,
		UpdatedBy: input.UpdatedBy,
	})
	if err != nil {
		return domain.Integration{}, crmmodule.MapNotFound(err, "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
	}
	return integ, nil
}

func (s *integrationService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
}

func (s *integrationService) Connect(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Integration, error) {
	integ, err := s.repo.Connect(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Integration{}, crmmodule.MapNotFound(err, "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
	}
	return integ, nil
}

func (s *integrationService) RevealSecret(ctx context.Context, scope coretenant.Scope, id string) (string, error) {
	integ, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return "", crmmodule.MapNotFound(err, "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
	}
	if integ.SecretEncrypted == "" {
		return "", nil
	}
	return corecrypto.DecryptSecret(s.encryptionKey, integ.SecretEncrypted)
}

func (s *integrationService) UpdateSecret(ctx context.Context, scope coretenant.Scope, id string, secret string, updatedBy string) (domain.Integration, error) {
	encrypted, err := corecrypto.EncryptSecret(s.encryptionKey, secret)
	if err != nil {
		return domain.Integration{}, err
	}
	integ, err := s.repo.UpdateSecret(ctx, scope, id, encrypted, updatedBy)
	if err != nil {
		return domain.Integration{}, crmmodule.MapNotFound(err, "INTEGRATION_NOT_FOUND", "integration not found or already deleted")
	}
	return integ, nil
}
