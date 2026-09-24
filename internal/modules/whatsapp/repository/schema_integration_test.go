//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestWhatsAppTablesRLSFlagsIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	want := map[string]bool{
		"wa_sessions":          true,
		"wa_conversations":     true,
		"wa_messages":          true,
		"wa_session_directory": false,
		"wa_webhook_events":    false,
	}
	for table, rls := range want {
		var enabled, forced bool
		if err := db.QueryRow(ctx, `
			SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1
		`, table).Scan(&enabled, &forced); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if enabled != rls || forced != rls {
			t.Errorf("%s: rls=%v force=%v, want %v", table, enabled, forced, rls)
		}
	}
}

// TestConversationRLSIntegration checks the database policy itself (no
// repository filter involved): a conversation written under organization A is
// invisible under organization B. Skipped when the test role bypasses RLS.
func TestConversationRLSIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()

	var bypass bool
	if err := db.QueryRow(ctx, `
		SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user
	`).Scan(&bypass); err != nil {
		t.Fatalf("read role: %v", err)
	}
	if bypass {
		t.Skip("test database role bypasses RLS; policy cannot be observed")
	}

	sessionA, err := repository.NewSessionRepository(db).Create(ctx, tenants.A.Scope, repository.CreateSessionParams{
		Name: "zc_test_rls_a", Purpose: domain.SessionPurposeSales,
	})
	if err != nil {
		t.Fatalf("Create session A: %v", err)
	}

	countAs := func(orgID, table string) int {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID); err != nil {
			t.Fatalf("set_config: %v", err)
		}
		var total int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", tenants.A.OrganizationID); err != nil {
		t.Fatalf("set_config: %v", err)
	}
	var conversationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO wa_conversations (organization_id, session_id, chat_id)
		VALUES ($1, $2, '6281234567890@c.us') RETURNING id
	`, tenants.A.OrganizationID, sessionA.ID).Scan(&conversationID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO wa_messages (organization_id, conversation_id, direction, body, status)
		VALUES ($1, $2, 'in', 'halo', 'delivered')
	`, tenants.A.OrganizationID, conversationID); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	// Writing a row for another organization must be rejected by WITH CHECK.
	if _, err := tx.Exec(ctx, "SAVEPOINT cross_tenant"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO wa_conversations (organization_id, session_id, chat_id)
		VALUES ($1, $2, 'x@c.us')
	`, tenants.B.OrganizationID, sessionA.ID); err == nil {
		t.Fatal("insert for organization B under organization A context succeeded")
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT cross_tenant"); err != nil {
		t.Fatalf("rollback to savepoint: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, table := range []string{"wa_conversations", "wa_messages"} {
		if got := countAs(tenants.A.OrganizationID, table); got != 1 {
			t.Errorf("%s visible to A = %d, want 1", table, got)
		}
		if got := countAs(tenants.B.OrganizationID, table); got != 0 {
			t.Errorf("%s visible to B = %d, want 0", table, got)
		}
	}
}

func TestWhatsAppSeedsIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	grants := map[string][]string{
		"organization_owner": {
			domain.PermissionSessionRead, domain.PermissionSessionManage,
			domain.PermissionConversationRead, domain.PermissionConversationReadAll,
			domain.PermissionConversationAssign, domain.PermissionMessageSend,
		},
		"super_admin": {
			domain.PermissionSessionRead, domain.PermissionSessionManage,
			domain.PermissionConversationRead, domain.PermissionConversationReadAll,
			domain.PermissionConversationAssign, domain.PermissionMessageSend,
		},
		"member": {
			domain.PermissionSessionRead, domain.PermissionConversationRead, domain.PermissionMessageSend,
		},
	}
	for role, want := range grants {
		rows, err := db.Query(ctx, `
			SELECT p.permission_name
			FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE (r.role_name = $1 OR r.slug = $1) AND p.module = 'whatsapp'
			ORDER BY p.permission_name
		`, role)
		if err != nil {
			t.Fatalf("%s grants: %v", role, err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got[name] = true
		}
		rows.Close()
		if len(got) != len(want) {
			t.Errorf("%s has %d whatsapp permissions, want %d: %v", role, len(got), len(want), got)
		}
		for _, name := range want {
			if !got[name] {
				t.Errorf("%s missing %s", role, name)
			}
		}
	}

	wantLimits := map[string]int64{"growth": 1, "business": 3, "enterprise": 10}
	rows, err := db.Query(ctx, `
		SELECT pp.code, e.value_int
		FROM product_plan_entitlements e
		JOIN product_features f ON f.id = e.feature_id
		JOIN product_plans pp ON pp.id = e.plan_id
		WHERE f.feature_key = $1
	`, domain.FeatureWhatsAppMaxSessions)
	if err != nil {
		t.Fatalf("entitlements: %v", err)
	}
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var code string
		var value int64
		if err := rows.Scan(&code, &value); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[code] = value
	}
	for code, want := range wantLimits {
		if got[code] != want {
			t.Errorf("%s max_sessions = %d, want %d", code, got[code], want)
		}
	}
}
