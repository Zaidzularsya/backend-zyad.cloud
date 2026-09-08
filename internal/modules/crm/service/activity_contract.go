package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrInvalidActivityEntityType = errors.New("invalid related entity type")
	ErrInvalidActivityType       = errors.New("invalid activity type")
)

type ActivityService interface {
	Create(context.Context, coretenant.Scope, repository.CreateActivityParams) (domain.Activity, error)
	Get(context.Context, coretenant.Scope, string) (domain.Activity, error)
	List(context.Context, coretenant.Scope, repository.ActivityListFilter) ([]domain.Activity, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateActivityParams) (domain.Activity, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Complete(context.Context, coretenant.Scope, string, string) (domain.Activity, error)
	Cancel(context.Context, coretenant.Scope, string, string) (domain.Activity, error)
	Assign(context.Context, coretenant.Scope, string, string, string) (domain.Activity, error)
}
