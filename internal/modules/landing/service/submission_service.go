package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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
	pageRepo       repository.PageRepository
	sink           LeadSink
	features       FeatureChecker
}

// NewSubmissionService builds the service. sink and features may be nil, in
// which case submissions are stored but never synced to CRM (status skipped).
func NewSubmissionService(
	submissionRepo repository.SubmissionRepository,
	formRepo repository.FormRepository,
	pageRepo repository.PageRepository,
	sink LeadSink,
	features FeatureChecker,
) SubmissionService {
	return &submissionService{
		submissionRepo: submissionRepo,
		formRepo:       formRepo,
		pageRepo:       pageRepo,
		sink:           sink,
		features:       features,
	}
}

func (s *submissionService) SubmitForm(ctx context.Context, scope coretenant.Scope, params repository.CreateSubmissionParams, isPlatformOrg bool) (domain.LandingSubmission, error) {
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
	clientKey := params.IdempotencyKey != ""
	if !clientKey {
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

	// 5. A repeated client key (double click, retry on a slow connection) is the
	// same submission: return it as-is. It is deliberately not synced again, so
	// two racing requests can never create two leads; a failed sync is retried
	// through RetryCRMSync.
	if clientKey {
		if existing, err := s.submissionRepo.FindByIdempotencyKey(ctx, scope, params.IdempotencyKey); err == nil {
			return existing, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return domain.LandingSubmission{}, err
		}
	}

	// 6. Save Submission
	sub, err := s.submissionRepo.Create(ctx, scope, params)
	if err != nil {
		if clientKey && isUniqueViolation(err) {
			// Lost the race against a concurrent request with the same key.
			if existing, findErr := s.submissionRepo.FindByIdempotencyKey(ctx, scope, params.IdempotencyKey); findErr == nil {
				return existing, nil
			}
		}
		return domain.LandingSubmission{}, err
	}

	// 7. CRM sync never fails the visitor: the outcome is stored on the row.
	return s.syncToCRM(ctx, scope, form, sub, isPlatformOrg), nil
}

// RetryCRMSync re-runs the CRM sync for a stored submission (for example after
// the form's PIC was fixed). It is a no-op once a lead is linked.
func (s *submissionService) RetryCRMSync(ctx context.Context, scope coretenant.Scope, id string, isPlatformOrg bool) (domain.LandingSubmission, error) {
	sub, err := s.submissionRepo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.LandingSubmission{}, err
	}
	if sub.CRMLeadID != "" {
		return sub, nil
	}
	form, err := s.formRepo.FindByID(ctx, scope, sub.FormID)
	if err != nil {
		return domain.LandingSubmission{}, err
	}
	return s.syncToCRM(ctx, scope, form, sub, isPlatformOrg), nil
}

// syncToCRM applies the sync rules and records the outcome. It returns the
// submission as stored afterwards; errors are logged, never returned.
func (s *submissionService) syncToCRM(ctx context.Context, scope coretenant.Scope, form domain.LandingForm, sub domain.LandingSubmission, isPlatformOrg bool) domain.LandingSubmission {
	if s.sink == nil || !form.CreateCRMLead {
		return sub
	}
	if !isPlatformOrg {
		// A missing checker must not grant the feature (fail closed).
		if s.features == nil || s.features.RequireFeature(ctx, scope.OrganizationID(), FeatureCRMLeadForm) != nil {
			return sub
		}
	}

	record := func(p repository.UpdateCRMSyncParams) domain.LandingSubmission {
		updated, err := s.submissionRepo.UpdateCRMSync(ctx, scope, sub.ID, p)
		if err != nil {
			slog.Warn("landing: failed to record CRM sync result", "submission_id", sub.ID, "error", err)
			return sub
		}
		return updated
	}
	fail := func(message string) domain.LandingSubmission {
		slog.Warn("landing: CRM sync failed", "submission_id", sub.ID, "error", message)
		return record(repository.UpdateCRMSyncParams{Status: domain.CRMSyncFailed, Error: truncateSyncError(message)})
	}

	// Notes need the field labels, which FindByID does not load.
	if fields, err := s.formRepo.ListFieldsByForm(ctx, scope, form.ID); err == nil {
		form.Fields = fields
	}

	// The page is only needed for the owner fallback.
	var page domain.LandingPage
	if strings.TrimSpace(form.LeadOwnerUserID) == "" {
		p, err := s.pageRepo.FindByID(ctx, scope, sub.LandingPageID)
		if err != nil {
			return fail("landing page lookup failed: " + err.Error())
		}
		page = p
	}

	input, err := buildLeadSinkInput(form, page, sub)
	if err != nil {
		return fail(err.Error())
	}

	result, err := s.sink.CreateFromSubmission(ctx, scope, input)
	if err != nil {
		return fail(err.Error())
	}

	status := domain.CRMSyncCreated
	if result.Merged {
		status = domain.CRMSyncMerged
	}
	return record(repository.UpdateCRMSyncParams{LeadID: result.LeadID, Status: status})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func truncateSyncError(message string) string {
	const max = 500
	if r := []rune(message); len(r) > max {
		return string(r[:max])
	}
	return message
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
