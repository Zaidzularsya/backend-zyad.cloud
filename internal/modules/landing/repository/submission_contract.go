package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateSubmissionParams struct {
	LandingPageID  string
	FormID         string
	Reference      string
	Status         domain.SubmissionStatus
	SubmittedData  map[string]any
	SourceURL      string
	Referrer       string
	UTMSource      string
	UTMMedium      string
	UTMCampaign    string
	UTMTerm        string
	UTMContent     string
	IPAddressHash  string
	UserAgent      string
	IdempotencyKey string
}

// UpdateCRMSyncParams records the outcome of syncing a submission to a CRM
// lead. LeadID/Error empty are stored as NULL.
type UpdateCRMSyncParams struct {
	LeadID string
	Status domain.CRMSyncStatus
	Error  string
}

type UpdateSubmissionParams struct {
	Status *domain.SubmissionStatus
}

type CreateSubmissionNoteParams struct {
	SubmissionID string
	Note         string
	CreatedBy    string
}

type SubmissionFilter struct {
	LandingPageID string
	FormID        string
	Status        domain.SubmissionStatus
	Limit         int
	Offset        int
}

type SubmissionRepository interface {
	// Submission Operations
	Create(context.Context, coretenant.Scope, CreateSubmissionParams) (domain.LandingSubmission, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingSubmission, error)
	List(context.Context, coretenant.Scope, SubmissionFilter) ([]domain.LandingSubmission, error)
	// FindByIdempotencyKey returns pgx.ErrNoRows when no live submission has the key.
	FindByIdempotencyKey(context.Context, coretenant.Scope, string) (domain.LandingSubmission, error)
	Update(context.Context, coretenant.Scope, string, UpdateSubmissionParams) (domain.LandingSubmission, error)
	UpdateCRMSync(context.Context, coretenant.Scope, string, UpdateCRMSyncParams) (domain.LandingSubmission, error)
	Delete(context.Context, coretenant.Scope, string) error

	// Submission Note Operations
	CreateNote(context.Context, coretenant.Scope, CreateSubmissionNoteParams) error
}
