//go:build integration

package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

// FindClaimableByEmail must find matching contacts across organizations under
// FORCE RLS, but only customers (here: a receivable account sourced from the
// contact), only non-deleted ones, and only with the exact normalized email.
// Meaningful only when the test role is NOT superuser/BYPASSRLS.
func TestEmailLookupFindsContactsAcrossTenants(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var bypass bool
	if err := db.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass); err != nil {
		t.Fatal(err)
	}
	if bypass {
		t.Skip("test role is superuser/BYPASSRLS: RLS does not apply, test would be meaningless")
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "budi-" + suffix + "@x.test"
	newOrg := func(label string) string {
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (type, slug, name, status, data_placement)
			VALUES ('customer', $1, $2, 'active', 'shared') RETURNING id::text`,
			"sc-"+label+"-"+suffix, "ISP "+label).Scan(&id); err != nil {
			t.Fatalf("insert org %s: %v", label, err)
		}
		return id
	}
	orgA, orgB, orgC := newOrg("a"), newOrg("b"), newOrg("c")
	orgs := []string{orgA, orgB, orgC}

	inTenant := func(org string, fn func(exec func(sql string, args ...any) string)) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", org); err != nil {
			t.Fatal(err)
		}
		fn(func(sql string, args ...any) string {
			var id string
			if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
				t.Fatalf("tenant %s: %v", org, err)
			}
			return id
		})
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	const insertContact = `INSERT INTO crm_contacts (organization_id, first_name, last_name, email, deleted_at)
		VALUES ($1::uuid, $2, 'Santoso', $3, $4) RETURNING id::text`
	const insertAccount = `INSERT INTO receivable_accounts (organization_id, name, source_type, source_id)
		VALUES ($1::uuid, 'Budi', 'crm_contact', $2::uuid) RETURNING id::text`

	var contactA, contactB, contactC, otherEmailA, deletedA string
	inTenant(orgA, func(exec func(string, ...any) string) {
		contactA = exec(insertContact, orgA, "Budi", "  "+strings.ToUpper(email)+" ", nil)
		exec(insertAccount, orgA, contactA)
		otherEmailA = exec(insertContact, orgA, "Lain", "other-"+email, nil)
		exec(insertAccount, orgA, otherEmailA)
		deletedA = exec(insertContact, orgA, "Lama", email, time.Now().UTC())
		exec(insertAccount, orgA, deletedA)
	})
	inTenant(orgB, func(exec func(string, ...any) string) {
		contactB = exec(insertContact, orgB, "Budi", email, nil)
		exec(insertAccount, orgB, contactB)
	})
	inTenant(orgC, func(exec func(string, ...any) string) {
		// A lead only: no billing account, no PPPoE onboarding.
		contactC = exec(insertContact, orgC, "Budi", email, nil)
	})
	t.Cleanup(func() {
		c := context.Background()
		for _, org := range orgs {
			tx, err := db.Begin(c)
			if err != nil {
				continue
			}
			_, _ = tx.Exec(c, "SELECT set_config('app.organization_id', $1, true)", org)
			_, _ = tx.Exec(c, `DELETE FROM receivable_accounts WHERE organization_id = $1::uuid`, org)
			_, _ = tx.Exec(c, `DELETE FROM crm_contacts WHERE organization_id = $1::uuid`, org)
			_ = tx.Commit(c)
			_, _ = db.Exec(c, `DELETE FROM organizations WHERE id = $1::uuid`, org)
		}
	})

	dir := socialContactDirectory{db: db, tenants: socialTenantScope{
		orgs: organizationrepo.NewOrganizationRepository(db),
		tx:   database.NewSharedTenantTransactor(db),
	}}

	got, err := dir.FindClaimableByEmail(ctx, " "+strings.ToUpper(email))
	if err != nil {
		t.Fatalf("FindClaimableByEmail: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, c := range got {
		ids = append(ids, c.OrganizationID+"/"+c.ContactID)
		if c.ContactName != "Budi Santoso" || c.OrganizationName == "" {
			t.Fatalf("contact = %+v", c)
		}
	}
	sort.Strings(ids)
	want := []string{orgA + "/" + contactA, orgB + "/" + contactB}
	sort.Strings(want)
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("claimable = %v, want %v (lead %s, other email %s, deleted %s excluded)", ids, want, contactC, otherEmailA, deletedA)
	}

	if _, ok, err := dir.IsClaimable(ctx, orgC, contactC); err != nil || ok {
		t.Fatalf("lead without billing claimable=%v err=%v", ok, err)
	}
	// A contact id from another tenant is never claimable under the wrong organization.
	if _, ok, err := dir.IsClaimable(ctx, orgB, contactA); err != nil || ok {
		t.Fatalf("cross-tenant contact claimable=%v err=%v", ok, err)
	}
	if got, err := dir.FindClaimableByEmail(ctx, "   "); err != nil || len(got) != 0 {
		t.Fatalf("blank email = %+v, %v", got, err)
	}

	// Without the lookup GUC nothing is visible outside a tenant scope.
	var visible int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM crm_contacts WHERE organization_id = ANY($1::uuid[])`, orgs).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("contacts visible without scope = %d, want 0", visible)
	}
}
