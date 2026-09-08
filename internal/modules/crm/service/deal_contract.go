package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrDealStageNotInPipeline = errors.New("stage does not belong to the deal's pipeline")
	ErrDealNotOpen            = errors.New("deal is not open")
)

type DealService interface {
	Create(context.Context, coretenant.Scope, repository.CreateDealParams) (domain.Deal, error)
	Get(context.Context, coretenant.Scope, string) (domain.Deal, error)
	List(context.Context, coretenant.Scope, repository.DealListFilter) ([]domain.Deal, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateDealParams) (domain.Deal, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	MoveStage(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
	CloseWon(context.Context, coretenant.Scope, string, string) (domain.Deal, error)
	CloseLost(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
	ApproveDiscount(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
}
