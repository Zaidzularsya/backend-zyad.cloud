//go:build integration

package app

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func (f usageFixture) accountPhone(t *testing.T, org, name, phone string) string {
	t.Helper()
	var id string
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `INSERT INTO receivable_accounts (organization_id, name, phone) VALUES ($1, $2, $3) RETURNING id::text`, org, name, phone).Scan(&id); err != nil {
			t.Fatalf("account: %v", err)
		}
	})
	return id
}

func TestAccountsByPhone_NormalizedFormats(t *testing.T) {
	f := setupUsageFixture(t)
	a := f.tenants.A.OrganizationID
	id := f.accountPhone(t, a, "Budi", "0812-3456-789")
	f.accountPhone(t, a, "Other", "0899-0000-111")
	f.accountPhone(t, a, "NoPhone", "")
	got, err := f.reader.AccountsByPhone(f.ctx, scopeOf(t, f, a), "628123456789")
	if err != nil || len(got) != 1 || got[0].ID != id || got[0].Name != "Budi" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestAccountsByPhone_DeletedExcluded(t *testing.T) {
	f := setupUsageFixture(t)
	a := f.tenants.A.OrganizationID
	id := f.accountPhone(t, a, "Gone", "081234567890")
	f.tx(a, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `UPDATE receivable_accounts SET deleted_at = now() WHERE id = $1::uuid`, id); err != nil {
			t.Fatal(err)
		}
	})
	got, err := f.reader.AccountsByPhone(f.ctx, scopeOf(t, f, a), "6281234567890")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestAccountsByPhone_TenantIsolation(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	f.accountPhone(t, b, "B", "0812-3456-789")
	got, err := f.reader.AccountsByPhone(f.ctx, scopeOf(t, f, a), "628123456789")
	if err != nil || len(got) != 0 {
		t.Fatalf("A saw %+v err=%v", got, err)
	}
	got, err = f.reader.AccountsByPhone(f.ctx, scopeOf(t, f, b), "628123456789")
	if err != nil || len(got) != 1 {
		t.Fatalf("B got %+v err=%v", got, err)
	}
}
