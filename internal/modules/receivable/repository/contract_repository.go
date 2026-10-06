package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

type contractRepository struct{ db *database.Pool }

func NewContractRepository(db *database.Pool) ContractRepository { return &contractRepository{db: db} }

const contractSelect = `
	SELECT c.id, c.account_id, c.contract_number, c.status, c.source_type, COALESCE(c.source_id::text, ''),
		c.currency, c.start_date, c.end_date, c.channels, COALESCE(c.pic_user_id::text, ''), c.notes,
		c.end_reason, c.ended_at, COALESCE(c.created_by::text, ''), c.created_at, c.updated_at,
		a.id, a.name, a.company_name, a.email, a.phone, a.address,
		COALESCE(a.source_type, ''), COALESCE(a.source_id::text, ''), a.created_at, a.updated_at
	FROM receivable_contracts c
	JOIN receivable_accounts a ON a.organization_id = c.organization_id AND a.id = c.account_id`

func scanContract(row pgx.Row) (domain.Contract, error) {
	var c domain.Contract
	var status, sourceType string
	err := row.Scan(&c.ID, &c.AccountID, &c.ContractNumber, &status, &sourceType, &c.SourceID,
		&c.Currency, &c.StartDate, &c.EndDate, &c.Channels, &c.PICUserID, &c.Notes,
		&c.EndReason, &c.EndedAt, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		&c.Account.ID, &c.Account.Name, &c.Account.CompanyName, &c.Account.Email, &c.Account.Phone, &c.Account.Address,
		&c.Account.SourceType, &c.Account.SourceID, &c.Account.CreatedAt, &c.Account.UpdatedAt)
	c.Status, c.SourceType = domain.ContractStatus(status), domain.SourceType(sourceType)
	return c, err
}

