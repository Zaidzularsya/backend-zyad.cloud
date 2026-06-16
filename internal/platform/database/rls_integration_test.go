//go:build integration

package database_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestOrganizationRLSRejectsMissingAndCrossTenantContextIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("rls"), ".", "_")
	tableName := "test_organization_rls_" + suffix
	quotedTable := pgx.Identifier{tableName}.Sanitize()
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, "DROP TABLE IF EXISTS "+quotedTable)
	})

	var canBypassRLS bool
	if err := db.QueryRow(ctx, `
		SELECT role.rolbypassrls OR role.rolsuper
		FROM pg_roles role
		WHERE role.rolname = current_user
	`).Scan(&canBypassRLS); err != nil {
		t.Fatalf("inspect runtime role: %v", err)
	}
	if canBypassRLS {
		t.Fatal("runtime role can bypass RLS")
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			value text NOT NULL
		)
	`, quotedTable)); err != nil {
		t.Fatalf("create RLS test table: %v", err)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, organization_id, value)
		VALUES
			('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1', '11111111-1111-1111-1111-111111111111', 'organization-a'),
			('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb2', '22222222-2222-2222-2222-222222222222', 'organization-b')
	`, quotedTable)); err != nil {
		t.Fatalf("seed RLS test rows: %v", err)
	}
	if _, err := db.Exec(
		ctx,
		`SELECT apply_organization_rls($1::regclass)`,
		tableName,
	); err != nil {
		t.Fatalf("apply organization RLS: %v", err)
	}
	connection, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire runtime connection: %v", err)
	}
	defer connection.Release()

	defer func() {
		cleanupCtx := context.Background()
		_, _ = connection.Exec(
			cleanupCtx,
			`SELECT set_config('app.organization_id', '', false)`,
		)
	}()

	assertVisibleRLSRows(t, ctx, connection, quotedTable, 0)

	if _, err := connection.Exec(
		ctx,
		`SELECT set_config('app.organization_id', $1, false)`,
		"11111111-1111-1111-1111-111111111111",
	); err != nil {
		t.Fatalf("set organization A context: %v", err)
	}
	assertVisibleRLSRows(t, ctx, connection, quotedTable, 1)

	_, err = connection.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, organization_id, value)
		VALUES (
			'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb3',
			'22222222-2222-2222-2222-222222222222',
			'cross-tenant'
		)
	`, quotedTable))
	if err == nil {
		t.Fatal("cross-tenant insert unexpectedly succeeded")
	}

	if _, err := connection.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, organization_id, value)
		VALUES (
			'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa4',
			'11111111-1111-1111-1111-111111111111',
			'organization-a-own-row'
		)
	`, quotedTable)); err != nil {
		t.Fatalf("same-tenant insert: %v", err)
	}
	assertVisibleRLSRows(t, ctx, connection, quotedTable, 2)
}

func TestReusableTenantIsolationSuiteIntegration(t *testing.T) {
	adapter := newIsolationTableAdapter(t, true, true)
	testutil.RunTenantIsolationSuite(t, adapter)
}

func TestTenantIsolationSuiteDetectsMissingRepositoryPredicateIntegration(t *testing.T) {
	adapter := newIsolationTableAdapter(t, false, false)
	err := testutil.CheckTenantIsolation(
		context.Background(),
		testutil.NewTenantPair(t),
		adapter,
	)
	if err == nil || !strings.Contains(err.Error(), "read organization B row") {
		t.Fatalf("CheckTenantIsolation() error = %v", err)
	}
}

type isolationTableAdapter struct {
	table               string
	transactor          *database.TenantTransactor
	repositoryPredicate bool
}

func newIsolationTableAdapter(
	t *testing.T,
	enableRLS bool,
	repositoryPredicate bool,
) *isolationTableAdapter {
	t.Helper()
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("isolation"), ".", "_")
	tableName := "test_tenant_isolation_" + suffix
	quotedTable := pgx.Identifier{tableName}.Sanitize()

	if _, err := db.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			organization_id uuid NOT NULL,
			record_key text NOT NULL,
			value text NOT NULL,
			PRIMARY KEY (organization_id, record_key)
		)
	`, quotedTable)); err != nil {
		t.Fatalf("create tenant isolation table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DROP TABLE IF EXISTS "+quotedTable)
	})

	if enableRLS {
		if _, err := db.Exec(
			ctx,
			`SELECT apply_organization_rls($1::regclass)`,
			tableName,
		); err != nil {
			t.Fatalf("apply tenant isolation RLS: %v", err)
		}
	}

	return &isolationTableAdapter{
		table:               quotedTable,
		transactor:          database.NewSharedTenantTransactor(db),
		repositoryPredicate: repositoryPredicate,
	}
}

func (a *isolationTableAdapter) Create(
	ctx context.Context,
	tenantContext coretenant.Context,
	key string,
	value string,
) error {
	return a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s (organization_id, record_key, value)
			VALUES ($1::uuid, $2, $3)
		`, a.table), tenantContext.OrganizationID(), key, value)
		return err
	})
}

