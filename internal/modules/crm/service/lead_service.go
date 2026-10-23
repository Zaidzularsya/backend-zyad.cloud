package service

import (
	"context"
	"regexp"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type leadService struct {
	repo         repository.LeadRepository
	contactRepo  repository.ContactRepository
	companyRepo  repository.CompanyRepository
	quotaGuard   ContactQuotaGuard
	ownerCheck   LeadOwnerValidator
	onConvert    LeadConvertedHook
	pipelineRepo repository.PipelineRepository
	quota        QuotaRoomRequirer
}

// annualRevenuePattern mengikuti batas kolom numeric(18,2): maks 16 digit
// bulat + 2 desimal, tanpa tanda minus.
var annualRevenuePattern = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)

type LeadServiceOption func(*leadService)

// WithLeadContactQuotaGuard enforces crm.max_contacts when a lead conversion
// creates a new contact — the same guard/feature key as ContactService.Create.
func WithLeadContactQuotaGuard(guard ContactQuotaGuard) LeadServiceOption {
	return func(service *leadService) {
		service.quotaGuard = guard
	}
}

// WithLeadQuotaEnforcer enforces crm.max_leads in Create, and also
// crm.max_companies / crm.max_deals when Convert creates a new company or deal
// (those rows bypass CompanyService/DealService). Update, Get and List never
// consult it.
func WithLeadQuotaEnforcer(quota QuotaRoomRequirer) LeadServiceOption {
	return func(service *leadService) {
		service.quota = quota
	}
}

// requireRoom is a no-op when no enforcer is wired (tests, legacy callers).
func (s *leadService) requireRoom(ctx context.Context, scope coretenant.Scope, featureKey string) error {
	if s.quota == nil {
		return nil
	}
	return s.quota.RequireRoom(ctx, scope.OrganizationID(), featureKey)
}

// WithLeadOwnerValidator mewajibkan owner_user_id (create/update/assign)
// menunjuk anggota aktif organization.
func WithLeadOwnerValidator(validator LeadOwnerValidator) LeadServiceOption {
	return func(service *leadService) {
		service.ownerCheck = validator
	}
}

// WithLeadConvertedHook dipanggil setelah lead berhasil di-convert, mis.
// untuk memindahkan percakapan WhatsApp lead ke contact barunya.
func WithLeadConvertedHook(hook LeadConvertedHook) LeadServiceOption {
	return func(service *leadService) {
		service.onConvert = hook
	}
}

// WithLeadDealPipelines mengaktifkan pembuatan deal saat convert (validasi
// pipeline/stage). Tanpa option ini, input deal ditolak ErrInvalidPipelineStage.
func WithLeadDealPipelines(repo repository.PipelineRepository) LeadServiceOption {
	return func(service *leadService) {
		service.pipelineRepo = repo
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
	if err := validateAnnualRevenue(params.AnnualRevenue); err != nil {
		return domain.Lead{}, err
	}
	if err := s.requireOwnerMember(ctx, scope, params.OwnerUserID); err != nil {
		return domain.Lead{}, err
	}
	if err := s.requireRoom(ctx, scope, domain.FeatureCRMMaxLeads); err != nil {
		return domain.Lead{}, err
	}
	return s.repo.Create(ctx, scope, params)
}

func (s *leadService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Lead, error) {
	lead, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return s.withPlaybook(ctx, scope, lead)
}

func (s *leadService) List(ctx context.Context, scope coretenant.Scope, filter repository.LeadListFilter) ([]domain.Lead, int64, error) {
	leads, total, err := s.repo.List(ctx, scope, filter)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(leads))
	for _, l := range leads {
		ids = append(ids, l.ID)
	}
	sums, err := s.repo.FindPlaybookSummaries(ctx, scope, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range leads {
		if sum, ok := sums[leads[i].ID]; ok {
			sum := sum
			leads[i].Playbook = &sum
		}
	}
	return leads, total, nil
}

func (s *leadService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateLeadParams) (domain.Lead, error) {
	// Terminal statuses carry side effects (reason, contact creation, run end)
	// that only their dedicated endpoints perform.
	if params.Status != nil && (*params.Status == domain.LeadStatusUnqualified || *params.Status == domain.LeadStatusConverted) {
		return domain.Lead{}, ErrUseDisqualifyEndpoint
	}
	if params.BudgetEstimate != nil && *params.BudgetEstimate != "" && !annualRevenuePattern.MatchString(*params.BudgetEstimate) {
		return domain.Lead{}, ErrInvalidBudgetEstimate
	}
	if params.TargetDate != nil && *params.TargetDate != "" {
		if _, err := time.Parse("2006-01-02", *params.TargetDate); err != nil {
			return domain.Lead{}, ErrInvalidTargetDate
		}
	}
	if params.AnnualRevenue != nil {
		if err := validateAnnualRevenue(*params.AnnualRevenue); err != nil {
			return domain.Lead{}, err
		}
	}
	if params.OwnerUserID != nil {
		if err := s.requireOwnerMember(ctx, scope, *params.OwnerUserID); err != nil {
			return domain.Lead{}, err
		}
	}
	if err := s.requireRoomToReactivate(ctx, scope, id, params.Status); err != nil {
		return domain.Lead{}, err
	}
	lead, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return s.withPlaybook(ctx, scope, lead)
}

// requireRoomToReactivate: a status change that takes an unqualified/converted
// lead back to a working status re-enters the active lead count (see the
// crm.max_leads meter), so it needs room. Other edits stay unguarded. The new
// status is never terminal here (Update rejects those above).
func (s *leadService) requireRoomToReactivate(ctx context.Context, scope coretenant.Scope, id string, status *domain.LeadStatus) error {
	if s.quota == nil || status == nil {
		return nil
	}
	current, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	if current.Status != domain.LeadStatusUnqualified && current.Status != domain.LeadStatusConverted {
		return nil
	}
	return s.requireRoom(ctx, scope, domain.FeatureCRMMaxLeads)
}

func (s *leadService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "LEAD_NOT_FOUND", "lead not found or already deleted")
}

