package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogservice "zyad.cloud/internal/modules/catalog/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

const (
	publicCatalogIdentity = "public-catalog"
	platformScopeTTL      = 5 * time.Minute
)

// platformScopeResolver menurunkan scope org platform untuk endpoint katalog publik.
// Hasilnya di-cache dalam proses; org platform praktis tidak berubah.
type platformScopeResolver struct {
	orgs     *organizationrepo.OrganizationRepository
	resolver *organizationservice.WorkerResolver

	mu      sync.Mutex
	scope   coretenant.Scope
	expires time.Time
}

func newPlatformScopeResolver(orgs *organizationrepo.OrganizationRepository) *platformScopeResolver {
	return &platformScopeResolver{
		orgs:     orgs,
		resolver: organizationservice.NewWorkerResolver(orgs, publicCatalogIdentity),
	}
}

func (r *platformScopeResolver) PlatformScope(ctx context.Context) (coretenant.Scope, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scope.IsValid() && time.Now().Before(r.expires) {
		return r.scope, nil
	}
	org, err := r.orgs.FindPlatform(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return coretenant.Scope{}, catalogservice.ErrPlatformNotFound
	}
	if err != nil {
		return coretenant.Scope{}, err
	}
	tc, err := r.resolver.ResolveWorkerOrganization(ctx, org.ID, publicCatalogIdentity)
	if err != nil {
		return coretenant.Scope{}, err
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		return coretenant.Scope{}, err
	}
	r.scope, r.expires = scope, time.Now().Add(platformScopeTTL)
	return scope, nil
}
