package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var ErrLeadAlreadyConverted = errors.New("lead already converted")

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
