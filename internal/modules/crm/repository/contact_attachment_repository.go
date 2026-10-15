package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type contactAttachmentRepository struct {
	db *database.Pool
}

func NewContactAttachmentRepository(db *database.Pool) ContactAttachmentRepository {
	return &contactAttachmentRepository{db: db}
}

func (r *contactAttachmentRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const contactAttachmentSelect = `
	SELECT a.id, a.contact_id, a.asset_object_id, o.filename, o.mime_type, o.size_bytes,
		a.created_by, a.created_at
	FROM crm_contact_attachments a
	JOIN asset_objects o
		ON o.id = a.asset_object_id
		AND o.organization_id = a.organization_id
		AND o.deleted_at IS NULL
`

func scanContactAttachment(row pgx.Row) (domain.ContactAttachment, error) {
	var attachment domain.ContactAttachment
	var createdBy *string
	err := row.Scan(
		&attachment.ID, &attachment.ContactID, &attachment.AssetObjectID,
		&attachment.Filename, &attachment.MimeType, &attachment.SizeBytes,
		&createdBy, &attachment.CreatedAt,
	)
	if err != nil {
		return domain.ContactAttachment{}, err
	}
	if createdBy != nil {
		attachment.CreatedBy = *createdBy
	}
	return attachment, nil
}

func (r *contactAttachmentRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateContactAttachmentParams) (domain.ContactAttachment, error) {
	if !scope.IsValid() {
		return domain.ContactAttachment{}, coretenant.ErrInvalidScope
	}

	var attachment domain.ContactAttachment
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO crm_contact_attachments (organization_id, contact_id, asset_object_id, created_by)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, scope.OrganizationID(), params.ContactID, params.AssetObjectID, nullableString(params.CreatedBy)).Scan(&id)
		if err != nil {
			return err
		}

		attachment, err = scanContactAttachment(tx.QueryRow(ctx,
			contactAttachmentSelect+` WHERE a.id = $1 AND a.organization_id = $2`,
			id, scope.OrganizationID(),
		))
		return err
	})
	if err != nil {
		return domain.ContactAttachment{}, err
	}
	return attachment, nil
}

func (r *contactAttachmentRepository) ListByContact(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.ContactAttachment, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	attachments := []domain.ContactAttachment{}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			contactAttachmentSelect+` WHERE a.contact_id = $1 AND a.organization_id = $2 ORDER BY a.created_at DESC`,
			contactID, scope.OrganizationID(),
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			attachment, scanErr := scanContactAttachment(rows)
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

func (r *contactAttachmentRepository) FindByContact(ctx context.Context, scope coretenant.Scope, contactID string, id string) (domain.ContactAttachment, error) {
	if !scope.IsValid() {
		return domain.ContactAttachment{}, coretenant.ErrInvalidScope
	}

	var attachment domain.ContactAttachment
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		attachment, scanErr = scanContactAttachment(tx.QueryRow(ctx,
			contactAttachmentSelect+` WHERE a.id = $1 AND a.contact_id = $2 AND a.organization_id = $3`,
			id, contactID, scope.OrganizationID(),
		))
		return scanErr
	})
	if err != nil {
		return domain.ContactAttachment{}, err
	}
	return attachment, nil
}

func (r *contactAttachmentRepository) Delete(ctx context.Context, scope coretenant.Scope, contactID string, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx,
			`DELETE FROM crm_contact_attachments WHERE id = $1 AND contact_id = $2 AND organization_id = $3`,
			id, contactID, scope.OrganizationID(),
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
