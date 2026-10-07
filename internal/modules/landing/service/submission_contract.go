package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// PublicSubmitResult is the minimal outcome a public visitor may see.
type PublicSubmitResult struct {
	Reference      string
	SuccessMessage string
	RedirectURL    string
}

type SubmissionService interface {
	SubmitPublic(ctx context.Context, scope coretenant.Scope, params repository.CreateSubmissionParams, isPlatformOrg bool) (PublicSubmitResult, error)
	SubmitForm(ctx context.Context, scope coretenant.Scope, params repository.CreateSubmissionParams, isPlatformOrg bool) (domain.LandingSubmission, error)
	RetryCRMSync(ctx context.Context, scope coretenant.Scope, id string, isPlatformOrg bool) (domain.LandingSubmission, error)
	GetSubmission(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSubmission, error)
	ListSubmissions(ctx context.Context, scope coretenant.Scope, filter repository.SubmissionFilter) ([]domain.LandingSubmission, error)
	UpdateSubmissionStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.SubmissionStatus) (domain.LandingSubmission, error)
	AddSubmissionNote(ctx context.Context, scope coretenant.Scope, id string, note string, createdBy string) error
	DeleteSubmission(ctx context.Context, scope coretenant.Scope, id string) error
}
