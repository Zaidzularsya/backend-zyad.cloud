package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type PipelineListFilter struct {
	IncludeArchived bool
	IncludeDeleted  bool
	Limit           int
	Offset          int
}

// StageInput describes one pipeline stage in a create/replace request. ID is
// nil for a new stage; when set, it must reference an existing stage owned
// by the same pipeline/organization (used by ReplaceStages to update rather
// than recreate it).
type StageInput struct {
	ID          *string
	Name        string
	Position    int
	Probability string
	IsWon       bool
	IsLost      bool
}

// CreatePipelineParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreatePipelineParams struct {
	Name      string
	IsDefault bool
	Stages    []StageInput
	CreatedBy string
}

type UpdatePipelineParams struct {
	Name      *string
	IsDefault *bool
	UpdatedBy string
}

// PipelineRepository is the tenant-owned data contract. Every method requires
// a verified immutable scope and row lookups include both scope and resource ID.
type PipelineRepository interface {
	Create(context.Context, coretenant.Scope, CreatePipelineParams) (domain.Pipeline, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Pipeline, error)
	List(context.Context, coretenant.Scope, PipelineListFilter) ([]domain.Pipeline, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdatePipelineParams) (domain.Pipeline, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	Archive(context.Context, coretenant.Scope, string, string) (domain.Pipeline, error)
	ReplaceStages(context.Context, coretenant.Scope, string, []StageInput, string) (domain.Pipeline, error)
	// CountActive reports how many non-archived, non-deleted pipelines an
	// organization has — used by PipelineService to allow the first pipeline
	// for free (RequireQuota only applies to additional ones).
	CountActive(context.Context, coretenant.Scope) (int64, error)
}
