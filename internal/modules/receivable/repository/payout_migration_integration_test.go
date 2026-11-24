//go:build integration

package repository_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

const insertPayoutAccount = `INSERT INTO organization_payout_accounts
	(organization_id, bank_code, bank_name, account_number, account_holder_name, status, submitted_at)
	VALUES ($1, 'BCA', 'Bank Central Asia', '1234567890', 'Test Holder', $2, now())`

func TestPayoutAccountConstraints(t *testing.T) {
	e := setup(t)
	orgID := e.tenants.A.OrganizationID

	// Every case runs in its own rolled-back transaction so the shared test DB stays clean.
	run := func(t *testing.T, statuses []string, wantCode string) {
		t.Helper()
		tx, err := e.db.Begin(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(e.ctx)
		for i, st := range statuses {
			_, err := tx.Exec(e.ctx, insertPayoutAccount, orgID, st)
			last := i == len(statuses)-1
			if !last || wantCode == "" {
				if err != nil {
					t.Fatalf("insert %s #%d: %v", st, i, err)
				}
				continue
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != wantCode {
				t.Fatalf("insert %s #%d: want SQLSTATE %s, got %v", st, i, wantCode, err)
			}
		}
	}

	t.Run("two pending rejected", func(t *testing.T) { run(t, []string{"pending", "pending"}, "23505") })
	t.Run("two verified rejected", func(t *testing.T) { run(t, []string{"verified", "verified"}, "23505") })
	t.Run("pending and verified allowed", func(t *testing.T) { run(t, []string{"pending", "verified"}, "") })
	t.Run("many history rows allowed", func(t *testing.T) {
		run(t, []string{"rejected", "rejected", "cancelled", "superseded", "superseded"}, "")
	})
	t.Run("invalid status rejected", func(t *testing.T) { run(t, []string{"foo"}, "23514") })
}
