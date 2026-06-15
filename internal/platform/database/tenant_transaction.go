package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
)

var (
	ErrTenantContextRequired      = errors.New("tenant context is required")
	ErrTenantPoolResolverRequired = errors.New("tenant pool resolver is required")
	ErrTenantPoolRequired         = errors.New("tenant database pool is required")
	ErrDedicatedPlacementNotReady = errors.New("dedicated tenant database placement is not available")
	ErrTenantTransactionCallback  = errors.New("tenant transaction callback is required")
)

type TenantPool interface {
	Begin(context.Context) (pgx.Tx, error)
}

type TenantPoolResolver interface {
	ResolveTenantPool(context.Context, coretenant.Context) (TenantPool, error)
}

type SharedTenantPoolResolver struct {
	pool *Pool
}

func NewSharedTenantPoolResolver(pool *Pool) *SharedTenantPoolResolver {
	return &SharedTenantPoolResolver{pool: pool}
}

func (r *SharedTenantPoolResolver) ResolveTenantPool(
	_ context.Context,
	tenantContext coretenant.Context,
) (TenantPool, error) {
	if !tenantContext.IsValid() || strings.TrimSpace(tenantContext.OrganizationID()) == "" {
		return nil, ErrTenantContextRequired
	}
	if tenantContext.DataPlacement() == coretenant.DataPlacementDedicated {
		return nil, ErrDedicatedPlacementNotReady
	}
	if tenantContext.DataPlacement() != coretenant.DataPlacementShared {
		return nil, ErrTenantContextRequired
	}
	if r == nil || r.pool == nil {
		return nil, ErrTenantPoolRequired
	}
	return r.pool, nil
}

type TenantTransactor struct {
	resolver TenantPoolResolver
}

func NewTenantTransactor(resolver TenantPoolResolver) *TenantTransactor {
	return &TenantTransactor{resolver: resolver}
}

func NewSharedTenantTransactor(pool *Pool) *TenantTransactor {
	return NewTenantTransactor(NewSharedTenantPoolResolver(pool))
}

func (t *TenantTransactor) Begin(ctx context.Context) (pgx.Tx, error) {
	tenantContext, err := coretenant.RequireContext(ctx)
	if err != nil {
		return nil, ErrTenantContextRequired
	}
	return t.BeginForTenant(ctx, tenantContext)
}

func (t *TenantTransactor) BeginForTenant(
	ctx context.Context,
	tenantContext coretenant.Context,
) (pgx.Tx, error) {
	if t == nil || t.resolver == nil {
		return nil, ErrTenantPoolResolverRequired
	}
	if !tenantContext.IsValid() || strings.TrimSpace(tenantContext.OrganizationID()) == "" {
		return nil, ErrTenantContextRequired
	}
	pool, err := t.resolver.ResolveTenantPool(ctx, tenantContext)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, ErrTenantPoolRequired
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tenant transaction: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		SELECT set_config('app.organization_id', $1, true)
	`, tenantContext.OrganizationID()); err != nil {
		_ = rollbackTenantTransaction(tx)
		return nil, fmt.Errorf("set tenant transaction context: %w", err)
	}
	return tx, nil
}

func (t *TenantTransactor) Within(
	ctx context.Context,
	fn func(context.Context, pgx.Tx) error,
) error {
	if fn == nil {
		return ErrTenantTransactionCallback
	}
	tx, err := t.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTenantTransaction(tx)

	if err := fn(ctx, tx); err != nil {
		if rollbackErr := rollbackTenantTransaction(tx); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf(
				"rollback tenant transaction: %w",
				rollbackErr,
			))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	return nil
}

func rollbackTenantTransaction(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}
