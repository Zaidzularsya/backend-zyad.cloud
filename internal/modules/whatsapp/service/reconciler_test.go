package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

type fakeDirectory struct {
	entries []domain.DirectoryEntry
}

func (d fakeDirectory) Resolve(context.Context, string) (domain.DirectoryEntry, error) {
	return domain.DirectoryEntry{}, errors.New("not used")
}

func (d fakeDirectory) ListActive(context.Context) ([]domain.DirectoryEntry, error) {
	return d.entries, nil
}

type fakeResolver struct {
	scopes   map[string]coretenant.Context
	identity string
}

func (r *fakeResolver) ResolveWorkerOrganization(_ context.Context, orgID, identity string) (coretenant.Context, error) {
	r.identity = identity
	tenantContext, ok := r.scopes[orgID]
	if !ok {
		return coretenant.Context{}, errors.New("organization inactive")
	}
	return tenantContext, nil
}

func TestStatusReconcilerRunOnce(t *testing.T) {
	provider := &fakeProvider{info: platformwhatsapp.SessionInfo{Status: platformwhatsapp.SessionStatusWorking}}
	repo := newFakeSessionRepo()
	svc := newTestService(repo, provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	resolver := &fakeResolver{scopes: map[string]coretenant.Context{testOrgID: scopeContext(t, testOrgID)}}
	reconciler := NewStatusReconciler(fakeDirectory{entries: []domain.DirectoryEntry{
		{SessionName: session.Name, OrganizationID: testOrgID, SessionID: session.ID},
		{SessionName: "zc_gone", OrganizationID: "3f2a9c1b-7d4e-4a11-9b2c-00000000dead", SessionID: "x"},
	}}, resolver, svc, nil)

	result, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Checked != 2 || result.Failed != 1 {
		t.Fatalf("result = %+v, want 2 checked / 1 failed", result)
	}
	if resolver.identity != WorkerIdentity {
		t.Fatalf("identity = %q", resolver.identity)
	}
	updated, _ := repo.GetByID(context.Background(), scope, session.ID)
	if updated.Status != domain.SessionStatusWorking {
		t.Fatalf("status after reconcile = %q", updated.Status)
	}
}

func scopeContext(t *testing.T, orgID string) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     orgID,
		OrganizationSlug:   "org",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}
	return tenantContext
}
