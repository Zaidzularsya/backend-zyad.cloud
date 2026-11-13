package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
)

var (
	// ErrDuplicateRecording: an arrears recording with this idempotency key already exists.
	ErrDuplicateRecording = errors.New("arrears recording for this idempotency key already exists")
	// ErrInvoiceNotOpen: the invoice is not issued/overdue (paid, void or draft).
	ErrInvoiceNotOpen = errors.New("invoice is not issued or overdue")
	// ErrDuplicateArrearsPeriod: the contract period already has an open arrears invoice.
	ErrDuplicateArrearsPeriod = errors.New("contract period already has an open arrears invoice")
)

type CreateRecordingParams struct {
	AccountID, ContractID, Reason, RecordedBy, IdempotencyKey string
}

// PeriodRange is a billed calendar range [Start, End] (inclusive dates).
type PeriodRange struct{ Start, End time.Time }

type ArrearsRepository interface {
	// CreateRecording: same (organization, idempotency key) → ErrDuplicateRecording.
	CreateRecording(ctx context.Context, scope coretenant.Scope, p CreateRecordingParams) (domain.ArrearsRecording, error)
	// FindRecordingByKey reports found=false (no error) when no recording has the key.
	FindRecordingByKey(ctx context.Context, scope coretenant.Scope, key string) (domain.ArrearsRecording, bool, error)
	// GetRecording returns pgx.ErrNoRows when missing.
	GetRecording(ctx context.Context, scope coretenant.Scope, id string) (domain.ArrearsRecording, error)
	// SetNotify stores the summary message result and stamps notified_at.
	SetNotify(ctx context.Context, scope coretenant.Scope, id, status, errText string) error
	// ReleaseRecordingKey replaces the key of a cancelled recording so the client key can be reused.
	ReleaseRecordingKey(ctx context.Context, scope coretenant.Scope, id string) error
	// InvoicesByRecording lists the recording's invoices (without items) by period start.
	InvoicesByRecording(ctx context.Context, scope coretenant.Scope, id string) ([]domain.Invoice, error)
	// ExistingPeriods lists the periods already billed on the contract by non-void invoices:
	// invoice-level periods of invoices linked by contract_id plus item-level periods of
	// items pointing at the contract's items (the initial sales-order invoice).
	ExistingPeriods(ctx context.Context, scope coretenant.Scope, contractID string) ([]PeriodRange, error)
	// SetIsolirFlag updates the flag and writes an audit row in one transaction, only when the
	// value changes. Only issued/overdue invoices (ErrInvoiceNotOpen otherwise; pgx.ErrNoRows if missing).
	SetIsolirFlag(ctx context.Context, scope coretenant.Scope, invoiceID string, value bool, actor string) (changed bool, err error)
}

type arrearsRepository struct{ db *database.Pool }

func NewArrearsRepository(db *database.Pool) ArrearsRepository { return &arrearsRepository{db: db} }

const recordingSelect = `
	SELECT id, account_id, contract_id, reason, COALESCE(recorded_by::text, ''), idempotency_key,
		notify_status, notify_error, notified_at, created_at
	FROM receivable_arrears_recordings`

func scanRecording(row pgx.Row) (domain.ArrearsRecording, error) {
	var r domain.ArrearsRecording
	err := row.Scan(&r.ID, &r.AccountID, &r.ContractID, &r.Reason, &r.RecordedBy, &r.IdempotencyKey,
		&r.NotifyStatus, &r.NotifyError, &r.NotifiedAt, &r.CreatedAt)
	return r, err
}

func (r *arrearsRepository) CreateRecording(ctx context.Context, scope coretenant.Scope, p CreateRecordingParams) (domain.ArrearsRecording, error) {
	var out domain.ArrearsRecording
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanRecording(tx.QueryRow(ctx, `
			INSERT INTO receivable_arrears_recordings (organization_id, account_id, contract_id, reason, recorded_by, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, account_id, contract_id, reason, COALESCE(recorded_by::text, ''), idempotency_key,
				notify_status, notify_error, notified_at, created_at`,
			scope.OrganizationID(), p.AccountID, p.ContractID, p.Reason, nullableString(p.RecordedBy), p.IdempotencyKey))
		return err
	})
	if isUniqueViolation(err, "receivable_arrears_recordings_key_unique") {
		return domain.ArrearsRecording{}, ErrDuplicateRecording
	}
	return out, err
}

