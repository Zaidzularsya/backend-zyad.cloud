package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// ContactQuotaGuard is a narrow interface over SubscriptionGuardService,
// mirroring the pattern in internal/modules/landing/service/page_service.go
// (LandingPageQuotaGuard).
type ContactQuotaGuard interface {
	RequireQuotaValue(
		ctx context.Context,
		organizationID string,
		featureKey string,
		limitKey string,
		usedValue int64,
		delta int64,
	) error
}

type contactService struct {
	repo       repository.ContactRepository
	quotaGuard ContactQuotaGuard
}

type ContactServiceOption func(*contactService)

func WithContactQuotaGuard(guard ContactQuotaGuard) ContactServiceOption {
	return func(service *contactService) {
		service.quotaGuard = guard
	}
}

func NewContactService(repo repository.ContactRepository, options ...ContactServiceOption) ContactService {
	service := &contactService{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *contactService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateContactParams) (domain.Contact, error) {
	if err := s.requireCreateQuota(ctx, scope); err != nil {
		return domain.Contact{}, err
	}
	return s.repo.Create(ctx, scope, params)
}

func (s *contactService) requireCreateQuota(ctx context.Context, scope coretenant.Scope) error {
	if s.quotaGuard == nil {
		return nil
	}
	total, err := s.repo.Count(ctx, scope, repository.ContactListFilter{})
	if err != nil {
		return err
	}
	return s.quotaGuard.RequireQuotaValue(
		ctx,
		scope.OrganizationID(),
		domain.FeatureCRMMaxContacts,
		"limit",
		total,
		1,
	)
}

func (s *contactService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Contact, error) {
	contact, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Contact{}, crmmodule.MapNotFound(err, "CONTACT_NOT_FOUND", "contact not found or already deleted")
	}
	return contact, nil
}

func (s *contactService) List(ctx context.Context, scope coretenant.Scope, filter repository.ContactListFilter) ([]domain.Contact, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *contactService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateContactParams) (domain.Contact, error) {
	contact, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Contact{}, crmmodule.MapNotFound(err, "CONTACT_NOT_FOUND", "contact not found or already deleted")
	}
	return contact, nil
}

func (s *contactService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "CONTACT_NOT_FOUND", "contact not found or already deleted")
}

func (s *contactService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "CONTACT_NOT_FOUND", "contact not found or not deleted")
}
