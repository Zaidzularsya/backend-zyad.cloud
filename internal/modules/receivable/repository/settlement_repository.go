package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/platform/database"
)

// SettlementPaymentParams adalah satu entri pemasukan (kind=payment) ledger penyelesaian.
// Amount berupa string desimal (numeric), tidak pernah float.
type SettlementPaymentParams struct {
	BeneficiaryOrganizationID, InvoiceID, ProviderReference, Amount string
	OccurredAt                                                      time.Time
}

// SettlementRepository menulis ke settlement_entries: pembukuan lintas tenant milik
// platform, tanpa RLS (lihat migration 000165). Pembatasan akses ada di service.
type SettlementRepository interface {
	// InsertPayment idempoten per provider_reference; created=false bila sudah ada.
	InsertPayment(ctx context.Context, p SettlementPaymentParams) (bool, error)
	// FindPaymentByReference: entri payment untuk referensi itu; pgx.ErrNoRows bila tidak ada.
	FindPaymentByReference(ctx context.Context, providerReference string) (SettlementPaymentParams, error)
}

type settlementRepository struct{ db *database.Pool }

func NewSettlementRepository(db *database.Pool) SettlementRepository {
	return &settlementRepository{db: db}
}

func (r *settlementRepository) InsertPayment(ctx context.Context, p SettlementPaymentParams) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, invoice_id, provider_reference, occurred_at)
		VALUES ($1, 'payment', $2::numeric, $3, $4, $5)
		ON CONFLICT (kind, provider_reference) WHERE kind = 'payment' DO NOTHING`,
		p.BeneficiaryOrganizationID, p.Amount, p.InvoiceID, p.ProviderReference, p.OccurredAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *settlementRepository) FindPaymentByReference(ctx context.Context, providerReference string) (SettlementPaymentParams, error) {
	var p SettlementPaymentParams
	err := r.db.QueryRow(ctx, `
		SELECT beneficiary_organization_id::text, invoice_id::text, provider_reference, amount::text, occurred_at
		FROM settlement_entries
		WHERE kind = 'payment' AND provider_reference = $1`, providerReference).
		Scan(&p.BeneficiaryOrganizationID, &p.InvoiceID, &p.ProviderReference, &p.Amount, &p.OccurredAt)
	return p, err
}

// ErrOrganizationNotFound: organisasi penerima tidak ada.
var ErrOrganizationNotFound = errors.New("settlement beneficiary organization not found")

// SettlementTotals: jumlah per jenis entri, string desimal 2 digit dari numeric(20,2).
type SettlementTotals struct{ PaidIn, Settled, Adjusted string }

// SettlementEntry adalah satu baris settlement_entries. Amount string desimal (bertanda untuk adjustment).
// Payout* terisi untuk settlement yang dicatat setelah rekening pencairan ada; kosong untuk entri lain.
type SettlementEntry struct {
	ID, OrganizationID, Kind, Amount, InvoiceID, ProviderReference, Note, CreatedBy string
	OccurredAt, CreatedAt                                                           time.Time
	PayoutAccountID, PayoutBankName, PayoutAccountNumber                            string
}

type SettlementOrgBalance struct {
	OrganizationID, OrganizationName string
	SettlementTotals
}

type SettlementBalanceFilter struct {
	Search        string
	Limit, Offset int
}

// SettlementManualEntry: entri yang dicatat super admin (kind settlement|adjustment).
type SettlementManualEntry struct {
	OrganizationID, Kind, Amount, Note, CreatedBy string
	OccurredAt                                    time.Time
}

// SettlementLedgerRepository: baca saldo dan catat entri manual. Terpisah dari
// SettlementRepository (jalur pemasukan DOKU) agar fake recorder tidak ikut berubah.
type SettlementLedgerRepository interface {
	Totals(ctx context.Context, orgID string) (SettlementTotals, error)
	// BeneficiaryType mengembalikan organizations.type, atau ErrOrganizationNotFound.
	BeneficiaryType(ctx context.Context, orgID string) (string, error)
	// ListBalances: organisasi yang punya minimal satu entri, urut nama.
	ListBalances(ctx context.Context, f SettlementBalanceFilter) ([]SettlementOrgBalance, int64, error)
	// ListEntries: terbaru lebih dulu.
	ListEntries(ctx context.Context, orgID string, limit, offset int) ([]SettlementEntry, int64, error)
	// InsertManualLocked mengunci ledger org (advisory lock transaksi), membaca total di dalam
	// kunci yang sama, menjalankan check, lalu menyisipkan, semuanya dalam satu transaksi.
	// Error dari check dikembalikan apa adanya dan tidak ada yang ditulis. check juga menerima id
	// rekening verified org yang dibaca di dalam kunci ("" bila tidak ada); untuk kind settlement id
	// itu disimpan sebagai payout_account_id.
	InsertManualLocked(ctx context.Context, in SettlementManualEntry, check func(t SettlementTotals, verifiedPayoutAccountID string) error) (SettlementEntry, error)
}

func NewSettlementLedgerRepository(db *database.Pool) SettlementLedgerRepository {
	return &settlementRepository{db: db}
}

const settlementTotalsSQL = `
	COALESCE(SUM(amount) FILTER (WHERE kind = 'payment'), 0)::numeric(20,2)::text,
	COALESCE(SUM(amount) FILTER (WHERE kind = 'settlement'), 0)::numeric(20,2)::text,
	COALESCE(SUM(amount) FILTER (WHERE kind = 'adjustment'), 0)::numeric(20,2)::text`

// settlementEntryColumns berurutan sesuai scanSettlementEntry. Dipakai di RETURNING (tanpa alias);
// ListEntries memakai versi beralias s. ditambah kolom rekening dari LEFT JOIN.
const settlementEntryColumns = `id::text, beneficiary_organization_id::text, kind, amount::text,
	COALESCE(invoice_id::text, ''), COALESCE(provider_reference, ''), note, occurred_at,
	COALESCE(created_by::text, ''), created_at, COALESCE(payout_account_id::text, '')`

const settlementEntryJoinedColumns = `s.id::text, s.beneficiary_organization_id::text, s.kind, s.amount::text,
	COALESCE(s.invoice_id::text, ''), COALESCE(s.provider_reference, ''), s.note, s.occurred_at,
	COALESCE(s.created_by::text, ''), s.created_at, COALESCE(s.payout_account_id::text, ''),
	COALESCE(p.bank_name, ''), COALESCE(p.account_number, '')`

type rowScanner interface{ Scan(dest ...any) error }

func scanSettlementEntry(row rowScanner, extra ...any) (SettlementEntry, error) {
	var e SettlementEntry
	dest := []any{&e.ID, &e.OrganizationID, &e.Kind, &e.Amount, &e.InvoiceID, &e.ProviderReference, &e.Note, &e.OccurredAt, &e.CreatedBy, &e.CreatedAt, &e.PayoutAccountID}
	err := row.Scan(append(dest, extra...)...)
	return e, err
}

// lockSettlementLedger: advisory lock transaksi per org untuk ledger settlement. Dipakai juga oleh
// mutasi rekening pencairan agar rekening verified tidak berubah di tengah pencatatan setoran.
func lockSettlementLedger(ctx context.Context, tx pgx.Tx, orgID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('settlement_entries:' || $1::text, 0))`, orgID)
	return err
}

