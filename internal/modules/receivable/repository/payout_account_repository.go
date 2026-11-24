package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/platform/database"
)

var (
	ErrPayoutAccountNotFound      = errors.New("payout account not found")
	ErrPayoutAccountPendingExists = errors.New("organization already has a pending payout account")
	ErrPayoutAccountInvalidState  = errors.New("payout account is not in a valid state for this action")
)

const (
	payoutStatusPending    = "pending"
	payoutStatusVerified   = "verified"
	payoutStatusRejected   = "rejected"
	payoutStatusCancelled  = "cancelled"
	payoutStatusSuperseded = "superseded"
)

// PayoutAccount adalah satu baris organization_payout_accounts. Isi rekening tidak pernah
// di-update; hanya status dan kolom review yang berubah.
type PayoutAccount struct {
	ID, OrganizationID, BankCode, BankName, AccountNumber, AccountHolderName, Status, SubmittedBy, ReviewedBy, ReviewNote string
	SubmittedAt                                                                                                           time.Time
	ReviewedAt                                                                                                            *time.Time
}

type PayoutAccountWithOrg struct {
	PayoutAccount
	OrganizationName string
}

type NewPayoutAccount struct {
	OrganizationID, BankCode, BankName, AccountNumber, AccountHolderName, ActorUserID string
	At                                                                                time.Time
}

// PayoutAudit: konteks request untuk baris audit_logs.
type PayoutAudit struct {
	ActorUserID, IPAddress, UserAgent string
}

type PayoutSummary struct {
	Verified   *PayoutAccount
	HasPending bool
}

// PayoutAccountRepository mengakses organization_payout_accounts (tanpa RLS, lihat migration
// 000187). Setiap mutasi mengambil advisory lock ledger settlement org yang sama dengan
// InsertManualLocked, sehingga perubahan rekening verified dan pencatatan setoran terserialkan.
type PayoutAccountRepository interface {
	// ListByOrganization: terbaru dulu.
	ListByOrganization(ctx context.Context, orgID string) ([]PayoutAccount, error)
	// ListPending: antrean verifikasi, paling lama diajukan dulu.
	ListPending(ctx context.Context, limit, offset int) ([]PayoutAccountWithOrg, int64, error)
	Find(ctx context.Context, id string) (PayoutAccount, error)
	InsertPending(ctx context.Context, in NewPayoutAccount, a PayoutAudit) (PayoutAccount, error)
	// Cancel hanya untuk pengajuan pending milik orgID; milik org lain dianggap tidak ada.
	Cancel(ctx context.Context, orgID, id string, at time.Time, a PayoutAudit) (PayoutAccount, error)
	Reject(ctx context.Context, id, note string, at time.Time, a PayoutAudit) (PayoutAccount, error)
	// Verify menjadikan rekening verified lama superseded dalam transaksi yang sama.
	Verify(ctx context.Context, id, note string, at time.Time, a PayoutAudit) (PayoutAccount, error)
	// AssignVerified: super admin menetapkan rekening. pending -> cancelled, verified -> superseded,
	// lalu baris baru langsung verified (submitted_by = reviewed_by = actor).
	AssignVerified(ctx context.Context, in NewPayoutAccount, note string, a PayoutAudit) (PayoutAccount, error)
	// SummaryForOrganizations: org tanpa rekening pending/verified tidak punya key (zero value).
	SummaryForOrganizations(ctx context.Context, orgIDs []string) (map[string]PayoutSummary, error)
}

type payoutAccountRepository struct{ db *database.Pool }

func NewPayoutAccountRepository(db *database.Pool) PayoutAccountRepository {
	return &payoutAccountRepository{db: db}
}

const payoutAccountColumns = `id::text, organization_id::text, bank_code, bank_name, account_number, account_holder_name,
	status, COALESCE(submitted_by::text, ''), COALESCE(reviewed_by::text, ''), review_note, submitted_at, reviewed_at`

