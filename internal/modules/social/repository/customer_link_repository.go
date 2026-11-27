package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database"
)

// Unique indexes guarding live (pending_user/active) links, migration 000189.
const (
	contactLiveUniqueIndex = "idx_customer_links_contact_live_unique"
	userOrgLiveUniqueIndex = "idx_customer_links_user_org_live_unique"
)

// Audit events of the social module (audit_logs.module = 'social').
const (
	auditEventLinkActivated = "customer_link_activated"
	auditEventLinkRevoked   = "customer_link_revoked"
	auditEventLinkDeclined  = "customer_link_declined"
)

const customerLinkColumns = `id::text, user_id::text, organization_id::text, crm_contact_id::text,
	status, verified_via, created_at, activated_at, revoked_at`

// CustomerLinkRepository reads/writes customer_links. The table is global (no RLS,
// ruling R4): reads are keyed by user id or link id and every write states the
// organization explicitly. Each state change writes its audit_logs row in the
// same transaction.
type CustomerLinkRepository struct {
	db *database.Pool
}

func NewCustomerLinkRepository(db *database.Pool) *CustomerLinkRepository {
	return &CustomerLinkRepository{db: db}
}

func (r *CustomerLinkRepository) ListByUser(ctx context.Context, userID string) ([]domain.CustomerLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+customerLinkColumns+`
		FROM customer_links
		WHERE user_id = $1::uuid
		ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list customer links: %w", err)
	}
	links, err := pgx.CollectRows(rows, scanCustomerLink)
	if err != nil {
		return nil, fmt.Errorf("scan customer links: %w", err)
	}
	return links, nil
}

func (r *CustomerLinkRepository) FindByID(ctx context.Context, linkID string) (domain.CustomerLink, bool, error) {
	rows, err := r.db.Query(ctx, `SELECT `+customerLinkColumns+` FROM customer_links WHERE id = $1::uuid`, linkID)
	if err != nil {
		return domain.CustomerLink{}, false, fmt.Errorf("find customer link: %w", err)
	}
	link, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerLink{}, false, nil
	}
	if err != nil {
		return domain.CustomerLink{}, false, fmt.Errorf("scan customer link: %w", err)
	}
	return link, true, nil
}

func (r *CustomerLinkRepository) HasLiveLinkForContact(ctx context.Context, organizationID, contactID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM customer_links
			WHERE organization_id = $1::uuid AND crm_contact_id = $2::uuid
				AND status IN ('pending_user', 'active')
		)`, organizationID, contactID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check live customer link: %w", err)
	}
	return exists, nil
}

// CreateActive inserts an active link. The partial unique indexes decide races:
// the loser gets domain.ErrCustomerLinkExists.
func (r *CustomerLinkRepository) CreateActive(ctx context.Context, link domain.CustomerLink) (domain.CustomerLink, error) {
	return r.inTx(ctx, func(tx pgx.Tx) (domain.CustomerLink, error) {
		rows, err := tx.Query(ctx, `
			INSERT INTO customer_links (user_id, organization_id, crm_contact_id, status, verified_via, activated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'active', $4, now())
			RETURNING `+customerLinkColumns,
			link.UserID, link.OrganizationID, link.ContactID, link.VerifiedVia)
		if isLiveLinkConflict(err) {
			return domain.CustomerLink{}, domain.ErrCustomerLinkExists
		}
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("insert customer link: %w", err)
		}
		created, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
		if isLiveLinkConflict(err) {
			return domain.CustomerLink{}, domain.ErrCustomerLinkExists
		}
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("insert customer link: %w", err)
		}
		return created, insertLinkAudit(ctx, tx, auditEventLinkActivated, created.UserID, created, nil)
	})
}

func (r *CustomerLinkRepository) ActivatePending(ctx context.Context, linkID, userID string) (domain.CustomerLink, error) {
	return r.inTx(ctx, func(tx pgx.Tx) (domain.CustomerLink, error) {
		rows, err := tx.Query(ctx, `
			UPDATE customer_links
			SET status = 'active', verified_via = 'staff', activated_at = now()
			WHERE id = $1::uuid AND user_id = $2::uuid AND status = 'pending_user'
			RETURNING `+customerLinkColumns, linkID, userID)
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("activate customer link: %w", err)
		}
		link, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CustomerLink{}, domain.ErrClaimNotAvailable
		}
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("activate customer link: %w", err)
		}
		return link, insertLinkAudit(ctx, tx, auditEventLinkActivated, userID, link, nil)
	})
}

