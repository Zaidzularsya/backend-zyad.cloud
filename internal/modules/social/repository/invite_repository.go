package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database"
)

const auditEventInviteSent = "customer_claim_invite_sent"

const claimInviteColumns = `id::text, organization_id::text, crm_contact_id::text, email, token_hash,
	expires_at, used_at, COALESCE(created_by::text, ''), created_at`

// InviteRepository reads/writes customer_claim_invites. The table is global (no
// RLS, ruling R4): the staff side always states the organization explicitly; the
// customer side is keyed by the token hash only.
type InviteRepository struct {
	db *database.Pool
}

func NewInviteRepository(db *database.Pool) *InviteRepository {
	return &InviteRepository{db: db}
}

// Create stores a new invite and marks every still-open invite of the same
// contact used (superseded), so a contact has at most one usable invite. An
// advisory lock per (organization, contact) serializes concurrent sends.
func (r *InviteRepository) Create(ctx context.Context, invite domain.ClaimInvite) (domain.ClaimInvite, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.ClaimInvite{}, err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('social.claim_invite:' || $1 || ':' || $2, 0))`,
		invite.OrganizationID, invite.ContactID); err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("lock claim invites of contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE customer_claim_invites SET used_at = $3
		WHERE organization_id = $1::uuid AND crm_contact_id = $2::uuid AND used_at IS NULL`,
		invite.OrganizationID, invite.ContactID, invite.CreatedAt); err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("supersede claim invites: %w", err)
	}
	rows, err := tx.Query(ctx, `
		INSERT INTO customer_claim_invites (organization_id, crm_contact_id, email, token_hash, expires_at, created_by, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, NULLIF($6, '')::uuid, $7)
		RETURNING `+claimInviteColumns,
		invite.OrganizationID, invite.ContactID, invite.Email, invite.TokenHash, invite.ExpiresAt, invite.CreatedBy, invite.CreatedAt)
	if err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("insert claim invite: %w", err)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanClaimInvite)
	if err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("insert claim invite: %w", err)
	}

	// The token hash and email stay out of the audit row.
	metadata, err := json.Marshal(map[string]any{"crm_contact_id": created.ContactID, "expires_at": created.ExpiresAt})
	if err != nil {
		return domain.ClaimInvite{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (module, event, actor_user_id, target_type, target_id, organization_id, metadata, created_at)
		VALUES ('social', $1, NULLIF($2, '')::uuid, 'customer_claim_invite', $3::uuid, $4::uuid, $5, now())`,
		auditEventInviteSent, created.CreatedBy, created.ID, created.OrganizationID, metadata); err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("insert claim invite audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ClaimInvite{}, fmt.Errorf("commit claim invite: %w", err)
	}
	return created, nil
}

func (r *InviteRepository) FindByTokenHash(ctx context.Context, tokenHash string) (domain.ClaimInvite, bool, error) {
	rows, err := r.db.Query(ctx, `SELECT `+claimInviteColumns+` FROM customer_claim_invites WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return domain.ClaimInvite{}, false, fmt.Errorf("find claim invite: %w", err)
	}
	invite, err := pgx.CollectExactlyOneRow(rows, scanClaimInvite)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClaimInvite{}, false, nil
	}
	if err != nil {
		return domain.ClaimInvite{}, false, fmt.Errorf("scan claim invite: %w", err)
	}
	return invite, true, nil
}

// Accept consumes the invite and creates the active link (verified_via=invite)
// in one transaction. The conditional UPDATE makes the token single-use: a
// concurrent or repeated accept, or one after expiry, gets domain.ErrInviteInvalid.
// When the link insert conflicts the transaction rolls back and the invite
// stays usable.
func (r *InviteRepository) Accept(ctx context.Context, tokenHash, userID string, now time.Time) (domain.CustomerLink, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.CustomerLink{}, err
	}
	defer tx.Rollback(context.Background())

	var inviteID, organizationID, contactID string
	err = tx.QueryRow(ctx, `
		UPDATE customer_claim_invites
		SET used_at = $3, used_by_user_id = $2::uuid
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $3
		RETURNING id::text, organization_id::text, crm_contact_id::text`,
		tokenHash, userID, now).Scan(&inviteID, &organizationID, &contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerLink{}, domain.ErrInviteInvalid
	}
	if err != nil {
		return domain.CustomerLink{}, fmt.Errorf("consume claim invite: %w", err)
	}
	link, err := insertActiveLink(ctx, tx, domain.CustomerLink{
		UserID:         userID,
		OrganizationID: organizationID,
		ContactID:      contactID,
		VerifiedVia:    domain.VerifiedViaInvite,
	}, map[string]any{"invite_id": inviteID})
	if err != nil {
		return domain.CustomerLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CustomerLink{}, fmt.Errorf("commit claim invite accept: %w", err)
	}
	return link, nil
}

func scanClaimInvite(row pgx.CollectableRow) (domain.ClaimInvite, error) {
	var i domain.ClaimInvite
	err := row.Scan(&i.ID, &i.OrganizationID, &i.ContactID, &i.Email, &i.TokenHash,
		&i.ExpiresAt, &i.UsedAt, &i.CreatedBy, &i.CreatedAt)
	return i, err
}