func scanPayoutAccount(row rowScanner, extra ...any) (PayoutAccount, error) {
	var p PayoutAccount
	dest := []any{&p.ID, &p.OrganizationID, &p.BankCode, &p.BankName, &p.AccountNumber, &p.AccountHolderName,
		&p.Status, &p.SubmittedBy, &p.ReviewedBy, &p.ReviewNote, &p.SubmittedAt, &p.ReviewedAt}
	err := row.Scan(append(dest, extra...)...)
	return p, err
}

// notFoundOr memetakan "tidak ada baris" dan id yang bukan uuid (22P02) ke ErrPayoutAccountNotFound.
func notFoundOr(err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == "22P02") {
		return ErrPayoutAccountNotFound
	}
	return err
}

func (r *payoutAccountRepository) ListByOrganization(ctx context.Context, orgID string) ([]PayoutAccount, error) {
	rows, err := r.db.Query(ctx, `SELECT `+payoutAccountColumns+` FROM organization_payout_accounts
		WHERE organization_id = $1
		ORDER BY submitted_at DESC, created_at DESC, id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PayoutAccount{}
	for rows.Next() {
		p, err := scanPayoutAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *payoutAccountRepository) ListPending(ctx context.Context, limit, offset int) ([]PayoutAccountWithOrg, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM organization_payout_accounts WHERE status = 'pending'`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.organization_id::text, p.bank_code, p.bank_name, p.account_number, p.account_holder_name,
			p.status, COALESCE(p.submitted_by::text, ''), COALESCE(p.reviewed_by::text, ''), p.review_note, p.submitted_at, p.reviewed_at,
			o.name
		FROM organization_payout_accounts p JOIN organizations o ON o.id = p.organization_id
		WHERE p.status = 'pending'
		ORDER BY p.submitted_at, p.id
		LIMIT $1 OFFSET $2`, clampLimit(limit), max(offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PayoutAccountWithOrg{}
	for rows.Next() {
		var w PayoutAccountWithOrg
		p, err := scanPayoutAccount(rows, &w.OrganizationName)
		if err != nil {
			return nil, 0, err
		}
		w.PayoutAccount = p
		out = append(out, w)
	}
	return out, total, rows.Err()
}

func (r *payoutAccountRepository) Find(ctx context.Context, id string) (PayoutAccount, error) {
	p, err := scanPayoutAccount(r.db.QueryRow(ctx, `SELECT `+payoutAccountColumns+` FROM organization_payout_accounts WHERE id = $1`, id))
	if err != nil {
		return PayoutAccount{}, notFoundOr(err)
	}
	return p, nil
}

func (r *payoutAccountRepository) InsertPending(ctx context.Context, in NewPayoutAccount, a PayoutAudit) (PayoutAccount, error) {
	var out PayoutAccount
	err := r.withLedgerLock(ctx, in.OrganizationID, func(tx pgx.Tx) error {
		p, err := insertPayoutAccountTx(ctx, tx, in, payoutStatusPending, "", nil)
		if isUniqueViolation(err, "idx_payout_accounts_one_pending") {
			return ErrPayoutAccountPendingExists
		}
		if err != nil {
			return err
		}
		out = p
		return insertPayoutAuditTx(ctx, tx, p, "payout_account.submitted", a, in.At)
	})
	return out, err
}

func (r *payoutAccountRepository) Cancel(ctx context.Context, orgID, id string, at time.Time, a PayoutAudit) (PayoutAccount, error) {
	// orgID kosong di transition berarti "tanpa batas org"; Cancel milik organisasi tidak boleh begitu.
	if orgID == "" {
		return PayoutAccount{}, ErrPayoutAccountNotFound
	}
	return r.transition(ctx, id, orgID, func(ctx context.Context, tx pgx.Tx, cur PayoutAccount) (PayoutAccount, string, error) {
		p, err := scanPayoutAccount(tx.QueryRow(ctx, `
			UPDATE organization_payout_accounts SET status = 'cancelled', updated_at = $2
			WHERE id = $1 RETURNING `+payoutAccountColumns, cur.ID, at.UTC()))
		return p, "payout_account.cancelled", err
	}, at, a)
}

func (r *payoutAccountRepository) Reject(ctx context.Context, id, note string, at time.Time, a PayoutAudit) (PayoutAccount, error) {
	return r.transition(ctx, id, "", func(ctx context.Context, tx pgx.Tx, cur PayoutAccount) (PayoutAccount, string, error) {
		p, err := reviewPayoutTx(ctx, tx, cur.ID, payoutStatusRejected, note, at, a.ActorUserID)
		return p, "payout_account.rejected", err
	}, at, a)
}

func (r *payoutAccountRepository) Verify(ctx context.Context, id, note string, at time.Time, a PayoutAudit) (PayoutAccount, error) {
	return r.transition(ctx, id, "", func(ctx context.Context, tx pgx.Tx, cur PayoutAccount) (PayoutAccount, string, error) {
		// Supersede dulu agar indeks unik "satu verified per org" tidak bentrok.
		if err := supersedeVerifiedTx(ctx, tx, cur.OrganizationID, at); err != nil {
			return PayoutAccount{}, "", err
		}
		p, err := reviewPayoutTx(ctx, tx, cur.ID, payoutStatusVerified, note, at, a.ActorUserID)
		return p, "payout_account.verified", err
	}, at, a)
}

func (r *payoutAccountRepository) AssignVerified(ctx context.Context, in NewPayoutAccount, note string, a PayoutAudit) (PayoutAccount, error) {
	var out PayoutAccount
	err := r.withLedgerLock(ctx, in.OrganizationID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_payout_accounts SET status = 'cancelled', updated_at = $2
			WHERE organization_id = $1 AND status = 'pending'`, in.OrganizationID, in.At.UTC()); err != nil {
			return err
		}
		if err := supersedeVerifiedTx(ctx, tx, in.OrganizationID, in.At); err != nil {
			return err
		}
		at := in.At.UTC()
		p, err := insertPayoutAccountTx(ctx, tx, in, payoutStatusVerified, note, &at)
		if err != nil {
			return err
		}
		out = p
		return insertPayoutAuditTx(ctx, tx, p, "payout_account.assigned_by_admin", a, in.At)
	})
	return out, err
}

