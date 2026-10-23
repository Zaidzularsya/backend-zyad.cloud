package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// StaticSelfServeConfig: resolver yang selalu mengembalikan konfigurasi tetap.
func StaticSelfServeConfig(cfg SelfServeConfig) SelfServeConfigResolver {
	return staticSelfServeConfig{cfg}
}

type staticSelfServeConfig struct{ cfg SelfServeConfig }

func (s staticSelfServeConfig) Resolve(context.Context, coretenant.Scope) (SelfServeConfig, error) {
	return s.cfg, nil
}

// PlatformScopeProvider mengembalikan scope org platform.
type PlatformScopeProvider interface {
	PlatformScope(ctx context.Context) (coretenant.Scope, error)
}

// NewScopedSelfServeResolver memilih resolver platform untuk org platform dan resolver tenant untuk lainnya.
func NewScopedSelfServeResolver(platforms PlatformScopeProvider, platform, tenant SelfServeConfigResolver) SelfServeConfigResolver {
	return scopedSelfServeResolver{platforms: platforms, platform: platform, tenant: tenant}
}

type scopedSelfServeResolver struct {
	platforms        PlatformScopeProvider
	platform, tenant SelfServeConfigResolver
}

func (r scopedSelfServeResolver) Resolve(ctx context.Context, scope coretenant.Scope) (SelfServeConfig, error) {
	ps, err := r.platforms.PlatformScope(ctx)
	if err != nil {
		return SelfServeConfig{}, err
	}
	if ps.OrganizationID() == scope.OrganizationID() {
		return r.platform.Resolve(ctx, scope)
	}
	return r.tenant.Resolve(ctx, scope)
}

// TenantOwnerLookup mendaftar owner aktif sebuah organisasi (role organization_owner).
type TenantOwnerLookup interface {
	ListOwners(ctx context.Context, organizationID string) ([]domain.OrganizationMember, error)
}

// TenantPipelineReader adalah bagian baca dari PipelineRepository; sengaja tanpa Create karena resolver
// tidak boleh membuat pipeline (kuota crm.max_pipelines tenant Free = 1).
type TenantPipelineReader interface {
	FindBySystemKey(ctx context.Context, scope coretenant.Scope, key string) (domain.Pipeline, error)
	List(context.Context, coretenant.Scope, repository.PipelineListFilter) ([]domain.Pipeline, int64, error)
}

// NewTenantSelfServeResolver: pemilik tenant jadi bot sekaligus pemilik deal; pipeline = pipeline sistem
// "self_serve" bila ada, selain itu pipeline default (atau yang tertua) yang belum diarsipkan.
func NewTenantSelfServeResolver(owners TenantOwnerLookup, pipelines TenantPipelineReader) SelfServeConfigResolver {
	return tenantSelfServeResolver{owners: owners, pipelines: pipelines}
}

type tenantSelfServeResolver struct {
	owners    TenantOwnerLookup
	pipelines TenantPipelineReader
}

func (r tenantSelfServeResolver) Resolve(ctx context.Context, scope coretenant.Scope) (SelfServeConfig, error) {
	owners, err := r.owners.ListOwners(ctx, scope.OrganizationID())
	if err != nil {
		return SelfServeConfig{}, fmt.Errorf("tenant owner: %w", err)
	}
	if len(owners) == 0 {
		return SelfServeConfig{}, fmt.Errorf("tenant has no active owner: %w", ErrSelfServeNotConfigured)
	}
	pipelineID, err := r.pipelineID(ctx, scope)
	if err != nil {
		return SelfServeConfig{}, err
	}
	return SelfServeConfig{BotUserID: owners[0].UserID, DealOwnerUserID: owners[0].UserID, PipelineID: pipelineID, TenantScope: true}, nil
}

func (r tenantSelfServeResolver) pipelineID(ctx context.Context, scope coretenant.Scope) (string, error) {
	p, err := r.pipelines.FindBySystemKey(ctx, scope, "self_serve")
	if err == nil {
		return p.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("tenant self-serve pipeline: %w", err)
	}
	list, _, err := r.pipelines.List(ctx, scope, repository.PipelineListFilter{})
	if err != nil {
		return "", fmt.Errorf("tenant pipelines: %w", err)
	}
	var best *domain.Pipeline
	for i := range list {
		c := &list[i]
		if c.ArchivedAt != nil || c.DeletedAt != nil {
			continue
		}
		if best == nil || (c.IsDefault && !best.IsDefault) || (c.IsDefault == best.IsDefault && c.CreatedAt.Before(best.CreatedAt)) {
			best = c
		}
	}
	if best == nil {
		return "", fmt.Errorf("tenant has no pipeline: %w", ErrSelfServeNotConfigured)
	}
	return best.ID, nil
}
