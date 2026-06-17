package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type submissionRepository struct {
	db *database.Pool
}

func NewSubmissionRepository(db *database.Pool) SubmissionRepository {
	return &submissionRepository{db: db}
}

// withTx executes a function within a tenant-scoped transaction
func (r *submissionRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *submissionRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateSubmissionParams) (domain.LandingSubmission, error) {
	if !scope.IsValid() {
		return domain.LandingSubmission{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_submissions (
			organization_id, landing_page_id, form_id, reference, status,
			submitted_data, source_url, referrer, utm_source, utm_medium,
			utm_campaign, utm_term, utm_content, ip_address_hash, user_agent,
			idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16
		) RETURNING
			id, landing_page_id, form_id, reference, status,
			submitted_data, source_url, referrer, utm_source, utm_medium,
			utm_campaign, utm_term, utm_content, ip_address_hash, user_agent,
			idempotency_key, submitted_at, updated_at
	`

	var submission domain.LandingSubmission

	submittedData := params.SubmittedData
	if submittedData == nil {
		submittedData = map[string]any{}
	}

	var idempotencyKeyInput interface{} = nil
	if params.IdempotencyKey != "" {
		idempotencyKeyInput = params.IdempotencyKey
	}

	var sourceURL, referrer, utmSource, utmMedium, utmCampaign, utmTerm, utmContent, ipAddressHash, userAgent, idempotencyKey *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.FormID,
			params.Reference,
			params.Status,
			submittedData,
			params.SourceURL,
			params.Referrer,
			params.UTMSource,
			params.UTMMedium,
			params.UTMCampaign,
			params.UTMTerm,
			params.UTMContent,
			params.IPAddressHash,
			params.UserAgent,
			idempotencyKeyInput,
		).Scan(
			&submission.ID, &submission.LandingPageID, &submission.FormID, &submission.Reference, &submission.Status,
			&submission.SubmittedData, &sourceURL, &referrer, &utmSource, &utmMedium,
			&utmCampaign, &utmTerm, &utmContent, &ipAddressHash, &userAgent,
			&idempotencyKey, &submission.SubmittedAt, &submission.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSubmission{}, err
	}
	submission.OrganizationID = scope.OrganizationID()
	if sourceURL != nil { submission.SourceURL = *sourceURL }
	if referrer != nil { submission.Referrer = *referrer }
	if utmSource != nil { submission.UTMSource = *utmSource }
	if utmMedium != nil { submission.UTMMedium = *utmMedium }
	if utmCampaign != nil { submission.UTMCampaign = *utmCampaign }
	if utmTerm != nil { submission.UTMTerm = *utmTerm }
	if utmContent != nil { submission.UTMContent = *utmContent }
	if ipAddressHash != nil { submission.IPAddressHash = *ipAddressHash }
	if userAgent != nil { submission.UserAgent = *userAgent }
	if idempotencyKey != nil { submission.IdempotencyKey = *idempotencyKey }

	return submission, nil
}

func (r *submissionRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSubmission, error) {
	if !scope.IsValid() {
		return domain.LandingSubmission{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, form_id, reference, status,
			submitted_data, source_url, referrer, utm_source, utm_medium,
			utm_campaign, utm_term, utm_content, ip_address_hash, user_agent,
			idempotency_key, submitted_at, updated_at
		FROM landing_submissions
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var submission domain.LandingSubmission
	var sourceURL, referrer, utmSource, utmMedium, utmCampaign, utmTerm, utmContent, ipAddressHash, userAgent, idempotencyKey *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&submission.ID, &submission.LandingPageID, &submission.FormID, &submission.Reference, &submission.Status,
			&submission.SubmittedData, &sourceURL, &referrer, &utmSource, &utmMedium,
			&utmCampaign, &utmTerm, &utmContent, &ipAddressHash, &userAgent,
			&idempotencyKey, &submission.SubmittedAt, &submission.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSubmission{}, err
	}
	submission.OrganizationID = scope.OrganizationID()
	if sourceURL != nil { submission.SourceURL = *sourceURL }
	if referrer != nil { submission.Referrer = *referrer }
	if utmSource != nil { submission.UTMSource = *utmSource }
	if utmMedium != nil { submission.UTMMedium = *utmMedium }
	if utmCampaign != nil { submission.UTMCampaign = *utmCampaign }
	if utmTerm != nil { submission.UTMTerm = *utmTerm }
	if utmContent != nil { submission.UTMContent = *utmContent }
	if ipAddressHash != nil { submission.IPAddressHash = *ipAddressHash }
	if userAgent != nil { submission.UserAgent = *userAgent }
	if idempotencyKey != nil { submission.IdempotencyKey = *idempotencyKey }

	return submission, nil
}

func (r *submissionRepository) List(ctx context.Context, scope coretenant.Scope, filter SubmissionFilter) ([]domain.LandingSubmission, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, form_id, reference, status,
			submitted_data, source_url, referrer, utm_source, utm_medium,
			utm_campaign, utm_term, utm_content, ip_address_hash, user_agent,
			idempotency_key, submitted_at, updated_at
		FROM landing_submissions
		WHERE organization_id = $1 AND deleted_at IS NULL
	`
	var args []interface{}
	args = append(args, scope.OrganizationID())
	argCount := 2

	if filter.LandingPageID != "" {
		query += fmt.Sprintf(" AND landing_page_id = $%d", argCount)
		args = append(args, filter.LandingPageID)
		argCount++
	}
	if filter.FormID != "" {
		query += fmt.Sprintf(" AND form_id = $%d", argCount)
		args = append(args, filter.FormID)
		argCount++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, filter.Status)
		argCount++
	}

	query += ` ORDER BY submitted_at DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
		argCount++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, filter.Offset)
		argCount++
	}

	var submissions []domain.LandingSubmission

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var submission domain.LandingSubmission
			var sourceURL, referrer, utmSource, utmMedium, utmCampaign, utmTerm, utmContent, ipAddressHash, userAgent, idempotencyKey *string
			err := rows.Scan(
				&submission.ID, &submission.LandingPageID, &submission.FormID, &submission.Reference, &submission.Status,
				&submission.SubmittedData, &sourceURL, &referrer, &utmSource, &utmMedium,
				&utmCampaign, &utmTerm, &utmContent, &ipAddressHash, &userAgent,
				&idempotencyKey, &submission.SubmittedAt, &submission.UpdatedAt,
			)
			if err != nil {
				return err
			}
			submission.OrganizationID = scope.OrganizationID()
			if sourceURL != nil { submission.SourceURL = *sourceURL }
			if referrer != nil { submission.Referrer = *referrer }
			if utmSource != nil { submission.UTMSource = *utmSource }
			if utmMedium != nil { submission.UTMMedium = *utmMedium }
			if utmCampaign != nil { submission.UTMCampaign = *utmCampaign }
			if utmTerm != nil { submission.UTMTerm = *utmTerm }
			if utmContent != nil { submission.UTMContent = *utmContent }
			if ipAddressHash != nil { submission.IPAddressHash = *ipAddressHash }
			if userAgent != nil { submission.UserAgent = *userAgent }
			if idempotencyKey != nil { submission.IdempotencyKey = *idempotencyKey }
			submissions = append(submissions, submission)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return submissions, nil
}

func (r *submissionRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateSubmissionParams) (domain.LandingSubmission, error) {
	if !scope.IsValid() {
		return domain.LandingSubmission{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_submissions SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Status != nil {
		args = append(args, *params.Status)
		query += fmt.Sprintf(", status = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `
		id, landing_page_id, form_id, reference, status,
		submitted_data, source_url, referrer, utm_source, utm_medium,
		utm_campaign, utm_term, utm_content, ip_address_hash, user_agent,
		idempotency_key, submitted_at, updated_at
	`

	var submission domain.LandingSubmission
	var sourceURL, referrer, utmSource, utmMedium, utmCampaign, utmTerm, utmContent, ipAddressHash, userAgent, idempotencyKey *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&submission.ID, &submission.LandingPageID, &submission.FormID, &submission.Reference, &submission.Status,
			&submission.SubmittedData, &sourceURL, &referrer, &utmSource, &utmMedium,
			&utmCampaign, &utmTerm, &utmContent, &ipAddressHash, &userAgent,
			&idempotencyKey, &submission.SubmittedAt, &submission.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSubmission{}, err
	}
	submission.OrganizationID = scope.OrganizationID()
	if sourceURL != nil { submission.SourceURL = *sourceURL }
	if referrer != nil { submission.Referrer = *referrer }
	if utmSource != nil { submission.UTMSource = *utmSource }
	if utmMedium != nil { submission.UTMMedium = *utmMedium }
	if utmCampaign != nil { submission.UTMCampaign = *utmCampaign }
	if utmTerm != nil { submission.UTMTerm = *utmTerm }
	if utmContent != nil { submission.UTMContent = *utmContent }
	if ipAddressHash != nil { submission.IPAddressHash = *ipAddressHash }
	if userAgent != nil { submission.UserAgent = *userAgent }
	if idempotencyKey != nil { submission.IdempotencyKey = *idempotencyKey }

	return submission, nil
}

func (r *submissionRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_submissions
		SET deleted_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *submissionRepository) CreateNote(ctx context.Context, scope coretenant.Scope, params CreateSubmissionNoteParams) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_submission_notes (
			organization_id, submission_id, note, created_by
		) VALUES (
			$1, $2, $3, $4
		)
	`
	
	var createdBy interface{} = nil
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			scope.OrganizationID(),
			params.SubmissionID,
			params.Note,
			createdBy,
		)
		return err
	})
}
