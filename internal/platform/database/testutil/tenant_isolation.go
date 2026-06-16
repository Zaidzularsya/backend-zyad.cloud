//go:build integration

package testutil

import (
	"context"
	"fmt"
	"slices"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

type TenantIsolationAdapter interface {
	Create(context.Context, coretenant.Context, string, string) error
	Read(context.Context, coretenant.Context, string) (string, bool, error)
	List(context.Context, coretenant.Context) ([]string, error)
	Update(context.Context, coretenant.Context, string, string) (bool, error)
	Delete(context.Context, coretenant.Context, string) (bool, error)
	BulkUpdate(context.Context, coretenant.Context, []string, string) (int64, error)
	Export(context.Context, coretenant.Context) ([]string, error)
}

func RunTenantIsolationSuite(
	t *testing.T,
	adapter TenantIsolationAdapter,
) {
	t.Helper()
	if err := CheckTenantIsolation(context.Background(), NewTenantPair(t), adapter); err != nil {
		t.Fatal(err)
	}
}

func CheckTenantIsolation(
	ctx context.Context,
	tenants TenantPair,
	adapter TenantIsolationAdapter,
) error {
	if adapter == nil {
		return fmt.Errorf("tenant isolation adapter is required")
	}
	if err := adapter.Create(ctx, coretenant.Context{}, "missing-context", "blocked"); err == nil {
		return fmt.Errorf("create without tenant context unexpectedly succeeded")
	}

	if err := adapter.Create(ctx, tenants.A.Context, "row-a", "value-a"); err != nil {
		return fmt.Errorf("create organization A row: %w", err)
	}
	if err := adapter.Create(ctx, tenants.B.Context, "row-b", "value-b"); err != nil {
		return fmt.Errorf("create organization B row: %w", err)
	}

	if _, found, err := adapter.Read(ctx, tenants.A.Context, "row-b"); err != nil {
		return fmt.Errorf("read organization B row as A: %w", err)
	} else if found {
		return fmt.Errorf("organization A read organization B row")
	}

	if keys, err := adapter.List(ctx, tenants.A.Context); err != nil {
		return fmt.Errorf("list organization A rows: %w", err)
	} else if err := requireKeys(keys, "row-a"); err != nil {
		return fmt.Errorf("organization A list: %w", err)
	}

	if updated, err := adapter.Update(
		ctx,
		tenants.A.Context,
		"row-b",
		"cross-tenant-update",
	); err != nil {
		return fmt.Errorf("update organization B row as A: %w", err)
	} else if updated {
		return fmt.Errorf("organization A updated organization B row")
	}

	if count, err := adapter.BulkUpdate(
		ctx,
		tenants.A.Context,
		[]string{"row-a", "row-b"},
		"bulk-a",
	); err != nil {
		return fmt.Errorf("bulk update organization A rows: %w", err)
	} else if count != 1 {
		return fmt.Errorf("organization A bulk update count = %d, want 1", count)
	}

	if value, found, err := adapter.Read(ctx, tenants.B.Context, "row-b"); err != nil {
		return fmt.Errorf("read organization B row after A bulk update: %w", err)
	} else if !found || value != "value-b" {
		return fmt.Errorf("organization B row changed by organization A bulk update")
	}

	if keys, err := adapter.Export(ctx, tenants.A.Context); err != nil {
		return fmt.Errorf("export organization A rows: %w", err)
	} else if err := requireKeys(keys, "row-a"); err != nil {
		return fmt.Errorf("organization A export: %w", err)
	}

	if deleted, err := adapter.Delete(ctx, tenants.A.Context, "row-b"); err != nil {
		return fmt.Errorf("delete organization B row as A: %w", err)
	} else if deleted {
		return fmt.Errorf("organization A deleted organization B row")
	}

	if deleted, err := adapter.Delete(ctx, tenants.A.Context, "row-a"); err != nil {
		return fmt.Errorf("delete organization A row: %w", err)
	} else if !deleted {
		return fmt.Errorf("organization A could not delete its own row")
	}
	return nil
}

func requireKeys(got []string, want ...string) error {
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("keys = %v, want %v", got, want)
	}
	return nil
}
