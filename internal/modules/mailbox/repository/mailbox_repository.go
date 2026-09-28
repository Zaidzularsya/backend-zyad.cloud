package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/platform/database"
)

type mailboxRepository struct {
	db *database.Pool
}

func NewMailboxRepository(db *database.Pool) MailboxRepository {
	return &mailboxRepository{db: db}
}

const mailboxColumns = `
	id, user_id, email_address, COALESCE(display_name, ''), username, secret_encrypted,
	smtp_host, smtp_port, smtp_security,
	COALESCE(imap_host, ''), COALESCE(imap_port, 0), COALESCE(imap_security, ''),
	status, COALESCE(last_error, ''), created_at, updated_at
`

func scanMailbox(row pgx.Row) (domain.Mailbox, error) {
	var m domain.Mailbox
	var smtpSecurity, imapSecurity, status string
	err := row.Scan(
		&m.ID, &m.UserID, &m.EmailAddress, &m.DisplayName, &m.Username, &m.SecretEncrypted,
		&m.SMTPHost, &m.SMTPPort, &smtpSecurity,
		&m.IMAPHost, &m.IMAPPort, &imapSecurity,
		&status, &m.LastError, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return domain.Mailbox{}, err
	}
	m.SMTPSecurity = domain.Security(smtpSecurity)
	m.IMAPSecurity = domain.Security(imapSecurity)
	m.Status = domain.MailboxStatus(status)
	return m, nil
}

func (r *mailboxRepository) ListByUser(ctx context.Context, scope coretenant.Scope, userID string) ([]domain.Mailbox, error) {
	mailboxes := []domain.Mailbox{}
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+mailboxColumns+` FROM user_mailboxes
			WHERE organization_id = $1 AND user_id = $2
			ORDER BY created_at
		`, scope.OrganizationID(), userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			mailbox, err := scanMailbox(rows)
			if err != nil {
				return err
			}
			mailboxes = append(mailboxes, mailbox)
		}
		return rows.Err()
	})
	return mailboxes, err
}

func (r *mailboxRepository) GetForUser(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	var mailbox domain.Mailbox
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		mailbox, err = scanMailbox(tx.QueryRow(ctx, `
			SELECT `+mailboxColumns+` FROM user_mailboxes
			WHERE organization_id = $1 AND user_id = $2 AND id = $3
		`, scope.OrganizationID(), userID, id))
		return err
	})
	return mailbox, err
}

func (r *mailboxRepository) Create(ctx context.Context, scope coretenant.Scope, userID string, p MailboxParams) (domain.Mailbox, error) {
	var mailbox domain.Mailbox
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		mailbox, err = scanMailbox(tx.QueryRow(ctx, `
			INSERT INTO user_mailboxes (
				organization_id, user_id, email_address, display_name, username, secret_encrypted,
				smtp_host, smtp_port, smtp_security, imap_host, imap_port, imap_security
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING `+mailboxColumns,
			scope.OrganizationID(), userID, p.EmailAddress, nullableString(p.DisplayName), p.Username, p.SecretEncrypted,
			p.SMTPHost, p.SMTPPort, string(p.SMTPSecurity),
			nullableString(p.IMAPHost), nullableInt(p.IMAPPort), nullableString(string(p.IMAPSecurity)),
		))
		return err
	})
	if isUniqueViolation(err) {
		return domain.Mailbox{}, ErrDuplicateMailbox
	}
	return mailbox, err
}

// Update replaces every setting and re-activates the mailbox (the service
// verified the new settings first).
func (r *mailboxRepository) Update(ctx context.Context, scope coretenant.Scope, userID, id string, p MailboxParams) (domain.Mailbox, error) {
	var mailbox domain.Mailbox
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		mailbox, err = scanMailbox(tx.QueryRow(ctx, `
			UPDATE user_mailboxes SET
				email_address = $4, display_name = $5, username = $6,
				secret_encrypted = COALESCE($7, secret_encrypted),
				smtp_host = $8, smtp_port = $9, smtp_security = $10,
				imap_host = $11, imap_port = $12, imap_security = $13,
				status = 'active', last_error = NULL, updated_at = now()
			WHERE organization_id = $1 AND user_id = $2 AND id = $3
			RETURNING `+mailboxColumns,
			scope.OrganizationID(), userID, id,
			p.EmailAddress, nullableString(p.DisplayName), p.Username, nullableString(p.SecretEncrypted),
			p.SMTPHost, p.SMTPPort, string(p.SMTPSecurity),
			nullableString(p.IMAPHost), nullableInt(p.IMAPPort), nullableString(string(p.IMAPSecurity)),
		))
		return err
	})
	if isUniqueViolation(err) {
		return domain.Mailbox{}, ErrDuplicateMailbox
	}
	return mailbox, err
}

func (r *mailboxRepository) SetStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.MailboxStatus, lastError string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE user_mailboxes SET status = $3, last_error = $4, updated_at = now()
			WHERE organization_id = $1 AND id = $2
		`, scope.OrganizationID(), id, string(status), nullableString(lastError))
		return err
	})
}

func (r *mailboxRepository) Delete(ctx context.Context, scope coretenant.Scope, userID, id string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM user_mailboxes WHERE organization_id = $1 AND user_id = $2 AND id = $3
		`, scope.OrganizationID(), userID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