func (a *isolationTableAdapter) Read(
	ctx context.Context,
	tenantContext coretenant.Context,
	key string,
) (string, bool, error) {
	var value string
	found := false
	err := a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		query, args := a.scopedQuery(
			"SELECT value FROM "+a.table+" WHERE record_key = $1",
			tenantContext,
			key,
		)
		err := tx.QueryRow(ctx, query, args...).Scan(&value)
		if err == pgx.ErrNoRows {
			return nil
		}
		found = err == nil
		return err
	})
	return value, found, err
}

func (a *isolationTableAdapter) List(
	ctx context.Context,
	tenantContext coretenant.Context,
) ([]string, error) {
	return a.queryKeys(ctx, tenantContext)
}

func (a *isolationTableAdapter) Update(
	ctx context.Context,
	tenantContext coretenant.Context,
	key string,
	value string,
) (bool, error) {
	var updated bool
	err := a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		query, args := a.scopedQuery(
			"UPDATE "+a.table+" SET value = $2 WHERE record_key = $1",
			tenantContext,
			key,
			value,
		)
		result, err := tx.Exec(ctx, query, args...)
		updated = err == nil && result.RowsAffected() == 1
		return err
	})
	return updated, err
}

func (a *isolationTableAdapter) Delete(
	ctx context.Context,
	tenantContext coretenant.Context,
	key string,
) (bool, error) {
	var deleted bool
	err := a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		query, args := a.scopedQuery(
			"DELETE FROM "+a.table+" WHERE record_key = $1",
			tenantContext,
			key,
		)
		result, err := tx.Exec(ctx, query, args...)
		deleted = err == nil && result.RowsAffected() == 1
		return err
	})
	return deleted, err
}

func (a *isolationTableAdapter) BulkUpdate(
	ctx context.Context,
	tenantContext coretenant.Context,
	keys []string,
	value string,
) (int64, error) {
	var count int64
	err := a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		query, args := a.scopedQuery(
			"UPDATE "+a.table+" SET value = $2 WHERE record_key = ANY($1)",
			tenantContext,
			keys,
			value,
		)
		result, err := tx.Exec(ctx, query, args...)
		if err == nil {
			count = result.RowsAffected()
		}
		return err
	})
	return count, err
}

func (a *isolationTableAdapter) Export(
	ctx context.Context,
	tenantContext coretenant.Context,
) ([]string, error) {
	return a.queryKeys(ctx, tenantContext)
}

func (a *isolationTableAdapter) queryKeys(
	ctx context.Context,
	tenantContext coretenant.Context,
) ([]string, error) {
	keys := make([]string, 0)
	err := a.within(ctx, tenantContext, func(tx pgx.Tx) error {
		query, args := a.scopedQuery(
			"SELECT record_key FROM "+a.table+" WHERE true",
			tenantContext,
		)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys = append(keys, key)
		}
		return rows.Err()
	})
	return keys, err
}

func (a *isolationTableAdapter) within(
	ctx context.Context,
	tenantContext coretenant.Context,
	fn func(pgx.Tx) error,
) error {
	tx, err := a.transactor.BeginForTenant(ctx, tenantContext)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *isolationTableAdapter) scopedQuery(
	query string,
	tenantContext coretenant.Context,
	args ...any,
) (string, []any) {
	if !a.repositoryPredicate {
		return query, args
	}
	args = append(args, tenantContext.OrganizationID())
	return query + fmt.Sprintf(
		" AND organization_id = $%d::uuid",
		len(args),
	), args
}

type rlsQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertVisibleRLSRows(
	t *testing.T,
	ctx context.Context,
	connection rlsQueryRower,
	quotedTable string,
	want int,
) {
	t.Helper()
	var count int
	err := connection.QueryRow(
		ctx,
		"SELECT count(*) FROM "+quotedTable,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count visible RLS rows: %v", err)
	}
	if count != want {
		t.Fatalf("visible RLS rows = %d, want %d", count, want)
	}
}
