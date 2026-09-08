package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type PipelineService interface {
	Create(context.Context, coretenant.Scope, repository.CreatePipelineParams) (domain.Pipeline, error)
	Get(context.Context, coretenant.Scope, string) (domain.Pipeline, error)
	List(context.Context, coretenant.Scope, repository.PipelineListFilter) ([]domain.Pipeline, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdatePipelineParams) (domain.Pipeline, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	Archive(context.Context, coretenant.Scope, string, string) (domain.Pipeline, error)
	ReplaceStages(context.Context, coretenant.Scope, string, []repository.StageInput, string) (domain.Pipeline, error)
}
