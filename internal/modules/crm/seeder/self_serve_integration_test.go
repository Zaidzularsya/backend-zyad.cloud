//go:build integration

package seeder_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/crm/seeder"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSeedSelfServeIdempotent(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	orgID := tenants.A.OrganizationID
	botEmail := "self-serve-bot-test@zyad.cloud"

	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active') ON CONFLICT DO NOTHING`, orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tx, err := db.Begin(ctx)
		if err != nil {
			return
		}
		defer tx.Rollback(ctx)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
		_, _ = tx.Exec(ctx, "DELETE FROM crm_pipeline_stages WHERE organization_id = $1", orgID)
		_, _ = tx.Exec(ctx, "DELETE FROM crm_pipelines WHERE organization_id = $1", orgID)
		_, _ = tx.Exec(ctx, "DELETE FROM organization_memberships WHERE organization_id = $1", orgID)
		_, _ = tx.Exec(ctx, "DELETE FROM users WHERE email = $1", botEmail)
		_ = tx.Commit(ctx)
	})

	first, err := seeder.SeedSelfServe(ctx, db, orgID, botEmail)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	second, err := seeder.SeedSelfServe(ctx, db, orgID, botEmail)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if first != second {
		t.Fatalf("not idempotent: %+v vs %+v", first, second)
	}

	var users int
	var status, name string
	if err := db.QueryRow(ctx, `SELECT count(*), max(status), max(name) FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, botEmail).
		Scan(&users, &status, &name); err != nil {
		t.Fatal(err)
	}
	if users != 1 || status != "active" || name != "Self-Serve Bot" {
		t.Fatalf("bot user: n=%d status=%s name=%s", users, status, name)
	}

	var memberships int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id = $1 AND user_id = $2 AND status = 'active'`,
		orgID, first.BotUserID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if memberships != 1 {
		t.Fatalf("bot active memberships = %d, want 1", memberships)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID); err != nil {
		t.Fatal(err)
	}
	var pipelines int
	var isDefault bool
	if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(bool_or(is_default), false) FROM crm_pipelines
		WHERE organization_id = $1 AND system_key = 'self_serve' AND deleted_at IS NULL`, orgID).Scan(&pipelines, &isDefault); err != nil {
		t.Fatal(err)
	}
	if pipelines != 1 || isDefault {
		t.Fatalf("pipelines=%d default=%v", pipelines, isDefault)
	}
	rows, err := tx.Query(ctx, `SELECT name, position, is_won, is_lost FROM crm_pipeline_stages
		WHERE pipeline_id = $1 AND deleted_at IS NULL ORDER BY position`, first.PipelineID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		name      string
		won, lost bool
	}{{"Checkout dimulai", false, false}, {"Menunggu pembayaran", false, false}, {"Won", true, false}, {"Lost", false, true}}
	i := 0
	for rows.Next() {
		var n string
		var pos int
		var won, lost bool
		if err := rows.Scan(&n, &pos, &won, &lost); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || n != want[i].name || pos != i || won != want[i].won || lost != want[i].lost {
			t.Fatalf("stage %d = %s pos=%d won=%v lost=%v", i, n, pos, won, lost)
		}
		i++
	}
	if i != 4 {
		t.Fatalf("stages = %d, want 4", i)
	}
}
