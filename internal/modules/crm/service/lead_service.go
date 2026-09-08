package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type leadService struct {
	repo        repository.LeadRepository
	contactRepo repository.ContactRepository
	companyRepo repository.CompanyRepository
	quotaGuard  ContactQuotaGuard
}

type LeadServiceOption func(*leadService)

// WithLeadContactQuotaGuard enforces crm.max_contacts when a lead conversion
// creates a new contact — the same guard/feature key as ContactService.Create.
func WithLeadContactQuotaGuard(guard ContactQuotaGuard) LeadServiceOption {
	return func(service *leadService) {
		service.quotaGuard = guard
	}
}

func NewLeadService(
	repo repository.LeadRepository,
	contactRepo repository.ContactRepository,
	companyRepo repository.CompanyRepository,
	options ...LeadServiceOption,
) LeadService {
	service := &leadService{
		repo:        repo,
		contactRepo: contactRepo,
		companyRepo: companyRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *leadService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateLeadParams) (domain.Lead, error) {
	return s.repo.Create(ctx, scope, params)
}

func (s *leadService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Lead, error) {
	lead, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return lead, nil
}

func (s *leadService) List(ctx context.Context, scope coretenant.Scope, filter repository.LeadListFilter) ([]domain.Lead, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *leadService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateLeadParams) (domain.Lead, error) {
	lead, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return lead, nil
}

func (s *leadService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "LEAD_NOT_FOUND", "lead not found or already deleted")
}

func (s *leadService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "LEAD_NOT_FOUND", "lead not found or not deleted")
}

func (s *leadService) Assign(ctx context.Context, scope coretenant.Scope, id string, ownerUserID string, updatedBy string) (domain.Lead, error) {
	lead, err := s.repo.Assign(ctx, scope, id, ownerUserID, updatedBy)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return lead, nil
}

// Convert creates a Contact (and optionally a Company) from a lead's captured
// details and marks the lead as converted.
//
// This is not atomic across crm_leads/crm_contacts/crm_companies — each
// repository call commits its own transaction (matching the per-resource
// withTx pattern used across this module). If MarkConverted fails after the
// Contact/Company were created, those rows remain as valid (if orphaned)
// records rather than being rolled back; callers should treat a failed
// Convert as needing manual follow-up, not automatic retry.
func (s *leadService) Convert(ctx context.Context, scope coretenant.Scope, id string, params ConvertLeadParams) (domain.LeadConversionResult, error) {
	lead, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.LeadConversionResult{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	if lead.Status == domain.LeadStatusConverted {
		return domain.LeadConversionResult{}, ErrLeadAlreadyConverted
	}

	var company *domain.Company
	companyID := ""
	if params.CreateCompany && lead.CompanyName != "" {
		created, err := s.companyRepo.Create(ctx, scope, repository.CreateCompanyParams{
			Name:        lead.CompanyName,
			OwnerUserID: params.OwnerUserID,
			CreatedBy:   params.ConvertedBy,
		})
		if err != nil {
			return domain.LeadConversionResult{}, err
		}
		company = &created
		companyID = created.ID
	}

	if err := s.requireContactQuota(ctx, scope); err != nil {
		return domain.LeadConversionResult{}, err
	}

	contact, err := s.contactRepo.Create(ctx, scope, repository.CreateContactParams{
		CompanyID:      companyID,
		FirstName:      lead.ContactName,
		Email:          lead.Email,
		Phone:          lead.Phone,
		Source:         lead.Source,
		OwnerUserID:    params.OwnerUserID,
		LifecycleStage: domain.ContactLifecycleContact,
		CreatedBy:      params.ConvertedBy,
	})
	if err != nil {
		return domain.LeadConversionResult{}, err
	}

	convertedLead, err := s.repo.MarkConverted(ctx, scope, id, repository.MarkConvertedParams{
		ConvertedContactID: contact.ID,
		ConvertedCompanyID: companyID,
		UpdatedBy:          params.ConvertedBy,
	})
	if err != nil {
		return domain.LeadConversionResult{}, err
	}

	return domain.LeadConversionResult{
		Lead:    convertedLead,
		Contact: contact,
		Company: company,
	}, nil
}

func (s *leadService) requireContactQuota(ctx context.Context, scope coretenant.Scope) error {
	if s.quotaGuard == nil {
		return nil
	}
	total, err := s.contactRepo.Count(ctx, scope, repository.ContactListFilter{})
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
