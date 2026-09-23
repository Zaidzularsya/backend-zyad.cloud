package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrLeadAlreadyConverted = errors.New("lead already converted")
	// ErrLeadOwnerNotMember: owner_user_id harus anggota aktif organization
	// yang sama — mencegah lead di-assign ke (dan nama owner dibocorkan dari)
	// user tenant lain.
	ErrLeadOwnerNotMember   = errors.New("owner_user_id is not an active member of this organization")
	ErrInvalidAnnualRevenue = errors.New("annual_revenue must be a non-negative number with at most 2 decimals")
)

// LeadOwnerValidator adalah subset repository.MemberRepository yang
// dibutuhkan LeadService.
type LeadOwnerValidator interface {
	IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error)
}

// ConvertLeadParams controls how a lead is converted into a Contact and
// optionally a Company. Deal creation lands in Fase 2 once crm_deals exists.
type ConvertLeadParams struct {
	CreateCompany bool
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
}