func (r *settlementRepository) Totals(ctx context.Context, orgID string) (SettlementTotals, error) {
	return queryTotals(ctx, r.db, orgID)
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func queryTotals(ctx context.Context, q queryRower, orgID string) (SettlementTotals, error) {
	var t SettlementTotals
	err := q.QueryRow(ctx, `SELECT `+settlementTotalsSQL+` FROM settlement_entries WHERE beneficiary_organization_id = $1`, orgID).
		Scan(&t.PaidIn, &t.Settled, &t.Adjusted)
	return t, err
}

func (r *settlementRepository) BeneficiaryType(ctx context.Context, orgID string) (string, error) {
	var t string
	err := r.db.QueryRow(ctx, `SELECT type FROM organizations WHERE id = $1`, orgID).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrganizationNotFound
	}
	return t, err
}

func (r *settlementRepository) ListBalances(ctx context.Context, f SettlementBalanceFilter) ([]SettlementOrgBalance, int64, error) {
	pattern := ""
	if s := strings.TrimSpace(f.Search); s != "" {
		pattern = "%" + escapeLike(s) + "%"
	}
	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT count(DISTINCT s.beneficiary_organization_id)
		FROM settlement_entries s JOIN organizations o ON o.id = s.beneficiary_organization_id
		WHERE ($1 = '' OR o.name ILIKE $1)`, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT o.id::text, o.name, `+settlementTotalsSQL+`
		FROM settlement_entries s JOIN organizations o ON o.id = s.beneficiary_organization_id
		WHERE ($1 = '' OR o.name ILIKE $1)
		GROUP BY o.id, o.name
		ORDER BY o.name, o.id
		LIMIT $2 OFFSET $3`, pattern, clampLimit(f.Limit), max(f.Offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []SettlementOrgBalance{}
	for rows.Next() {
		var b SettlementOrgBalance
		if err := rows.Scan(&b.OrganizationID, &b.OrganizationName, &b.PaidIn, &b.Settled, &b.Adjusted); err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func (r *settlementRepository) ListEntries(ctx context.Context, orgID string, limit, offset int) ([]SettlementEntry, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM settlement_entries WHERE beneficiary_organization_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `SELECT `+settlementEntryJoinedColumns+`
		FROM settlement_entries s LEFT JOIN organization_payout_accounts p ON p.id = s.payout_account_id
		WHERE s.beneficiary_organization_id = $1
		ORDER BY s.occurred_at DESC, s.created_at DESC, s.id DESC
		LIMIT $2 OFFSET $3`, orgID, clampLimit(limit), max(offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []SettlementEntry{}
	for rows.Next() {
		var bankName, accountNumber string
		e, err := scanSettlementEntry(rows, &bankName, &accountNumber)
		if err != nil {
			return nil, 0, err
		}
		e.PayoutBankName, e.PayoutAccountNumber = bankName, accountNumber
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// InsertManualLocked: pg_advisory_xact_lock per org menserialkan setoran/penyesuaian pada org
// yang sama sehingga dua setoran paralel tidak sama-sama lolos cek saldo. Entri payment tidak
// ikut mengunci: payment hanya menambah saldo, jadi tidak bisa membuat cek yang lolos menjadi salah.
// Kunci dilepas otomatis saat transaksi commit/rollback. Mutasi rekening pencairan memakai kunci
// yang sama, jadi rekening verified yang dibaca di sini tidak bisa di-supersede sebelum entri commit.
func (r *settlementRepository) InsertManualLocked(ctx context.Context, in SettlementManualEntry, check func(t SettlementTotals, verifiedPayoutAccountID string) error) (SettlementEntry, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SettlementEntry{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockSettlementLedger(ctx, tx, in.OrganizationID); err != nil {
		return SettlementEntry{}, err
	}
	totals, err := queryTotals(ctx, tx, in.OrganizationID)
	if err != nil {
		return SettlementEntry{}, err
	}
	var payoutID, bankName, accountNumber string
	err = tx.QueryRow(ctx, `SELECT id::text, bank_name, account_number FROM organization_payout_accounts
		WHERE organization_id = $1 AND status = 'verified'`, in.OrganizationID).Scan(&payoutID, &bankName, &accountNumber)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SettlementEntry{}, err
	}
	if err := check(totals, payoutID); err != nil {
		return SettlementEntry{}, err
	}
	if in.Kind != "settlement" {
		payoutID, bankName, accountNumber = "", "", ""
	}
	e, err := scanSettlementEntry(tx.QueryRow(ctx, `
		INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, note, occurred_at, created_by, payout_account_id)
		VALUES ($1, $2, $3::numeric, $4, $5, $6, $7)
		RETURNING `+settlementEntryColumns,
		in.OrganizationID, in.Kind, in.Amount, in.Note, in.OccurredAt.UTC(), nullableString(in.CreatedBy), nullableString(payoutID)))
	if err != nil {
		return SettlementEntry{}, err
	}
	e.PayoutBankName, e.PayoutAccountNumber = bankName, accountNumber
	if err := tx.Commit(ctx); err != nil {
		return SettlementEntry{}, err
	}
	return e, nil
}
