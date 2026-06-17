package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type SubmissionService interface {
	SubmitForm(ctx context.Context, scope coretenant.Scope, params repository.CreateSubmissionParams) (domain.LandingSubmission, error)
	GetSubmission(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSubmission, error)
	ListSubmissions(ctx context.Context, scope coretenant.Scope, filter repository.SubmissionFilter) ([]domain.LandingSubmission, error)
	UpdateSubmissionStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.SubmissionStatus) (domain.LandingSubmission, error)
	AddSubmissionNote(ctx context.Context, scope coretenant.Scope, id string, note string, createdBy string) error
	DeleteSubmission(ctx context.Context, scope coretenant.Scope, id string) error
}