func (r *CustomerLinkRepository) DeclinePending(ctx context.Context, linkID, userID string) error {
	_, err := r.inTx(ctx, func(tx pgx.Tx) (domain.CustomerLink, error) {
		rows, err := tx.Query(ctx, `
			UPDATE customer_links SET status = 'declined'
			WHERE id = $1::uuid AND user_id = $2::uuid AND status = 'pending_user'
			RETURNING `+customerLinkColumns, linkID, userID)
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("decline customer link: %w", err)
		}
		link, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CustomerLink{}, domain.ErrClaimNotAvailable
		}
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("decline customer link: %w", err)
		}
		return link, insertLinkAudit(ctx, tx, auditEventLinkDeclined, userID, link, nil)
	})
	return err
}

// InsertDeclined records a declined email-match offer. Declined rows are not
// "live", so they never collide with the unique indexes.
func (r *CustomerLinkRepository) InsertDeclined(ctx context.Context, userID, organizationID, contactID string) error {
	_, err := r.inTx(ctx, func(tx pgx.Tx) (domain.CustomerLink, error) {
		rows, err := tx.Query(ctx, `
			INSERT INTO customer_links (user_id, organization_id, crm_contact_id, status, verified_via)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'declined', 'email')
			RETURNING `+customerLinkColumns, userID, organizationID, contactID)
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("insert declined customer link: %w", err)
		}
		link, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("insert declined customer link: %w", err)
		}
		return link, insertLinkAudit(ctx, tx, auditEventLinkDeclined, userID, link, nil)
	})
	return err
}

// Revoke ends the link only while it is still live, so a concurrent revoke
// reports domain.ErrCustomerLinkNotFound instead of double-auditing.
func (r *CustomerLinkRepository) Revoke(ctx context.Context, link domain.CustomerLink, actorUserID string, byStaff bool) error {
	_, err := r.inTx(ctx, func(tx pgx.Tx) (domain.CustomerLink, error) {
		rows, err := tx.Query(ctx, `
			UPDATE customer_links
			SET status = 'revoked', revoked_at = now(), revoked_by = NULLIF($3, '')::uuid
			WHERE id = $1::uuid AND organization_id = $2::uuid AND status IN ('pending_user', 'active')
			RETURNING `+customerLinkColumns, link.ID, link.OrganizationID, actorUserID)
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("revoke customer link: %w", err)
		}
		revoked, err := pgx.CollectExactlyOneRow(rows, scanCustomerLink)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CustomerLink{}, domain.ErrCustomerLinkNotFound
		}
		if err != nil {
			return domain.CustomerLink{}, fmt.Errorf("revoke customer link: %w", err)
		}
		return revoked, insertLinkAudit(ctx, tx, auditEventLinkRevoked, actorUserID, revoked,
			map[string]any{"by_staff": byStaff, "previous_status": link.Status})
	})
	return err
}

func (r *CustomerLinkRepository) inTx(ctx context.Context, fn func(pgx.Tx) (domain.CustomerLink, error)) (domain.CustomerLink, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.CustomerLink{}, err
	}
	defer tx.Rollback(context.Background())
	link, err := fn(tx)
	if err != nil {
		return domain.CustomerLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CustomerLink{}, fmt.Errorf("commit customer link change: %w", err)
	}
	return link, nil
}

// insertLinkAudit follows the audit_logs column pattern of the user module;
// organization_id is the tenant that owns the CRM contact.
func insertLinkAudit(ctx context.Context, tx pgx.Tx, event, actorUserID string, link domain.CustomerLink, extra map[string]any) error {
	meta := map[string]any{
		"crm_contact_id": link.ContactID,
		"status":         link.Status,
		"verified_via":   link.VerifiedVia,
	}
	for k, v := range extra {
		meta[k] = v
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, actor_user_id, target_user_id, target_type, target_id, organization_id, metadata, created_at
		)
		VALUES ('social', $1, NULLIF($2, '')::uuid, $3::uuid, 'customer_link', $4::uuid, $5::uuid, $6, now())`,
		event, actorUserID, link.UserID, link.ID, link.OrganizationID, metadata); err != nil {
		return fmt.Errorf("insert customer link audit: %w", err)
	}
	return nil
}

func scanCustomerLink(row pgx.CollectableRow) (domain.CustomerLink, error) {
	var l domain.CustomerLink
	err := row.Scan(&l.ID, &l.UserID, &l.OrganizationID, &l.ContactID, &l.Status, &l.VerifiedVia,
		&l.CreatedAt, &l.ActivatedAt, &l.RevokedAt)
	return l, err
}

func isLiveLinkConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(pgErr.ConstraintName == contactLiveUniqueIndex || pgErr.ConstraintName == userOrgLiveUniqueIndex)
}
