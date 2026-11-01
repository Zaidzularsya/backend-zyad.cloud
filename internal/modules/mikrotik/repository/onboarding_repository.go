package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// OnboardingCounts are the raw numbers behind the onboarding checklist.
// Routers: non-deleted only. Secrets: only rows still present on the router.
// ProductsMarked: product triggers with enabled = true.
type OnboardingCounts struct {
	Routers, RoutersReady                     int
	SecretsTotal, SecretsPaired, NeedsProfile int
	ProductsMarked                            int
}

// AccountStatus: Secrets counts the account's paired secrets (present or not,
// they still belong to the customer); Isolated is true when any is isolated.
type AccountStatus struct {
	Secrets  int
	Isolated bool
	Exempt   bool
}

// OnboardingRepository serves the read-only summary queries that span several
// mikrotik tables. It is a separate type so the write-side repository
// interfaces stay unchanged.
type OnboardingRepository struct{ db *database.Pool }

func NewOnboardingRepository(db *database.Pool) *OnboardingRepository {
	return &OnboardingRepository{db: db}
}

// Counts returns every onboarding counter in one round trip.
func (r *OnboardingRepository) Counts(ctx context.Context, scope coretenant.Scope) (OnboardingCounts, error) {
	var c OnboardingCounts
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
			(SELECT COUNT(*) FROM mikrotik_routers WHERE organization_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM mikrotik_routers WHERE organization_id = $1 AND deleted_at IS NULL AND isolir_ready),
			COUNT(*) FILTER (WHERE present),
			COUNT(*) FILTER (WHERE present AND account_id IS NOT NULL),
			COUNT(*) FILTER (WHERE present AND needs_original_profile),
			(SELECT COUNT(*) FROM mikrotik_product_triggers WHERE organization_id = $1 AND enabled)
			FROM mikrotik_ppp_secrets WHERE organization_id = $1`, scope.OrganizationID()).
			Scan(&c.Routers, &c.RoutersReady, &c.SecretsTotal, &c.SecretsPaired, &c.NeedsProfile, &c.ProductsMarked)
	})
	return c, err
}

// AccountStatuses summarises paired secrets and exemption of the given
// accounts in one grouped query. Accounts with neither a paired secret nor a
// policy row are absent from the result.
func (r *OnboardingRepository) AccountStatuses(ctx context.Context, scope coretenant.Scope, accountIDs []string) (map[string]AccountStatus, error) {
	out := make(map[string]AccountStatus, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		// FULL JOIN of two per-account aggregates: an exempt account without a
		// secret still reports its exemption.
		rows, err := tx.Query(ctx, `
			WITH sec AS (
				SELECT account_id, COUNT(*) AS n, bool_or(isolation_state = 'isolated') AS isolated
				FROM mikrotik_ppp_secrets
				WHERE organization_id = $1 AND account_id = ANY($2::uuid[])
				GROUP BY account_id
			), pol AS (
				SELECT account_id, exempt FROM mikrotik_customer_policies
				WHERE organization_id = $1 AND account_id = ANY($2::uuid[])
			)
			SELECT COALESCE(sec.account_id, pol.account_id)::text, COALESCE(sec.n, 0), COALESCE(sec.isolated, false), COALESCE(pol.exempt, false)
			FROM sec FULL JOIN pol ON pol.account_id = sec.account_id`,
			scope.OrganizationID(), accountIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var st AccountStatus
			if err := rows.Scan(&id, &st.Secrets, &st.Isolated, &st.Exempt); err != nil {
				return err
			}
			out[id] = st
		}
		return rows.Err()
	})
	return out, err
}
