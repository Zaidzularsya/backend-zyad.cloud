//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

type fixedTenantPoolResolver struct {
	pool database.TenantPool
}

func (r fixedTenantPoolResolver) ResolveTenantPool(
	context.Context,
	coretenant.Context,
) (database.TenantPool, error) {
	return r.pool, nil
}

func TestTenantTransactorSetsAndCleansTransactionContextIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	connection, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer connection.Release()

	tenantContext := integrationTenantContext(t)
	ctx = coretenant.WithContext(ctx, tenantContext)
	transactor := database.NewTenantTransactor(
		fixedTenantPoolResolver{pool: connection},
	)

	if err := transactor.Within(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var organizationID string
		if err := tx.QueryRow(
			ctx,
			`SELECT current_setting('app.organization_id', true)`,
		).Scan(&organizationID); err != nil {
			return err
		}
		if organizationID != tenantContext.OrganizationID() {
			t.Fatalf("transaction organization = %q", organizationID)
		}
		return nil
	}); err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	assertNoTenantSetting(t, ctx, connection)
}

func TestTenantTransactorRollbackDoesNotLeakContextIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	connection, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer connection.Release()

	tenantContext := integrationTenantContext(t)
	ctx = coretenant.WithContext(ctx, tenantContext)
	transactor := database.NewTenantTransactor(
		fixedTenantPoolResolver{pool: connection},
	)
	callbackError := errors.New("rollback requested")

	err = transactor.Within(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var organizationID string
		if err := tx.QueryRow(
			ctx,
			`SELECT current_setting('app.organization_id', true)`,
		).Scan(&organizationID); err != nil {
			return err
		}
		if organizationID != tenantContext.OrganizationID() {
			t.Fatalf("transaction organization = %q", organizationID)
		}
		return callbackError
	})
	if !errors.Is(err, callbackError) {
		t.Fatalf("Within() error = %v", err)
	}
	assertNoTenantSetting(t, ctx, connection)
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertNoTenantSetting(
	t *testing.T,
	ctx context.Context,
	connection queryRower,
) {
	t.Helper()
	var organizationID sql.NullString
	if err := connection.QueryRow(
		ctx,
		`SELECT current_setting('app.organization_id', true)`,
	).Scan(&organizationID); err != nil {
		t.Fatalf("read connection tenant setting: %v", err)
	}
	if organizationID.Valid && organizationID.String != "" {
		t.Fatalf("tenant setting leaked on connection: %q", organizationID.String)
	}
}

func integrationTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
