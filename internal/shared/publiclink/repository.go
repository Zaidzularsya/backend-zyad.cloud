package publiclink

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

const (
	DocumentQuotation = "quotation"
	DocumentInvoice   = "invoice"
)

var ErrLinkInvalid = errors.New("link invalid, revoked or expired")

type Link struct {
	ID, OrganizationID, DocumentType, DocumentID string
	Token                                        string // token mentah (hasil dekripsi); kosong pada hasil Resolve
	ExpiresAt                                    time.Time
	RevokedAt                                    *time.Time
	CreatedAt                                    time.Time
}

type Repository interface {
	// FindActive: link belum dicabut milik dokumen (boleh sudah kedaluwarsa).
	FindActive(ctx context.Context, scope coretenant.Scope, docType, docID string) (link Link, tokenEnc string, err error) // pgx.ErrNoRows
	Insert(ctx context.Context, scope coretenant.Scope, docType, docID string, hash []byte, tokenEnc string, expiresAt time.Time, createdBy string) (Link, error)
	UpdateExpiry(ctx context.Context, scope coretenant.Scope, id string, expiresAt time.Time) error
	Revoke(ctx context.Context, scope coretenant.Scope, docType, docID string, at time.Time) error
	// FindByHash tanpa scope (tabel directory tanpa RLS).
	FindByHash(ctx context.Context, hash []byte) (Link, error)
	Touch(ctx context.Context, id string, at time.Time) error // last_viewed_at, view_count+1
}

type repository struct{ db *database.Pool }

func NewRepository(db *database.Pool) Repository { return &repository{db: db} }

const linkColumns = `id::text, organization_id::text, document_type, document_id::text, expires_at, revoked_at, created_at`

func scanLink(row pgx.Row, extra ...any) (Link, error) {
	var l Link
	dest := append([]any{&l.ID, &l.OrganizationID, &l.DocumentType, &l.DocumentID, &l.ExpiresAt, &l.RevokedAt, &l.CreatedAt}, extra...)
	err := row.Scan(dest...)
	return l, err
}

func (r *repository) FindActive(ctx context.Context, scope coretenant.Scope, docType, docID string) (Link, string, error) {
	if !scope.IsValid() {
		return Link{}, "", coretenant.ErrInvalidScope
	}
	var enc string
	l, err := scanLink(r.db.QueryRow(ctx, `
		SELECT `+linkColumns+`, token_enc FROM public_links
		WHERE organization_id = $1 AND document_type = $2 AND document_id = $3 AND revoked_at IS NULL
	`, scope.OrganizationID(), docType, docID), &enc)
	return l, enc, err
}

func (r *repository) Insert(ctx context.Context, scope coretenant.Scope, docType, docID string, hash []byte, tokenEnc string, expiresAt time.Time, createdBy string) (Link, error) {
	if !scope.IsValid() {
		return Link{}, coretenant.ErrInvalidScope
	}
	var by any
	if createdBy != "" {
		by = createdBy
	}
	return scanLink(r.db.QueryRow(ctx, `
		INSERT INTO public_links (organization_id, document_type, document_id, token_hash, token_enc, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+linkColumns,
		scope.OrganizationID(), docType, docID, hash, tokenEnc, expiresAt, by))
}

func (r *repository) UpdateExpiry(ctx context.Context, scope coretenant.Scope, id string, expiresAt time.Time) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	_, err := r.db.Exec(ctx, `UPDATE public_links SET expires_at = $3 WHERE organization_id = $1 AND id = $2`,
		scope.OrganizationID(), id, expiresAt)
	return err
}

func (r *repository) Revoke(ctx context.Context, scope coretenant.Scope, docType, docID string, at time.Time) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	_, err := r.db.Exec(ctx, `
		UPDATE public_links SET revoked_at = $4
		WHERE organization_id = $1 AND document_type = $2 AND document_id = $3 AND revoked_at IS NULL
	`, scope.OrganizationID(), docType, docID, at)
	return err
}

func (r *repository) FindByHash(ctx context.Context, hash []byte) (Link, error) {
	return scanLink(r.db.QueryRow(ctx, `SELECT `+linkColumns+` FROM public_links WHERE token_hash = $1`, hash))
}

func (r *repository) Touch(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE public_links SET last_viewed_at = $2, view_count = view_count + 1 WHERE id = $1`, id, at)
	return err
}