func (r *payoutAccountRepository) SummaryForOrganizations(ctx context.Context, orgIDs []string) (map[string]PayoutSummary, error) {
	out := map[string]PayoutSummary{}
	if len(orgIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT `+payoutAccountColumns+` FROM organization_payout_accounts
		WHERE organization_id = ANY($1::uuid[]) AND status IN ('pending', 'verified')`, orgIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPayoutAccount(rows)
		if err != nil {
			return nil, err
		}
		s := out[p.OrganizationID]
		if p.Status == payoutStatusVerified {
			s.Verified = &p
		} else {
			s.HasPending = true
		}
		out[p.OrganizationID] = s
	}
	return out, rows.Err()
}

// withLedgerLock menjalankan fn dalam transaksi yang memegang advisory lock ledger settlement org.
func (r *payoutAccountRepository) withLedgerLock(ctx context.Context, orgID string, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockSettlementLedger(ctx, tx, orgID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type payoutTransitionFn func(ctx context.Context, tx pgx.Tx, cur PayoutAccount) (PayoutAccount, string, error)

// transition: baca org pemilik (tidak pernah berubah), kunci ledger org itu, lalu baca ulang baris
// FOR UPDATE di dalam kunci dan pastikan masih pending sebelum fn mengubahnya. orgID kosong berarti
// tanpa batasan org (super admin); bila diisi, baris milik org lain dianggap tidak ada.
func (r *payoutAccountRepository) transition(ctx context.Context, id, orgID string, fn payoutTransitionFn, at time.Time, a PayoutAudit) (PayoutAccount, error) {
	var owner string
	err := r.db.QueryRow(ctx, `SELECT organization_id::text FROM organization_payout_accounts
		WHERE id = $1 AND ($2 = '' OR organization_id = NULLIF($2, '')::uuid)`, id, orgID).Scan(&owner)
	if err != nil {
		return PayoutAccount{}, notFoundOr(err)
	}
	var out PayoutAccount
	err = r.withLedgerLock(ctx, owner, func(tx pgx.Tx) error {
		cur, err := scanPayoutAccount(tx.QueryRow(ctx, `SELECT `+payoutAccountColumns+` FROM organization_payout_accounts
			WHERE id = $1 AND organization_id = $2 FOR UPDATE`, id, owner))
		if err != nil {
			return notFoundOr(err)
		}
		if cur.Status != payoutStatusPending {
			return ErrPayoutAccountInvalidState
		}
		p, event, err := fn(ctx, tx, cur)
		if err != nil {
			return err
		}
		out = p
		return insertPayoutAuditTx(ctx, tx, p, event, a, at)
	})
	return out, err
}

func insertPayoutAccountTx(ctx context.Context, tx pgx.Tx, in NewPayoutAccount, status, note string, reviewedAt *time.Time) (PayoutAccount, error) {
	reviewer := ""
	if status == payoutStatusVerified {
		reviewer = in.ActorUserID
	}
	return scanPayoutAccount(tx.QueryRow(ctx, `
		INSERT INTO organization_payout_accounts (organization_id, bank_code, bank_name, account_number, account_holder_name,
			status, submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid, $8, NULLIF($9, '')::uuid, $10, $11, $8, $8)
		RETURNING `+payoutAccountColumns,
		in.OrganizationID, in.BankCode, in.BankName, in.AccountNumber, in.AccountHolderName,
		status, in.ActorUserID, in.At.UTC(), reviewer, reviewedAt, note))
}

func reviewPayoutTx(ctx context.Context, tx pgx.Tx, id, status, note string, at time.Time, reviewerID string) (PayoutAccount, error) {
	return scanPayoutAccount(tx.QueryRow(ctx, `
		UPDATE organization_payout_accounts
		SET status = $2, review_note = $3, reviewed_by = NULLIF($4, '')::uuid, reviewed_at = $5, updated_at = $5
		WHERE id = $1 RETURNING `+payoutAccountColumns, id, status, note, reviewerID, at.UTC()))
}

func supersedeVerifiedTx(ctx context.Context, tx pgx.Tx, orgID string, at time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE organization_payout_accounts SET status = $3, updated_at = $2
		WHERE organization_id = $1 AND status = 'verified'`, orgID, at.UTC(), payoutStatusSuperseded)
	return err
}

func insertPayoutAuditTx(ctx context.Context, tx pgx.Tx, p PayoutAccount, event string, a PayoutAudit, at time.Time) error {
	metadata, err := json.Marshal(map[string]string{
		"bank_code":             p.BankCode,
		"account_number_masked": MaskAccountNumber(p.AccountNumber),
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (module, event, organization_id, actor_user_id, target_type, target_id,
			metadata, ip_address, user_agent, created_at)
		VALUES ('receivable', $1, $2::uuid, NULLIF($3, '')::uuid, 'organization_payout_account', $4::uuid,
			$5, NULLIF($6, '')::inet, NULLIF($7, ''), $8)`,
		event, p.OrganizationID, a.ActorUserID, p.ID, metadata, a.IPAddress, a.UserAgent, at.UTC())
	return err
}

// maskAccountNumber: "••••" + 4 digit terakhir. Audit tidak boleh memuat nomor rekening penuh.
// Sengaja duplikat kecil dari aturan masking service agar repository tidak mengimpor service.
func MaskAccountNumber(number string) string {
	r := []rune(number)
	if len(r) > 4 {
		r = r[len(r)-4:]
	}
	return "••••" + string(r)
}
