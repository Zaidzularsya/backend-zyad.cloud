//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

const (
	sqlStateUniqueViolation = "23505"
	sqlStateCheckViolation  = "23514"
)

// execInOrg menjalankan statement dalam transaksi ber-scope organisasi (crm_*
// memakai FORCE RLS). Hasil di-commit bila sukses; mengembalikan SQLSTATE error
// ("" bila sukses).
func execInOrg(t *testing.T, db *database.Pool, orgID, sql string, args ...any) string {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return pgErr.Code
		}
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return ""
}

func TestSelfServeSchemaConstraints(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	org := tenants.A.OrganizationID
	workspace := tenants.B.OrganizationID

	insertCompany := func(name string) string {
		return execInOrg(t, db, org,
			`INSERT INTO crm_companies (organization_id, name, tenant_organization_id) VALUES ($1, $2, $3)`, org, name, workspace)
	}
	if code := insertCompany("Studio Rina"); code != "" {
		t.Fatalf("first company: %s", code)
	}
	if code := insertCompany("Studio Rina 2"); code != sqlStateUniqueViolation {
		t.Fatalf("duplicate workspace link: got %q want unique violation", code)
	}

	// pipeline + stage + deal untuk quote
	if code := execInOrg(t, db, org, `
		WITH p AS (INSERT INTO crm_pipelines (organization_id, name, system_key) VALUES ($1, 'Self-Serve', 'self_serve') RETURNING id),
		s AS (INSERT INTO crm_pipeline_stages (organization_id, pipeline_id, name, position) SELECT $1, id, 'Checkout dimulai', 0 FROM p RETURNING id, pipeline_id)
		INSERT INTO crm_deals (organization_id, pipeline_id, stage_id, title, value) SELECT $1, pipeline_id, id, 'Deal A', 100 FROM s`, org); code != "" {
		t.Fatalf("seed pipeline/deal: %s", code)
	}
	if code := execInOrg(t, db, org, `INSERT INTO crm_pipelines (organization_id, name, system_key) VALUES ($1, 'Dup', 'self_serve')`, org); code != sqlStateUniqueViolation {
		t.Fatalf("duplicate system_key: got %q", code)
	}

	insertQuote := func(number, channel, status string) string {
		return execInOrg(t, db, org, `
			INSERT INTO crm_quotations (organization_id, deal_id, quotation_number, status, channel)
			SELECT $1, id, $2, $4, NULLIF($3, '') FROM crm_deals WHERE organization_id = $1 AND title = 'Deal A'`,
			org, number, channel, status)
	}
	if code := insertQuote("QUO-SS-1", "self_serve", "draft"); code != "" {
		t.Fatalf("first self-serve quote: %s", code)
	}
	if code := insertQuote("QUO-SS-2", "self_serve", "draft"); code != sqlStateUniqueViolation {
		t.Fatalf("second draft self-serve quote: got %q want unique violation", code)
	}
	if code := execInOrg(t, db, org, `UPDATE crm_quotations SET status = 'superseded' WHERE organization_id = $1 AND quotation_number = 'QUO-SS-1'`, org); code != "" {
		t.Fatalf("supersede: %s", code)
	}
	if code := insertQuote("QUO-SS-3", "self_serve", "draft"); code != "" {
		t.Fatalf("draft after superseded: %s", code)
	}
	if code := insertQuote("QUO-SS-4", "email", "superseded"); code != sqlStateCheckViolation {
		t.Fatalf("channel=email: got %q want check violation", code)
	}
}