// Restore brings the lead back into the active count, so it needs room like Create.
func (s *leadService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if err := s.requireRoom(ctx, scope, domain.FeatureCRMMaxLeads); err != nil {
		return err
	}
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "LEAD_NOT_FOUND", "lead not found or not deleted")
}

func (s *leadService) Assign(ctx context.Context, scope coretenant.Scope, id string, ownerUserID string, updatedBy string) (domain.Lead, error) {
	if err := s.requireOwnerMember(ctx, scope, ownerUserID); err != nil {
		return domain.Lead{}, err
	}
	lead, err := s.repo.Assign(ctx, scope, id, ownerUserID, updatedBy)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return lead, nil
}

// requireOwnerMember: string kosong berarti "tanpa owner" dan selalu lolos.
func (s *leadService) requireOwnerMember(ctx context.Context, scope coretenant.Scope, ownerUserID string) error {
	if s.ownerCheck == nil || ownerUserID == "" {
		return nil
	}
	ok, err := s.ownerCheck.IsActiveMember(ctx, scope, ownerUserID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeadOwnerNotMember
	}
	return nil
}

// validateAnnualRevenue: string kosong berarti "tidak diisi" (NULL).
func validateAnnualRevenue(value string) error {
	if value == "" || annualRevenuePattern.MatchString(value) {
		return nil
	}
	return ErrInvalidAnnualRevenue
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

func (s *leadService) Disqualify(ctx context.Context, scope coretenant.Scope, id string, params repository.DisqualifyLeadParams) (domain.Lead, error) {
	if !params.Reason.IsValid() {
		return domain.Lead{}, ErrInvalidDisqualifyReason
	}
	lead, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	if lead.Status == domain.LeadStatusConverted {
		return domain.Lead{}, ErrLeadAlreadyConverted
	}
	updated, err := s.repo.Disqualify(ctx, scope, id, params)
	if err != nil {
		return domain.Lead{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return s.withPlaybook(ctx, scope, updated)
}

func (s *leadService) StartPlaybook(ctx context.Context, scope coretenant.Scope, id string, startedBy string) (domain.PlaybookRun, error) {
	run, err := s.repo.StartPlaybook(ctx, scope, id, startedBy)
	return run, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
}

func (s *leadService) ListEvents(ctx context.Context, scope coretenant.Scope, id string, page, perPage int) ([]domain.LeadEvent, int64, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	events, total, err := s.repo.ListEvents(ctx, scope, id, perPage, (page-1)*perPage)
	return events, total, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found")
}

func (s *leadService) withPlaybook(ctx context.Context, scope coretenant.Scope, lead domain.Lead) (domain.Lead, error) {
	sums, err := s.repo.FindPlaybookSummaries(ctx, scope, []string{lead.ID})
	if err != nil {
		return domain.Lead{}, err
	}
	if sum, ok := sums[lead.ID]; ok {
		lead.Playbook = &sum
	}
	return lead, nil
}
