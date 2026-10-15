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

	ErrInvalidActivityStatus        = errors.New("status must be 'pending' or 'completed' when creating an activity")
	ErrDueAtRequired                = errors.New("due_at is required for a pending activity")
	ErrPlaybookStepCancelNotAllowed = errors.New("playbook steps end through an outcome, not cancel")
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
	CompleteWithOutcome(ctx context.Context, scope coretenant.Scope, id string, params repository.CompleteActivityParams) (repository.CompleteActivityResult, error)
}
