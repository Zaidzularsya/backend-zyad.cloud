package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type leadAttachmentRepository struct {
	db *database.Pool
}

func NewLeadAttachmentRepository(db *database.Pool) LeadAttachmentRepository {
	return &leadAttachmentRepository{db: db}
}

func (r *leadAttachmentRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const leadAttachmentSelect = `
	SELECT a.id, a.lead_id, a.asset_object_id, o.filename, o.mime_type, o.size_bytes,
		a.created_by, a.created_at
	FROM crm_lead_attachments a
	JOIN asset_objects o
		ON o.id = a.asset_object_id
		AND o.organization_id = a.organization_id
		AND o.deleted_at IS NULL
`

func scanLeadAttachment(row pgx.Row) (domain.LeadAttachment, error) {
	var attachment domain.LeadAttachment
	var createdBy *string
	err := row.Scan(
		&attachment.ID, &attachment.LeadID, &attachment.AssetObjectID,
		&attachment.Filename, &attachment.MimeType, &attachment.SizeBytes,
		&createdBy, &attachment.CreatedAt,
	)
	if err != nil {
		return domain.LeadAttachment{}, err
	}
	if createdBy != nil {
		attachment.CreatedBy = *createdBy
	}
	return attachment, nil
}

func (r *leadAttachmentRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateLeadAttachmentParams) (domain.LeadAttachment, error) {
	if !scope.IsValid() {
		return domain.LeadAttachment{}, coretenant.ErrInvalidScope
	}

	var attachment domain.LeadAttachment
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO crm_lead_attachments (organization_id, lead_id, asset_object_id, created_by)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, scope.OrganizationID(), params.LeadID, params.AssetObjectID, nullableString(params.CreatedBy)).Scan(&id)
		if err != nil {
			return err
		}

		attachment, err = scanLeadAttachment(tx.QueryRow(ctx,
			leadAttachmentSelect+` WHERE a.id = $1 AND a.organization_id = $2`,
			id, scope.OrganizationID(),
		))
		return err
	})
	if err != nil {
		return domain.LeadAttachment{}, err
	}
	return attachment, nil
}

func (r *leadAttachmentRepository) ListByLead(ctx context.Context, scope coretenant.Scope, leadID string) ([]domain.LeadAttachment, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	attachments := []domain.LeadAttachment{}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			leadAttachmentSelect+` WHERE a.lead_id = $1 AND a.organization_id = $2 ORDER BY a.created_at DESC`,
			leadID, scope.OrganizationID(),
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			attachment, scanErr := scanLeadAttachment(rows)
			if scanErr != nil {
				return scanErr
			}
			attachments = append(attachments, attachment)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return attachments, nil
}

func (r *leadAttachmentRepository) FindByLead(ctx context.Context, scope coretenant.Scope, leadID string, id string) (domain.LeadAttachment, error) {
	if !scope.IsValid() {
		return domain.LeadAttachment{}, coretenant.ErrInvalidScope
	}

	var attachment domain.LeadAttachment
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		attachment, scanErr = scanLeadAttachment(tx.QueryRow(ctx,
			leadAttachmentSelect+` WHERE a.id = $1 AND a.lead_id = $2 AND a.organization_id = $3`,
			id, leadID, scope.OrganizationID(),
		))
		return scanErr
	})
	if err != nil {
		return domain.LeadAttachment{}, err
	}
	return attachment, nil
}

func (r *leadAttachmentRepository) Delete(ctx context.Context, scope coretenant.Scope, leadID string, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx,
			`DELETE FROM crm_lead_attachments WHERE id = $1 AND lead_id = $2 AND organization_id = $3`,
			id, leadID, scope.OrganizationID(),
		)
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
