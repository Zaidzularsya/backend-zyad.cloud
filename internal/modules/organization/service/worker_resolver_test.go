package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
)

type fakeWorkerResolverStore struct {
	organization model.Organization
	err          error
}

func (f fakeWorkerResolverStore) FindByID(
	context.Context,
	string,
) (model.Organization, error) {
	return f.organization, f.err
}

func TestWorkerResolverBuildsVerifiedContext(t *testing.T) {
	resolver := NewWorkerResolver(fakeWorkerResolverStore{
		organization: workerTestOrganization(coretenant.DataPlacementShared),
	}, "notification-worker")

	tenantContext, err := resolver.ResolveWorkerOrganization(
		context.Background(),
		resolverOrganizationID,
		" Notification-Worker ",
	)
	if err != nil {
		t.Fatalf("ResolveWorkerOrganization() error = %v", err)
	}
	if tenantContext.OrganizationID() != resolverOrganizationID ||
		tenantContext.ResolutionSource() != coretenant.ResolutionSourceWorker {
		t.Fatalf("tenant context = %#v", tenantContext)
	}
}

func TestWorkerResolverRejectsUnknownIdentity(t *testing.T) {
	resolver := NewWorkerResolver(fakeWorkerResolverStore{}, "notification-worker")
	_, err := resolver.ResolveWorkerOrganization(
		context.Background(),
		resolverOrganizationID,
		"unknown-worker",
	)
	assertWorkerResolverErrorCode(t, err, "INTERNAL_SERVICE_IDENTITY_DENIED")
}

func TestWorkerResolverRejectsUnknownOrInactiveOrganization(t *testing.T) {
	resolver := NewWorkerResolver(
		fakeWorkerResolverStore{err: pgx.ErrNoRows},
		"notification-worker",
	)
	_, err := resolver.ResolveWorkerOrganization(
		context.Background(),
		resolverOrganizationID,
		"notification-worker",
	)
	assertWorkerResolverErrorCode(t, err, "WORKER_ORGANIZATION_NOT_FOUND")

	resolver = NewWorkerResolver(fakeWorkerResolverStore{
		organization: model.Organization{
			ID:            resolverOrganizationID,
			Slug:          "acme",
			Type:          coretenant.OrganizationTypeCustomer,
			Status:        coretenant.OrganizationStatusSuspended,
			DataPlacement: coretenant.DataPlacementShared,
		},
	}, "notification-worker")
	_, err = resolver.ResolveWorkerOrganization(
		context.Background(),
		resolverOrganizationID,
		"notification-worker",
	)
	assertWorkerResolverErrorCode(t, err, "WORKER_ORGANIZATION_INACTIVE")
}

func TestWorkerResolverRejectsUnavailableDataPlacement(t *testing.T) {
	resolver := NewWorkerResolver(fakeWorkerResolverStore{
		organization: workerTestOrganization(coretenant.DataPlacementDedicated),
	}, "notification-worker")
	_, err := resolver.ResolveWorkerOrganization(
		context.Background(),
		resolverOrganizationID,
		"notification-worker",
	)
	assertWorkerResolverErrorCode(t, err, "WORKER_DATA_PLACEMENT_UNAVAILABLE")
}

func workerTestOrganization(placement coretenant.DataPlacement) model.Organization {
	return model.Organization{
		ID:            resolverOrganizationID,
		Slug:          "acme",
		Type:          coretenant.OrganizationTypeCustomer,
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: placement,
	}
}

func assertWorkerResolverErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}
