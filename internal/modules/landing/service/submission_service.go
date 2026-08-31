package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrFormNotActive = errors.New("form is not active")
	ErrSpamDetected  = errors.New("submission rejected as spam")
)

type submissionService struct {
	submissionRepo repository.SubmissionRepository
	formRepo       repository.FormRepository
}

func NewSubmissionService(submissionRepo repository.SubmissionRepository, formRepo repository.FormRepository) SubmissionService {
	return &submissionService{
		submissionRepo: submissionRepo,
		formRepo:       formRepo,
	}
}

func (s *submissionService) SubmitForm(ctx context.Context, scope coretenant.Scope, params repository.CreateSubmissionParams) (domain.LandingSubmission, error) {
	// 1. Validate Form
	form, err := s.formRepo.FindByID(ctx, scope, params.FormID)
	if err != nil {
		return domain.LandingSubmission{}, err
	}
	if !form.IsActive {
		return domain.LandingSubmission{}, ErrFormNotActive
	}
	params.LandingPageID = form.LandingPageID

	// 2. Generate Idempotency Key (Hash of payload + form_id + time bucket to prevent duplicate within a window)
	// Alternatively, just trust the client's IdempotencyKey if provided.
	if params.IdempotencyKey == "" {
		hashStr := fmt.Sprintf("%s:%v:%v", params.FormID, params.IPAddressHash, time.Now().UnixMilli()/5000) // 5s bucket
		hash := sha256.Sum256([]byte(hashStr))
		params.IdempotencyKey = hex.EncodeToString(hash[:])
	}

	// 3. Spam Detection Logic (Honeypot)
	// If the frontend sends a honeypot field (e.g. "_honey" or "website_url" that should be empty)
	if val, ok := params.SubmittedData["_honey"]; ok && val != "" {
		return domain.LandingSubmission{}, ErrSpamDetected
	}

	// 4. Set Initial Status
	if params.Status == "" {
		params.Status = domain.SubmissionStatusNew
	}

	// 5. Save Submission
	return s.submissionRepo.Create(ctx, scope, params)
}

func (s *submissionService) GetSubmission(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSubmission, error) {
	return s.submissionRepo.FindByID(ctx, scope, id)
}

func (s *submissionService) ListSubmissions(ctx context.Context, scope coretenant.Scope, filter repository.SubmissionFilter) ([]domain.LandingSubmission, error) {
	return s.submissionRepo.List(ctx, scope, filter)
}

func (s *submissionService) UpdateSubmissionStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.SubmissionStatus) (domain.LandingSubmission, error) {
	return s.submissionRepo.Update(ctx, scope, id, repository.UpdateSubmissionParams{
		Status: &status,
	})
}

func (s *submissionService) AddSubmissionNote(ctx context.Context, scope coretenant.Scope, id string, note string, createdBy string) error {
	return s.submissionRepo.CreateNote(ctx, scope, repository.CreateSubmissionNoteParams{
		SubmissionID: id,
		Note:         note,
		CreatedBy:    createdBy,
	})
}

func (s *submissionService) DeleteSubmission(ctx context.Context, scope coretenant.Scope, id string) error {
	return s.submissionRepo.Delete(ctx, scope, id)
}
