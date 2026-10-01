package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrLeadAlreadyConverted = repository.ErrLeadAlreadyConverted
	// ErrLeadOwnerNotMember: owner_user_id harus anggota aktif organization
	// yang sama — mencegah lead di-assign ke (dan nama owner dibocorkan dari)
	// user tenant lain.
	ErrLeadOwnerNotMember   = errors.New("owner_user_id is not an active member of this organization")
	ErrInvalidAnnualRevenue = errors.New("annual_revenue must be a non-negative number with at most 2 decimals")

	ErrUseDisqualifyEndpoint   = errors.New("use POST /leads/:id/disqualify (unqualified) or /convert (converted) to change to this status")
	ErrInvalidDisqualifyReason = errors.New("reason must be one of: unresponsive, not_interested, not_fit, budget, competitor, bad_data, duplicate, bad_timing")
	ErrInvalidBudgetEstimate   = errors.New("budget_estimate must be a non-negative number with at most 2 decimals")
	ErrInvalidTargetDate       = errors.New("target_date must be YYYY-MM-DD")

	ErrInvalidPipelineStage = errors.New("pipeline or stage is not valid for a new deal")
	ErrInvalidStartStage    = errors.New("a deal cannot start in a won or lost stage")
	ErrInvalidDealInput     = errors.New("deal title is required (max 200), value must be a non-negative number with at most 2 decimals, expected_close_date must be YYYY-MM-DD")
	ErrInvalidCompanyInput  = errors.New("company mode must be none, existing (with company_id) or new (with name)")
	ErrLeadNotConverted     = repository.ErrLeadNotConverted
	ErrLeadDealExists       = repository.ErrLeadDealExists
)

// LeadOwnerValidator adalah subset repository.MemberRepository yang
// dibutuhkan LeadService.
type LeadOwnerValidator interface {
	IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error)
}

// LeadConvertedHook dipanggil setelah konversi lead tersimpan. Didefinisikan
// di sini supaya modul lain (whatsapp) bisa bereaksi tanpa CRM meng-import
// modul tersebut.
type LeadConvertedHook interface {
	LeadConverted(ctx context.Context, scope coretenant.Scope, leadID, contactID string) error
}

type ConvertCompanyInput struct {
	Mode                           string // "none" | "existing" | "new"
	CompanyID                      string
	Name, Industry, Website, Phone string
}

type ConvertDealInput struct {
	PipelineID, StageID, Title, Value, ExpectedCloseDate string // date "YYYY-MM-DD" atau ""
	Description, DecisionMaker, OwnerUserID              string
}

// ConvertLeadParams controls how a lead is converted into a Contact, an
// optional Company and an optional Deal, all in one transaction.
type ConvertLeadParams struct {
	CreateCompany bool                 // kontrak lama: company baru dari lead.CompanyName
	Company       *ConvertCompanyInput // nil = pakai CreateCompany
	Deal          *ConvertDealInput    // nil = tanpa deal
	OwnerUserID   string
	ConvertedBy   string
}

type LeadService interface {
	Create(context.Context, coretenant.Scope, repository.CreateLeadParams) (domain.Lead, error)
	Get(context.Context, coretenant.Scope, string) (domain.Lead, error)
	List(context.Context, coretenant.Scope, repository.LeadListFilter) ([]domain.Lead, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateLeadParams) (domain.Lead, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	Assign(context.Context, coretenant.Scope, string, string, string) (domain.Lead, error)
	Convert(context.Context, coretenant.Scope, string, ConvertLeadParams) (domain.LeadConversionResult, error)
	CreateDealForLead(ctx context.Context, scope coretenant.Scope, leadID string, input ConvertDealInput, createdBy string) (domain.Lead, domain.Deal, error)
	Disqualify(ctx context.Context, scope coretenant.Scope, id string, params repository.DisqualifyLeadParams) (domain.Lead, error)
	StartPlaybook(ctx context.Context, scope coretenant.Scope, id string, startedBy string) (domain.PlaybookRun, error)
	ListEvents(ctx context.Context, scope coretenant.Scope, id string, page, perPage int) ([]domain.LeadEvent, int64, error)
}
