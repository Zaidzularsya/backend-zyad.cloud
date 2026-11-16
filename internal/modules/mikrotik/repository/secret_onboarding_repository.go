package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// SecretOnboarding is one row of mikrotik_secret_onboardings. Empty strings
// are stored as NULL ids.
type SecretOnboarding struct {
	SecretID, Stage                                          string
	ContactID, DealID, QuotationID, SalesOrderID, ContractID string
	AccountID, LastError                                     string
	UpdatedAt                                                time.Time
}

// SecretOnboardingRepository stores the onboarding progress per secret.
type SecretOnboardingRepository struct{ db *database.Pool }

func NewSecretOnboardingRepository(db *database.Pool) *SecretOnboardingRepository {
	return &SecretOnboardingRepository{db: db}
}

const secretOnboardingColumns = `secret_id::text, stage, COALESCE(contact_id::text, ''), COALESCE(deal_id::text, ''),
	COALESCE(quotation_id::text, ''), COALESCE(sales_order_id::text, ''), COALESCE(contract_id::text, ''),
	COALESCE(account_id::text, ''), last_error, updated_at`

func scanSecretOnboarding(row pgx.Row) (SecretOnboarding, error) {
	var o SecretOnboarding
	err := row.Scan(&o.SecretID, &o.Stage, &o.ContactID, &o.DealID, &o.QuotationID, &o.SalesOrderID, &o.ContractID,
		&o.AccountID, &o.LastError, &o.UpdatedAt)
	return o, err
}

// Get returns found=false when the secret has no onboarding row.
func (r *SecretOnboardingRepository) Get(ctx context.Context, scope coretenant.Scope, secretID string) (SecretOnboarding, bool, error) {
	var out SecretOnboarding
	found := false
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		o, err := scanSecretOnboarding(tx.QueryRow(ctx, `SELECT `+secretOnboardingColumns+`
			FROM mikrotik_secret_onboardings WHERE organization_id = $1 AND secret_id = $2`, scope.OrganizationID(), secretID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out, found = o, true
		return nil
	})
	return out, found, err
}

// ListBySecrets returns the rows of the given secrets keyed by secret id.
func (r *SecretOnboardingRepository) ListBySecrets(ctx context.Context, scope coretenant.Scope, secretIDs []string) (map[string]SecretOnboarding, error) {
	out := map[string]SecretOnboarding{}
	if len(secretIDs) == 0 {
		return out, nil
	}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+secretOnboardingColumns+`
			FROM mikrotik_secret_onboardings WHERE organization_id = $1 AND secret_id = ANY($2::uuid[])`, scope.OrganizationID(), secretIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanSecretOnboarding(rows)
			if err != nil {
				return err
			}
			out[o.SecretID] = o
		}
		return rows.Err()
	})
	return out, err
}

// Save upserts the row. created_by is kept from the first save.
func (r *SecretOnboardingRepository) Save(ctx context.Context, scope coretenant.Scope, o SecretOnboarding, actor string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO mikrotik_secret_onboardings (organization_id, secret_id, stage, contact_id, deal_id, quotation_id,
				sales_order_id, contract_id, account_id, last_error, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (organization_id, secret_id) DO UPDATE SET
				stage = EXCLUDED.stage, contact_id = EXCLUDED.contact_id, deal_id = EXCLUDED.deal_id,
				quotation_id = EXCLUDED.quotation_id, sales_order_id = EXCLUDED.sales_order_id,
				contract_id = EXCLUDED.contract_id, account_id = EXCLUDED.account_id,
				last_error = EXCLUDED.last_error, updated_at = now()`,
			scope.OrganizationID(), o.SecretID, o.Stage, nullableString(o.ContactID), nullableString(o.DealID),
			nullableString(o.QuotationID), nullableString(o.SalesOrderID), nullableString(o.ContractID),
			nullableString(o.AccountID), o.LastError, nullableString(actor))
		return err
	})
}
