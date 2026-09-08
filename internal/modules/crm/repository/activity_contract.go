package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type ActivityListFilter struct {
	RelatedEntityType domain.ActivityEntityType
	RelatedEntityID   string
	AssigneeUserID    string
	Status            domain.ActivityStatus
	IncludeDeleted    bool
	Limit             int
	Offset            int
}

// CreateActivityParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateActivityParams struct {
	RelatedEntityType domain.ActivityEntityType
	RelatedEntityID   string
	Type              domain.ActivityType
	Subject           string
	Description       string
	DueAt             *time.Time
	AssigneeUserID    string
	CreatedBy         string
}

type UpdateActivityParams struct {
	Type           *domain.ActivityType
	Subject        *string
	Description    *string
	DueAt          *time.Time
	AssigneeUserID *string
	UpdatedBy      string
}

// ActivityRepository is the tenant-owned data contract. Every method requires
// a verified immutable scope and row lookups include both scope and resource ID.
type ActivityRepository interface {
	Create(context.Context, coretenant.Scope, CreateActivityParams) (domain.Activity, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Activity, error)
	List(context.Context, coretenant.Scope, ActivityListFilter) ([]domain.Activity, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateActivityParams) (domain.Activity, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Complete(context.Context, coretenant.Scope, string, string) (domain.Activity, error)
	Cancel(context.Context, coretenant.Scope, string, string) (domain.Activity, error)
	Assign(context.Context, coretenant.Scope, string, string, string) (domain.Activity, error)
}
