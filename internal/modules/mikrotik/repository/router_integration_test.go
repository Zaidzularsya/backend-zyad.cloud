//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

type routerEnv struct {
	ctx     context.Context
	db      *database.Pool
	tenants testutil.TenantPair
	repo    repository.RouterRepository
}

func setupRouter(t *testing.T) routerEnv {
	t.Helper()
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active'),
		       ($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING`, tenants.A.OrganizationID, tenants.B.OrganizationID); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	cleanup := func() {
		// mikrotik_routers is under FORCE RLS: delete per organization scope.
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Errorf("cleanup begin: %v", err)
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM mikrotik_routers WHERE organization_id = $1", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM audit_logs WHERE module = 'mikrotik' AND organization_id = $1", orgID)
			_ = tx.Commit(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return routerEnv{ctx: ctx, db: db, tenants: tenants, repo: repository.NewRouterRepository(db)}
}

func routerParams(name string) repository.CreateRouterParams {
	return repository.CreateRouterParams{
		Name: name, Host: "10.1.1.1", Port: 443, APIType: domain.APITypeREST, UseTLS: true,
		TLSFingerprint: "ab12", Username: "admin", PasswordEncrypted: "enc:secret",
	}
}

func (e routerEnv) create(t *testing.T, scope coretenant.Scope, name string) domain.Router {
	t.Helper()
	r, err := e.repo.Create(e.ctx, scope, routerParams(name))
	if err != nil {
		t.Fatalf("create router %q: %v", name, err)
	}
	return r
}

func TestRouterCreateAndFind(t *testing.T) {
	e := setupRouter(t)
	created := e.create(t, e.tenants.A.Scope, "Core Router")
	if created.ID == "" || created.OrganizationID != e.tenants.A.OrganizationID {
		t.Fatalf("unexpected identity: %+v", created)
	}
	got, err := e.repo.FindByID(e.ctx, e.tenants.A.Scope, created.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Name != "Core Router" || got.Host != "10.1.1.1" || got.Port != 443 || got.APIType != domain.APITypeREST ||
		!got.UseTLS || got.TLSFingerprint != "ab12" || got.Username != "admin" || got.PasswordEncrypted != "enc:secret" {
		t.Fatalf("unexpected router: %+v", got)
	}
	if got.IsolirReady || got.IsolirReadyAt != nil || got.LastCheckAt != nil || got.LastCheckStatus != "" {
		t.Fatalf("new router must be not ready and unchecked: %+v", got)
	}
	if err := e.repo.RecordCheck(e.ctx, e.tenants.A.Scope, created.ID, "ok", "7.15"); err != nil {
		t.Fatalf("record check: %v", err)
	}
	if err := e.repo.RecordCheck(e.ctx, e.tenants.A.Scope, created.ID, "unreachable", ""); err != nil {
		t.Fatalf("record check: %v", err)
	}
	got, _ = e.repo.FindByID(e.ctx, e.tenants.A.Scope, created.ID)
	if got.LastCheckAt == nil || got.LastCheckStatus != "unreachable" || got.RouterOSVersion != "7.15" {
		t.Fatalf("empty version must keep previous: %+v", got)
	}
}

func TestRouterNameUniquePerOrganizationCaseInsensitive(t *testing.T) {
	e := setupRouter(t)
	e.create(t, e.tenants.A.Scope, "Core Router")
	_, err := e.repo.Create(e.ctx, e.tenants.A.Scope, routerParams("core router"))
	if !errors.Is(err, mikrotik.ErrRouterNameTaken) {
		t.Fatalf("expected ErrRouterNameTaken, got %v", err)
	}
	if _, err := e.repo.Create(e.ctx, e.tenants.B.Scope, routerParams("CORE ROUTER")); err != nil {
		t.Fatalf("same name in another org must be accepted: %v", err)
	}
}

func TestRouterIsolatedBetweenTenants(t *testing.T) {
	e := setupRouter(t)
	a := e.create(t, e.tenants.A.Scope, "Core Router")
	if _, err := e.repo.FindByID(e.ctx, e.tenants.B.Scope, a.ID); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("find: expected ErrRouterNotFound, got %v", err)
	}
	newName := "Hijacked"
	if _, err := e.repo.Update(e.ctx, e.tenants.B.Scope, a.ID, repository.UpdateRouterParams{Name: &newName}); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("update: expected ErrRouterNotFound, got %v", err)
	}
	if err := e.repo.SoftDelete(e.ctx, e.tenants.B.Scope, a.ID, ""); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("delete: expected ErrRouterNotFound, got %v", err)
	}
	if err := e.repo.RecordCheck(e.ctx, e.tenants.B.Scope, a.ID, "ok", ""); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("record check: expected ErrRouterNotFound, got %v", err)
	}
	items, total, err := e.repo.List(e.ctx, e.tenants.B.Scope, 50, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("list for B: items=%d total=%d err=%v", len(items), total, err)
	}
}

func TestRouterSoftDeleteHidesFromListAndFreesName(t *testing.T) {
	e := setupRouter(t)
	a := e.create(t, e.tenants.A.Scope, "Core Router")
	if err := e.repo.SoftDelete(e.ctx, e.tenants.A.Scope, a.ID, ""); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := e.repo.FindByID(e.ctx, e.tenants.A.Scope, a.ID); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("expected ErrRouterNotFound after delete, got %v", err)
	}
	if err := e.repo.SoftDelete(e.ctx, e.tenants.A.Scope, a.ID, ""); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("second delete: expected ErrRouterNotFound, got %v", err)
	}
	items, total, err := e.repo.List(e.ctx, e.tenants.A.Scope, 50, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("list: items=%d total=%d err=%v", len(items), total, err)
	}
	e.create(t, e.tenants.A.Scope, "Core Router")
}

func TestMarkSetupSetsReadyAtOnlyWhenReady(t *testing.T) {
	e := setupRouter(t)
	a := e.create(t, e.tenants.A.Scope, "Core Router")
	if err := e.repo.MarkSetup(e.ctx, e.tenants.A.Scope, a.ID, true, "isolir", "https://pay.example.com"); err != nil {
		t.Fatalf("mark setup: %v", err)
	}
	got, _ := e.repo.FindByID(e.ctx, e.tenants.A.Scope, a.ID)
	if !got.IsolirReady || got.IsolirReadyAt == nil || got.IsolirProfileName != "isolir" || got.IsolirRedirectURL != "https://pay.example.com" {
		t.Fatalf("unexpected ready state: %+v", got)
	}
	if err := e.repo.MarkSetup(e.ctx, e.tenants.A.Scope, a.ID, false, "isolir", "https://pay.example.com"); err != nil {
		t.Fatalf("mark setup: %v", err)
	}
	got, _ = e.repo.FindByID(e.ctx, e.tenants.A.Scope, a.ID)
	if got.IsolirReady || got.IsolirReadyAt != nil {
		t.Fatalf("not ready must clear ready_at: %+v", got)
	}
}

func TestRouterUpdateEndpointChangeResetsReadiness(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	a := e.create(t, scope, "Core Router")
	ready := func() {
		t.Helper()
		if err := e.repo.MarkSetup(e.ctx, scope, a.ID, true, "isolir", "https://pay.example.com"); err != nil {
			t.Fatalf("mark setup: %v", err)
		}
	}

	// Non-endpoint changes (name, password) keep readiness.
	ready()
	name, pw := "Renamed", "enc:new"
	got, err := e.repo.Update(e.ctx, scope, a.ID, repository.UpdateRouterParams{Name: &name, PasswordEncrypted: &pw})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.IsolirReady || got.IsolirReadyAt == nil || got.Name != "Renamed" || got.PasswordEncrypted != "enc:new" {
		t.Fatalf("non-endpoint update must keep readiness: %+v", got)
	}

	// Endpoint changes reset readiness; untouched fields (password) stay.
	port := 8729
	got, err = e.repo.Update(e.ctx, scope, a.ID, repository.UpdateRouterParams{Port: &port})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.IsolirReady || got.IsolirReadyAt != nil || got.Port != 8729 || got.PasswordEncrypted != "enc:new" || got.Host != "10.1.1.1" {
		t.Fatalf("endpoint update must reset readiness only: %+v", got)
	}
	ready()
	host := "10.2.2.2"
	got, err = e.repo.Update(e.ctx, scope, a.ID, repository.UpdateRouterParams{Host: &host})
	if err != nil || got.IsolirReady || got.IsolirReadyAt != nil {
		t.Fatalf("host change must reset readiness: %+v err=%v", got, err)
	}
	// Name conflict on update maps to ErrRouterNameTaken.
	e.create(t, scope, "Other")
	other := "other"
	if _, err := e.repo.Update(e.ctx, scope, a.ID, repository.UpdateRouterParams{Name: &other}); !errors.Is(err, mikrotik.ErrRouterNameTaken) {
		t.Fatalf("expected ErrRouterNameTaken, got %v", err)
	}
}

func TestInsertAuditPersistsWithOrganization(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	a := e.create(t, scope, "Core Router")
	err := e.repo.InsertAudit(e.ctx, scope, repository.AuditEntry{
		Event: "router.created", TargetID: a.ID, Metadata: map[string]any{"name": "Core Router"},
	})
	if err != nil {
		t.Fatalf("insert audit: %v", err)
	}
	var module, targetType, metaOrg string
	var actorNull bool
	err = e.db.QueryRow(e.ctx, `
		SELECT module, target_type, metadata->>'organization_id', actor_user_id IS NULL
		FROM audit_logs WHERE module = 'mikrotik' AND event = 'router.created' AND target_id = $1`, a.ID,
	).Scan(&module, &targetType, &metaOrg, &actorNull)
	if err != nil {
		t.Fatalf("select audit: %v", err)
	}
	if module != "mikrotik" || targetType != "mikrotik_router" || metaOrg != e.tenants.A.OrganizationID || !actorNull {
		t.Fatalf("unexpected audit row: %s %s %s actorNull=%v", module, targetType, metaOrg, actorNull)
	}
}

func TestHasReadyRouter(t *testing.T) {
	e := setupRouter(t)
	ready := func(scope coretenant.Scope) bool {
		t.Helper()
		ok, err := e.repo.HasReadyRouter(e.ctx, scope)
		if err != nil {
			t.Fatalf("has ready router: %v", err)
		}
		return ok
	}
	if ready(e.tenants.A.Scope) {
		t.Fatal("no routers: want false")
	}
	a := e.create(t, e.tenants.A.Scope, "Core Router")
	if ready(e.tenants.A.Scope) {
		t.Fatal("router not ready: want false")
	}
	if err := e.repo.MarkSetup(e.ctx, e.tenants.A.Scope, a.ID, true, "isolir", ""); err != nil {
		t.Fatalf("mark setup: %v", err)
	}
	if !ready(e.tenants.A.Scope) {
		t.Fatal("ready router: want true")
	}
	if ready(e.tenants.B.Scope) {
		t.Fatal("other organization's ready router must not count")
	}
	if err := e.repo.SoftDelete(e.ctx, e.tenants.A.Scope, a.ID, ""); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if ready(e.tenants.A.Scope) {
		t.Fatal("deleted router must not count")
	}
}
