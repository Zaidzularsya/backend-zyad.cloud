package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/platform/database"
)

type PolicyRepository interface {
	// UpsertCustomerPolicy writes the exemption (ON CONFLICT organization_id, account_id).
	// ErrAccountNotFound when the composite FK rejects a foreign account.
	UpsertCustomerPolicy(ctx context.Context, scope coretenant.Scope, accountID string, exempt bool, reason, by string) (domain.CustomerPolicy, error)
	// GetCustomerPolicy reports found=false when no row exists.
	GetCustomerPolicy(ctx context.Context, scope coretenant.Scope, accountID string) (domain.CustomerPolicy, bool, error)
}

type policyRepository struct {
	db *database.Pool
}

func NewPolicyRepository(db *database.Pool) PolicyRepository {
	return &policyRepository{db: db}
}

const policyColumns = `organization_id, account_id, exempt, reason, set_by, set_at`

func scanPolicy(row pgx.Row) (domain.CustomerPolicy, error) {
	var p domain.CustomerPolicy
	err := row.Scan(&p.OrganizationID, &p.AccountID, &p.Exempt, &p.Reason, &p.SetBy, &p.SetAt)
	return p, err
}

func (r *policyRepository) UpsertCustomerPolicy(ctx context.Context, scope coretenant.Scope, accountID string, exempt bool, reason, by string) (domain.CustomerPolicy, error) {
	var out domain.CustomerPolicy
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		p, err := scanPolicy(tx.QueryRow(ctx, `
			INSERT INTO mikrotik_customer_policies (organization_id, account_id, exempt, reason, set_by, set_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (organization_id, account_id) DO UPDATE
			SET exempt = EXCLUDED.exempt, reason = EXCLUDED.reason, set_by = EXCLUDED.set_by, set_at = now()
			RETURNING `+policyColumns,
			scope.OrganizationID(), accountID, exempt, reason, nullableString(by)))
		out = p
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "fk_mikrotik_customer_policies_account" {
		return domain.CustomerPolicy{}, mikrotik.ErrAccountNotFound
	}
	return out, err
}

func (r *policyRepository) GetCustomerPolicy(ctx context.Context, scope coretenant.Scope, accountID string) (domain.CustomerPolicy, bool, error) {
	var out domain.CustomerPolicy
	found := true
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		p, err := scanPolicy(tx.QueryRow(ctx,
			`SELECT `+policyColumns+` FROM mikrotik_customer_policies WHERE organization_id = $1 AND account_id = $2`,
			scope.OrganizationID(), accountID))
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		out = p
		return err
	})
	return out, found && err == nil, err
}