func (r *arrearsRepository) FindRecordingByKey(ctx context.Context, scope coretenant.Scope, key string) (domain.ArrearsRecording, bool, error) {
	var out domain.ArrearsRecording
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanRecording(tx.QueryRow(ctx, recordingSelect+` WHERE organization_id = $1 AND idempotency_key = $2`,
			scope.OrganizationID(), key))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ArrearsRecording{}, false, nil
	}
	return out, err == nil, err
}

func (r *arrearsRepository) GetRecording(ctx context.Context, scope coretenant.Scope, id string) (domain.ArrearsRecording, error) {
	var out domain.ArrearsRecording
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanRecording(tx.QueryRow(ctx, recordingSelect+` WHERE organization_id = $1 AND id = $2`,
			scope.OrganizationID(), id))
		return err
	})
	return out, err
}

func (r *arrearsRepository) SetNotify(ctx context.Context, scope coretenant.Scope, id, status, errText string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE receivable_arrears_recordings SET notify_status = $3, notify_error = $4, notified_at = now()
			WHERE organization_id = $1 AND id = $2`, scope.OrganizationID(), id, status, errText)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *arrearsRepository) ReleaseRecordingKey(ctx context.Context, scope coretenant.Scope, id string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE receivable_arrears_recordings SET idempotency_key = 'cancelled:' || id::text
			WHERE organization_id = $1 AND id = $2`, scope.OrganizationID(), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *arrearsRepository) InvoicesByRecording(ctx context.Context, scope coretenant.Scope, id string) ([]domain.Invoice, error) {
	out := []domain.Invoice{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, invoiceSelect+`
			WHERE i.organization_id = $1 AND i.arrears_recording_id = $2
			ORDER BY i.period_start, i.id`, scope.OrganizationID(), id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			inv, err := scanInvoice(rows)
			if err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

func (r *arrearsRepository) ExistingPeriods(ctx context.Context, scope coretenant.Scope, contractID string) ([]PeriodRange, error) {
	out := []PeriodRange{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.period_start, i.period_end
			FROM receivable_invoices i
			WHERE i.organization_id = $1 AND i.contract_id = $2 AND i.status <> 'void'
			  AND i.period_start IS NOT NULL AND i.period_end IS NOT NULL
			UNION
			SELECT ii.period_start, ii.period_end
			FROM receivable_invoice_items ii
			JOIN receivable_invoices i ON i.organization_id = ii.organization_id AND i.id = ii.invoice_id
			JOIN receivable_contract_items ci ON ci.organization_id = ii.organization_id AND ci.id = ii.contract_item_id
			WHERE ii.organization_id = $1 AND ci.contract_id = $2 AND i.status <> 'void'
			  AND ii.period_start IS NOT NULL AND ii.period_end IS NOT NULL
			ORDER BY 1, 2`, scope.OrganizationID(), contractID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PeriodRange
			if err := rows.Scan(&p.Start, &p.End); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

func (r *arrearsRepository) SetIsolirFlag(ctx context.Context, scope coretenant.Scope, invoiceID string, value bool, actor string) (bool, error) {
	changed := false
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var status string
		var current bool
		if err := tx.QueryRow(ctx, `
			SELECT status, isolir_flag FROM receivable_invoices WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
			scope.OrganizationID(), invoiceID).Scan(&status, &current); err != nil {
			return err
		}
		if status != string(domain.InvoiceIssued) && status != string(domain.InvoiceOverdue) {
			return ErrInvoiceNotOpen
		}
		if current == value {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET isolir_flag = $3, updated_by = $4, updated_at = now()
			WHERE organization_id = $1 AND id = $2`, scope.OrganizationID(), invoiceID, value, nullableString(actor)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO receivable_isolir_flag_audits (organization_id, invoice_id, old_value, new_value, changed_by)
			VALUES ($1, $2, $3, $4, $5)`, scope.OrganizationID(), invoiceID, current, value, nullableString(actor)); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}
