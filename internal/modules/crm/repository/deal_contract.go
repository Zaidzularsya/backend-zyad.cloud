package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type DealListFilter struct {
	Search         string
	PipelineID     string
	StageID        string
	Status         domain.DealStatus
	OwnerUserID    string
	ContactID      string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreateDealParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateDealParams struct {
	PipelineID        string
	StageID           string
	CompanyID         string
	ContactID         string
	Title             string
	Value             string
	Currency          string
	ExpectedCloseDate *time.Time
	OwnerUserID       string
	CreatedBy         string
	Description       string
	DecisionMaker     string
}

type UpdateDealParams struct {
	StageID           *string
	CompanyID         *string
	ContactID         *string
	Title             *string
	Value             *string
	Currency          *string
	ExpectedCloseDate *time.Time
	OwnerUserID       *string
	Description       *string
	DecisionMaker     *string
	UpdatedBy         string
}

// DealRepository is the tenant-owned data contract. Every method requires a
// verified immutable scope and row lookups include both scope and resource ID.
type DealRepository interface {
	Create(context.Context, coretenant.Scope, CreateDealParams) (domain.Deal, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Deal, error)
	List(context.Context, coretenant.Scope, DealListFilter) ([]domain.Deal, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateDealParams) (domain.Deal, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	MoveStage(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
	CloseWon(context.Context, coretenant.Scope, string, string) (domain.Deal, error)
	CloseLost(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
	ApproveDiscount(context.Context, coretenant.Scope, string, string, string) (domain.Deal, error)
	// ListOpenByCompanyAndPipeline: deal berstatus open milik company di pipeline,
	// terlama dulu.
	ListOpenByCompanyAndPipeline(ctx context.Context, scope coretenant.Scope, companyID, pipelineID string) ([]domain.Deal, error)
}
