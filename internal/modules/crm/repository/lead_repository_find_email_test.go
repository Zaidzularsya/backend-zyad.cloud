//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestFindOpenByEmail_IgnoresConvertedUnqualifiedDeletedAndOtherOrg(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	disablePlaybook(t, db, tenants.B.Scope)
	leads := repository.NewLeadRepository(db)
	const email = "dup@example.com"

	mk := func(name string) domain.Lead {
		l, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: name, Email: email})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	converted := mk("Converted")
	if _, err := leads.ConvertLead(ctx, tenants.A.Scope, converted.ID, repository.ConvertLeadTxParams{
		Contact: repository.CreateContactParams{FirstName: "Converted"},
	}); err != nil {
		t.Fatal(err)
	}
	unqualified := mk("Unqualified")
	if _, err := leads.Disqualify(ctx, tenants.A.Scope, unqualified.ID, repository.DisqualifyLeadParams{Reason: domain.DisqualifyNotInterested}); err != nil {
		t.Fatal(err)
	}
	deleted := mk("Deleted")
	if err := leads.Delete(ctx, tenants.A.Scope, deleted.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := leads.Create(ctx, tenants.B.Scope, repository.CreateLeadParams{ContactName: "Other org", Email: email}); err != nil {
		t.Fatal(err)
	}

	if got, err := leads.FindOpenByEmail(ctx, tenants.A.Scope, email); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %+v err=%v", got, err)
	}

	older := mk("Older open")
	newer := mk("Newer open")
	got, err := leads.FindOpenByEmail(ctx, tenants.A.Scope, email)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != newer.ID || got.ID == older.ID {
		t.Fatalf("expected newest open lead %s, got %s", newer.ID, got.ID)
	}
}

func TestFindOpenByEmail_CaseInsensitive(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	leads := repository.NewLeadRepository(db)

	lead, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Budi", Email: "budi@x.id"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"Budi@X.id", "  BUDI@x.ID "} {
		got, err := leads.FindOpenByEmail(ctx, tenants.A.Scope, q)
		if err != nil || got.ID != lead.ID {
			t.Fatalf("query %q: got %+v err=%v", q, got, err)
		}
	}
	if _, err := leads.FindOpenByEmail(ctx, tenants.B.Scope, "budi@x.id"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other org must not match, err=%v", err)
	}
}
