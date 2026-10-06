package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

const maxDealTitle = 120

// DefaultDealTitle: ringkasan kebutuhan (dipotong 120 karakter) atau
// "Deal {nama lead}". Dipakai FE untuk prefill dan BE sebagai fallback.
func DefaultDealTitle(lead domain.Lead) string {
	summary := strings.Join(strings.Fields(lead.RequirementSummary), " ")
	if summary == "" {
		return "Deal " + lead.ContactName
	}
	if utf8.RuneCountInString(summary) > maxDealTitle {
		return string([]rune(summary)[:maxDealTitle-1]) + "…"
	}
	return summary
}

func (s *leadService) Convert(ctx context.Context, scope coretenant.Scope, id string, params ConvertLeadParams) (domain.LeadConversionResult, error) {
	lead, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.LeadConversionResult{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	if lead.Status == domain.LeadStatusConverted {
		return domain.LeadConversionResult{}, ErrLeadAlreadyConverted
	}

	tx := repository.ConvertLeadTxParams{
		Contact: repository.CreateContactParams{
			FirstName:      lead.ContactName,
			Email:          lead.Email,
			Phone:          lead.Phone,
			JobTitle:       lead.JobTitle,
			Address:        lead.Address,
			Source:         lead.Source,
			OwnerUserID:    params.OwnerUserID,
			LifecycleStage: domain.ContactLifecycleContact,
			CreatedBy:      params.ConvertedBy,
		},
		UpdatedBy: params.ConvertedBy,
	}

	if err := s.applyCompanyInput(ctx, scope, lead, params, &tx); err != nil {
		return domain.LeadConversionResult{}, err
	}
	if err := s.applyContactInput(ctx, scope, params.Contact, &tx); err != nil {
		return domain.LeadConversionResult{}, err
	}
	if params.Deal != nil {
		dealInput := *params.Deal
		if dealInput.OwnerUserID == "" {
			// Owner deal default ke owner contact hasil convert.
			dealInput.OwnerUserID = params.OwnerUserID
		}
		deal, err := s.buildDealParams(ctx, scope, dealInput, params.ConvertedBy)
		if err != nil {
			return domain.LeadConversionResult{}, err
		}
		tx.Deal = &deal
	}
	if err := s.requireOwnerMember(ctx, scope, params.OwnerUserID); err != nil {
		return domain.LeadConversionResult{}, err
	}
	if tx.ExistingContactID == "" { // contact existing tidak menambah pemakaian kuota
		if err := s.requireContactQuota(ctx, scope); err != nil {
			return domain.LeadConversionResult{}, err
		}
	}

	result, err := s.repo.ConvertLead(ctx, scope, id, tx)
	if err != nil {
		return domain.LeadConversionResult{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	if s.onConvert != nil {
		// Konversi sudah tersimpan; kegagalan hook tidak membatalkannya.
		if err := s.onConvert.LeadConverted(ctx, scope, id, result.Contact.ID); err != nil {
			slog.WarnContext(ctx, "lead converted hook failed", "lead_id", id, "contact_id", result.Contact.ID, "error", err)
		}
	}
	return result, nil
}

func (s *leadService) applyContactInput(ctx context.Context, scope coretenant.Scope, in *ConvertContactInput, tx *repository.ConvertLeadTxParams) error {
	if in == nil || in.Mode == "" || in.Mode == "new" {
		return nil
	}
	if in.Mode != "existing" || in.ContactID == "" || s.contactRepo == nil {
		return ErrInvalidContactInput
	}
	if _, err := s.contactRepo.FindByID(ctx, scope, in.ContactID); err != nil {
		return crmmodule.MapNotFound(err, "CONTACT_NOT_FOUND", "contact not found or already deleted")
	}
	tx.ExistingContactID = in.ContactID
	return nil
}

func (s *leadService) applyCompanyInput(ctx context.Context, scope coretenant.Scope, lead domain.Lead, params ConvertLeadParams, tx *repository.ConvertLeadTxParams) error {
	in := params.Company
	if in == nil { // kontrak lama
		if params.CreateCompany && strings.TrimSpace(lead.CompanyName) != "" {
			tx.NewCompany = &repository.CreateCompanyParams{
				Name: strings.TrimSpace(lead.CompanyName), OwnerUserID: params.OwnerUserID, CreatedBy: params.ConvertedBy,
			}
		}
		return nil
	}
	switch in.Mode {
	case "none":
		return nil
	case "existing":
		if in.CompanyID == "" || s.companyRepo == nil {
			return ErrInvalidCompanyInput
		}
		if _, err := s.companyRepo.FindByID(ctx, scope, in.CompanyID); err != nil {
			return crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
		}
		tx.ExistingCompanyID = in.CompanyID
		return nil
	case "new":
		name := strings.TrimSpace(in.Name)
		if name == "" || utf8.RuneCountInString(name) > 200 {
			return ErrInvalidCompanyInput
		}
		tx.NewCompany = &repository.CreateCompanyParams{
			Name: name, Industry: strings.TrimSpace(in.Industry), Website: strings.TrimSpace(in.Website),
			Phone: strings.TrimSpace(in.Phone), OwnerUserID: params.OwnerUserID, CreatedBy: params.ConvertedBy,
		}
		return nil
	default:
		return ErrInvalidCompanyInput
	}
}

// buildDealParams memvalidasi input deal: pipeline milik org & tidak
// diarsipkan, stage milik pipeline itu & bukan Won/Lost, judul, nilai, tanggal.
func (s *leadService) buildDealParams(ctx context.Context, scope coretenant.Scope, in ConvertDealInput, createdBy string) (repository.CreateDealParams, error) {
	if s.pipelineRepo == nil {
		return repository.CreateDealParams{}, ErrInvalidPipelineStage
	}
	pipeline, err := s.pipelineRepo.FindByID(ctx, scope, in.PipelineID)
	if err != nil || pipeline.ArchivedAt != nil {
		return repository.CreateDealParams{}, ErrInvalidPipelineStage
	}
	var stage *domain.PipelineStage
	for i := range pipeline.Stages {
		if pipeline.Stages[i].ID == in.StageID {
			stage = &pipeline.Stages[i]
		}
	}
	if stage == nil {
		return repository.CreateDealParams{}, ErrInvalidPipelineStage
	}
	if stage.IsWon || stage.IsLost {
		return repository.CreateDealParams{}, ErrInvalidStartStage
	}

	title := strings.TrimSpace(in.Title)
	value := strings.TrimSpace(in.Value)
	if value == "" {
		value = "0"
	}
	if title == "" || utf8.RuneCountInString(title) > 200 || !annualRevenuePattern.MatchString(value) {
		return repository.CreateDealParams{}, ErrInvalidDealInput
	}
	var closeDate *time.Time
	if in.ExpectedCloseDate != "" {
		d, err := time.Parse("2006-01-02", in.ExpectedCloseDate)
		if err != nil {
			return repository.CreateDealParams{}, ErrInvalidDealInput
		}
		closeDate = &d
	}
	if err := s.requireOwnerMember(ctx, scope, in.OwnerUserID); err != nil {
		return repository.CreateDealParams{}, err
	}
	return repository.CreateDealParams{
		PipelineID: pipeline.ID, StageID: stage.ID, Title: title, Value: value,
		ExpectedCloseDate: closeDate, OwnerUserID: in.OwnerUserID, CreatedBy: createdBy,
		Description: strings.TrimSpace(in.Description), DecisionMaker: strings.TrimSpace(in.DecisionMaker),
	}, nil
}

func (s *leadService) CreateDealForLead(ctx context.Context, scope coretenant.Scope, leadID string, input ConvertDealInput, createdBy string) (domain.Lead, domain.Deal, error) {
	params, err := s.buildDealParams(ctx, scope, input, createdBy)
	if err != nil {
		return domain.Lead{}, domain.Deal{}, err
	}
	lead, deal, err := s.repo.AttachDeal(ctx, scope, leadID, params)
	if err != nil {
		return domain.Lead{}, domain.Deal{}, crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
	}
	return lead, deal, nil
}
