package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

type StartImpersonationParams struct {
	OperatorSessionID    string
	OperatorUserID       string
	TargetOrganizationID string
	TargetUserID         string
	Reason               string
	TicketReference      string
	ExpiresAt            time.Time
	Metadata             map[string]any
	RequestID            string
	IPAddress            string
	UserAgent            string
	StartedAt            time.Time
}

type StopImpersonationParams struct {
	OperatorSessionID string
	OperatorUserID    string
	Reason            string
	RequestID         string
	IPAddress         string
	UserAgent         string
	StoppedAt         time.Time
}

type ImpersonationRepository struct {
	db *database.Pool
}

func NewImpersonationRepository(db *database.Pool) *ImpersonationRepository {
	return &ImpersonationRepository{db: db}
}

func (r *ImpersonationRepository) Start(
	ctx context.Context,
	params StartImpersonationParams,
) (model.ImpersonationSession, error) {
	metadataBytes, err := json.Marshal(nonNilMetadata(params.Metadata))
	if err != nil {
		return model.ImpersonationSession{}, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.ImpersonationSession{}, err
	}
	defer tx.Rollback(ctx)

	var session model.ImpersonationSession
	var storedMetadata []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO organization_impersonation_sessions (
			operator_session_id,
			operator_user_id,
			target_organization_id,
			target_user_id,
			reason,
			ticket_reference,
			started_at,
			expires_at,
			metadata
		)
		SELECT
			$1::uuid,
			$2::uuid,
			organization.id,
			NULLIF($4, '')::uuid,
			$5,
			NULLIF($6, ''),
			$7,
			$8,
			$9::jsonb
		FROM organizations organization
		WHERE organization.id = $3::uuid
			AND organization.type = $10
			AND organization.status = $11
			AND organization.deleted_at IS NULL
		RETURNING `+impersonationSelectColumns,
		strings.TrimSpace(params.OperatorSessionID),
		strings.TrimSpace(params.OperatorUserID),
		strings.TrimSpace(params.TargetOrganizationID),
		strings.TrimSpace(params.TargetUserID),
		strings.TrimSpace(params.Reason),
		strings.TrimSpace(params.TicketReference),
		params.StartedAt,
		params.ExpiresAt,
		string(metadataBytes),
		string(coretenant.OrganizationTypeCustomer),
		string(coretenant.OrganizationStatusActive),
	).Scan(impersonationScanDest(&session, &storedMetadata)...)
	if err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := decodeMetadata(storedMetadata, &session.Metadata); err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := insertImpersonationAudit(
		ctx,
		tx,
		session,
		"impersonation_started",
		params.RequestID,
		params.IPAddress,
		params.UserAgent,
		params.StartedAt,
		map[string]any{
			"reason":           session.Reason,
			"ticket_reference": session.TicketReference,
			"expires_at":       session.ExpiresAt.UTC().Format(time.RFC3339),
		},
	); err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ImpersonationSession{}, err
	}
	return session, nil
}

func (r *ImpersonationRepository) Stop(
	ctx context.Context,
	params StopImpersonationParams,
) (model.ImpersonationSession, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.ImpersonationSession{}, err
	}
	defer tx.Rollback(ctx)

	var session model.ImpersonationSession
	var storedMetadata []byte
	err = tx.QueryRow(ctx, `
		UPDATE organization_impersonation_sessions
		SET
			stopped_at = $3,
			stopped_by_user_id = $2::uuid,
			stop_reason = $4,
			updated_at = now()
		WHERE operator_session_id = $1::uuid
			AND operator_user_id = $2::uuid
			AND stopped_at IS NULL
		RETURNING `+impersonationSelectColumns,
		strings.TrimSpace(params.OperatorSessionID),
		strings.TrimSpace(params.OperatorUserID),
		params.StoppedAt,
		strings.TrimSpace(params.Reason),
	).Scan(impersonationScanDest(&session, &storedMetadata)...)
	if err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := decodeMetadata(storedMetadata, &session.Metadata); err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := insertImpersonationAudit(
		ctx,
		tx,
		session,
		"impersonation_stopped",
		params.RequestID,
		params.IPAddress,
		params.UserAgent,
		params.StoppedAt,
		map[string]any{"reason": session.StopReason},
	); err != nil {
		return model.ImpersonationSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ImpersonationSession{}, err
	}
	return session, nil
}

const impersonationSelectColumns = `
	id,
	operator_session_id,
	operator_user_id,
	target_organization_id,
	COALESCE(target_user_id::text, ''),
	reason,
	COALESCE(ticket_reference, ''),
	started_at,
	expires_at,
	stopped_at,
	COALESCE(stopped_by_user_id::text, ''),
	COALESCE(stop_reason, ''),
	metadata,
	created_at,
	updated_at
`

func impersonationScanDest(
	session *model.ImpersonationSession,
	metadata *[]byte,
) []any {
	return []any{
		&session.ID,
		&session.OperatorSessionID,
		&session.OperatorUserID,
		&session.TargetOrganizationID,
		&session.TargetUserID,
		&session.Reason,
		&session.TicketReference,
		&session.StartedAt,
		&session.ExpiresAt,
		&session.StoppedAt,
		&session.StoppedByUserID,
		&session.StopReason,
		metadata,
		&session.CreatedAt,
		&session.UpdatedAt,
	}
}

func insertImpersonationAudit(
	ctx context.Context,
	tx pgx.Tx,
	session model.ImpersonationSession,
	event string,
	requestID string,
	ipAddress string,
	userAgent string,
	createdAt time.Time,
	metadata map[string]any,
) error {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module,
			event,
			organization_id,
			session_id,
			actor_user_id,
			operator_user_id,
			effective_user_id,
			impersonation_session_id,
			target_user_id,
			target_type,
			target_id,
			metadata,
			ip_address,
			user_agent,
			resolution_source,
			request_id,
			created_at
		)
		VALUES (
			'organization',
			$1,
			$2::uuid,
			$3::uuid,
			$4::uuid,
			$4::uuid,
			NULLIF($5, '')::uuid,
			$6::uuid,
			NULLIF($5, '')::uuid,
			'organization_impersonation_session',
			$6::uuid,
			$7::jsonb,
			NULLIF($8, '')::inet,
			NULLIF($9, ''),
			'internal',
			NULLIF($10, ''),
			$11
		)
	`, event, session.TargetOrganizationID, session.OperatorSessionID,
		session.OperatorUserID, session.TargetUserID, session.ID,
		string(metadataBytes), strings.TrimSpace(ipAddress),
		strings.TrimSpace(userAgent), strings.TrimSpace(requestID), createdAt)
	return err
}

func nonNilMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}