func loadContractItems(ctx context.Context, tx pgx.Tx, orgID, contractID string) ([]domain.ContractItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, description, quantity::text, unit, unit_price::text, COALESCE(discount_percent::text, ''),
			tax_percent::text, COALESCE(product_id::text, ''), sku, COALESCE(source_line_id::text, ''),
			billing_frequency, payment_timing, period_index, next_period_start, next_period_end, position
		FROM receivable_contract_items
		WHERE organization_id = $1 AND contract_id = $2 ORDER BY position, created_at, id`, orgID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ContractItem{}
	for rows.Next() {
		var it domain.ContractItem
		var freq, timing string
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.Unit, &it.UnitPrice, &it.DiscountPercent,
			&it.TaxPercent, &it.ProductID, &it.SKU, &it.SourceLineID, &freq, &timing,
			&it.PeriodIndex, &it.NextPeriodStart, &it.NextPeriodEnd, &it.Position); err != nil {
			return nil, err
		}
		it.Frequency, it.PaymentTiming = pricing.Frequency(freq), pricing.PaymentTiming(timing)
		items = append(items, it)
	}
	return items, rows.Err()
}

func findContractInTx(ctx context.Context, tx pgx.Tx, orgID, where string, args ...any) (domain.Contract, error) {
	c, err := scanContract(tx.QueryRow(ctx, contractSelect+` WHERE c.organization_id = $1 AND `+where, append([]any{orgID}, args...)...))
	if err != nil {
		return domain.Contract{}, err
	}
	c.Items, err = loadContractItems(ctx, tx, orgID, c.ID)
	return c, err
}

func (r *contractRepository) Create(ctx context.Context, scope coretenant.Scope, p CreateContractParams) (domain.Contract, error) {
	sourceType := p.SourceType
	if sourceType == "" {
		sourceType = string(domain.SourceManual)
	}
	currency := p.Currency
	if currency == "" {
		currency = "IDR"
	}
	var out domain.Contract
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO receivable_contracts (organization_id, account_id, contract_number, source_type, source_id,
				currency, start_date, channels, pic_user_id, notes, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
			RETURNING id`,
			scope.OrganizationID(), p.AccountID, p.Number, sourceType, nullableString(p.SourceID), currency,
			p.StartDate, channelsOrDefault(p.Channels), nullableString(p.PICUserID), p.Notes,
			nullableString(p.CreatedBy)).Scan(&id); err != nil {
			return err
		}
		for i, it := range p.Items {
			if _, err := tx.Exec(ctx, `
				INSERT INTO receivable_contract_items (organization_id, contract_id, description, quantity, unit, unit_price,
					discount_percent, tax_percent, product_id, sku, source_line_id, billing_frequency, payment_timing,
					period_index, next_period_start, next_period_end, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
				scope.OrganizationID(), id, it.Description, it.Quantity, it.Unit, it.UnitPrice,
				nullableString(it.DiscountPercent), it.TaxPercent, nullableString(it.ProductID), it.SKU,
				nullableString(it.SourceLineID), string(it.Frequency), string(it.PaymentTiming),
				it.PeriodIndex, it.NextPeriodStart, it.NextPeriodEnd, i); err != nil {
				return err
			}
		}
		var err error
		out, err = findContractInTx(ctx, tx, scope.OrganizationID(), `c.id = $2`, id)
		return err
	})
	if isUniqueViolation(err, "idx_receivable_contracts_source_unique") {
		return domain.Contract{}, ErrDuplicateSource
	}
	return out, err
}

func (r *contractRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Contract, error) {
	var out domain.Contract
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findContractInTx(ctx, tx, scope.OrganizationID(), `c.id = $2`, id)
		return err
	})
	return out, err
}

func (r *contractRepository) FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID string) (domain.Contract, error) {
	var out domain.Contract
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findContractInTx(ctx, tx, scope.OrganizationID(), `c.source_type = $2 AND c.source_id = $3`, sourceType, sourceID)
		return err
	})
	return out, err
}

func (r *contractRepository) List(ctx context.Context, scope coretenant.Scope, f ContractListFilter) ([]domain.Contract, int64, error) {
	conds := []string{"c.organization_id = $1"}
	args := []any{scope.OrganizationID()}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("c.status = ?", f.Status)
	}
	if f.AccountID != "" {
		add("c.account_id = ?", f.AccountID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		add("(c.contract_number ILIKE ? OR a.name ILIKE ? OR a.company_name ILIKE ?)", "%"+escapeLike(s)+"%")
	}
	where := strings.Join(conds, " AND ")
	out := []domain.Contract{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM receivable_contracts c
			JOIN receivable_accounts a ON a.organization_id = c.organization_id AND a.id = c.account_id
			WHERE `+where, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, contractSelect+` WHERE `+where+fmt.Sprintf(
			` ORDER BY c.created_at DESC, c.id LIMIT %d OFFSET %d`, clampLimit(f.Limit), max(f.Offset, 0)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanContract(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, total, err
}

// updateActive menjalankan UPDATE kondisional status = 'active'; nol baris → ErrContractNotActive
// bila contract ada, pgx.ErrNoRows bila tidak.
func (r *contractRepository) updateActive(ctx context.Context, scope coretenant.Scope, id, set string, args ...any) (domain.Contract, error) {
	var out domain.Contract
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		all := append([]any{id, scope.OrganizationID()}, args...)
		tag, err := tx.Exec(ctx, `UPDATE receivable_contracts SET `+set+`, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'active'`, all...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM receivable_contracts WHERE id = $1 AND organization_id = $2)`,
				id, scope.OrganizationID()).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return pgx.ErrNoRows
			}
			return ErrContractNotActive
		}
		out, err = findContractInTx(ctx, tx, scope.OrganizationID(), `c.id = $2`, id)
		return err
	})
	return out, err
}

func (r *contractRepository) SetEndDate(ctx context.Context, scope coretenant.Scope, id string, endDate *time.Time, by string) (domain.Contract, error) {
	return r.updateActive(ctx, scope, id, `end_date = $3, updated_by = $4`, endDate, nullableString(by))
}

func (r *contractRepository) End(ctx context.Context, scope coretenant.Scope, id string, endDate time.Time, reason, by string) (domain.Contract, error) {
	return r.updateActive(ctx, scope, id,
		`status = 'ended', end_date = $3, end_reason = $4, ended_at = now(), updated_by = $5`, endDate, reason, nullableString(by))
}
