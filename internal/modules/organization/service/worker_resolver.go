package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
)

type WorkerResolverStore interface {
	FindByID(context.Context, string) (model.Organization, error)
}

type WorkerResolver struct {
	store             WorkerResolverStore
	allowedIdentities map[string]struct{}
}

func NewWorkerResolver(
	store WorkerResolverStore,
	allowedIdentities ...string,
) *WorkerResolver {
	allowed := make(map[string]struct{}, len(allowedIdentities))
	for _, identity := range allowedIdentities {
		identity = canonicalInternalIdentity(identity)
		if identity != "" {
			allowed[identity] = struct{}{}
		}
	}
	return &WorkerResolver{store: store, allowedIdentities: allowed}
}

func (r *WorkerResolver) ResolveWorkerOrganization(
	ctx context.Context,
	organizationID string,
	serviceIdentity string,
) (coretenant.Context, error) {
	organizationID = strings.TrimSpace(organizationID)
	serviceIdentity = canonicalInternalIdentity(serviceIdentity)
	if !validUUID(organizationID) {
		return coretenant.Context{}, coreerrors.New(
			"WORKER_ORGANIZATION_REQUIRED",
			"worker event requires a valid organization ID",
			http.StatusUnprocessableEntity,
		)
	}
	if _, allowed := r.allowedIdentities[serviceIdentity]; !allowed {
		return coretenant.Context{}, coreerrors.New(
			"INTERNAL_SERVICE_IDENTITY_DENIED",
			"internal service identity is not allowed",
			http.StatusForbidden,
		)
	}
	if r.store == nil {
		return coretenant.Context{}, coreerrors.New(
			"WORKER_TENANT_RESOLVER_STORE_REQUIRED",
			"worker tenant resolver store is required",
			http.StatusInternalServerError,
		)
	}

	organization, err := r.store.FindByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coretenant.Context{}, coreerrors.New(
				"WORKER_ORGANIZATION_NOT_FOUND",
				"worker organization was not found",
				http.StatusNotFound,
			)
		}
		return coretenant.Context{}, coreerrors.Wrap(
			"WORKER_TENANT_RESOLUTION_FAILED",
			"failed to resolve worker organization",
			http.StatusInternalServerError,
			err,
		)
	}
	if !organization.IsActive() {
		return coretenant.Context{}, coreerrors.New(
			"WORKER_ORGANIZATION_INACTIVE",
			"worker organization is not active",
			http.StatusConflict,
		)
	}
	if organization.DataPlacement != coretenant.DataPlacementShared {
		return coretenant.Context{}, coreerrors.New(
			"WORKER_DATA_PLACEMENT_UNAVAILABLE",
			"worker data placement is not available",
			http.StatusServiceUnavailable,
		)
	}

	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     organization.ID,
		OrganizationSlug:   organization.Slug,
		OrganizationType:   organization.Type,
		OrganizationStatus: organization.Status,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      organization.DataPlacement,
	})
	if err != nil {
		return coretenant.Context{}, coreerrors.Wrap(
			"TENANT_CONTEXT_INVALID",
			"resolved worker tenant context is invalid",
			http.StatusInternalServerError,
			err,
		)
	}
	return tenantContext, nil
}

func canonicalInternalIdentity(identity string) string {
	return strings.ToLower(strings.TrimSpace(identity))
}
