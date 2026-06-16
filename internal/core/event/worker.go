package event

import (
	"context"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
)

type WorkerTenantResolver interface {
	ResolveWorkerOrganization(
		context.Context,
		string,
		string,
	) (coretenant.Context, error)
}

func WithWorkerTenantContext(
	ctx context.Context,
	resolver WorkerTenantResolver,
	serviceIdentity string,
	event Envelope,
) (context.Context, coretenant.Context, error) {
	if err := event.Validate(); err != nil {
		return ctx, coretenant.Context{}, err
	}
	tenantContext, err := resolver.ResolveWorkerOrganization(
		ctx,
		event.OrganizationID,
		strings.TrimSpace(serviceIdentity),
	)
	if err != nil {
		return ctx, coretenant.Context{}, err
	}
	return coretenant.WithContext(ctx, tenantContext), tenantContext, nil
}
