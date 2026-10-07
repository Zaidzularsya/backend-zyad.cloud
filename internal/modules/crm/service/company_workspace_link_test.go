package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type linkRepo struct {
	repository.CompanyRepository
	company domain.Company
	setErr  error
	setTo   *string
}

func (r *linkRepo) FindByID(context.Context, coretenant.Scope, string) (domain.Company, error) {
	return r.company, nil
}

func (r *linkRepo) SetTenantOrganization(_ context.Context, _ coretenant.Scope, _ string, tenantOrgID, _ string) (domain.Company, error) {
	if r.setErr != nil {
		return domain.Company{}, r.setErr
	}
	r.setTo = &tenantOrgID
	c := r.company
	c.TenantOrganizationID = nilIfEmpty(tenantOrgID)
	return c, nil
}

type linkDirectory map[string]WorkspaceInfo

func (d linkDirectory) FindWorkspace(_ context.Context, id string) (WorkspaceInfo, error) {
	ws, ok := d[id]
	if !ok {
		return WorkspaceInfo{}, ErrWorkspaceNotFound
	}
	return ws, nil
}

type linkHookRecorder struct{ calls [][2]string }

func (h *linkHookRecorder) OnWorkspaceLinkChanged(_ context.Context, _ coretenant.Scope, _ string, old, next *string) error {
	val := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	h.calls = append(h.calls, [2]string{val(old), val(next)})
	return nil
}

func appErrCode(t *testing.T, err error) (string, int) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError", err)
	}
	return appErr.Code, appErr.Status
}

func newLinkService(company domain.Company) (CompanyService, *linkRepo, *linkHookRecorder) {
	repo := &linkRepo{company: company}
	hook := &linkHookRecorder{}
	dir := linkDirectory{
		"ws-a":        {ID: "ws-a", Name: "A", Customer: true},
		"ws-b":        {ID: "ws-b", Name: "B", Customer: true},
		"ws-platform": {ID: "ws-platform", Name: "Platform", Customer: false},
	}
	return NewCompanyService(repo, WithWorkspaceLinking(dir, hook)), repo, hook
}

func TestSetWorkspaceLinkLinkSwapAndUnlink(t *testing.T) {
	svc, repo, hook := newLinkService(domain.Company{ID: "co-1"})
	ctx, scope := context.Background(), coretenant.Scope{}

	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", strPtr("ws-a"), "u1"); err != nil {
		t.Fatal(err)
	}
	repo.company.TenantOrganizationID = strPtr("ws-a")
	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", strPtr("ws-a"), "u1"); err != nil { // tidak berubah
		t.Fatal(err)
	}
	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", strPtr("ws-b"), "u1"); err != nil {
		t.Fatal(err)
	}
	repo.company.TenantOrganizationID = strPtr("ws-b")
	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", nil, "u1"); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{"-", "ws-a"}, {"ws-a", "ws-b"}, {"ws-b", "-"}}
	if len(hook.calls) != len(want) {
		t.Fatalf("hook calls = %v, want %v", hook.calls, want)
	}
	for i := range want {
		if hook.calls[i] != want[i] {
			t.Fatalf("hook calls = %v, want %v", hook.calls, want)
		}
	}
	if repo.setTo == nil || *repo.setTo != "" {
		t.Fatalf("unlink must pass an empty tenant org id, got %v", repo.setTo)
	}
}

func TestSetWorkspaceLinkRejectsNonCustomerAndUnknown(t *testing.T) {
	svc, repo, hook := newLinkService(domain.Company{ID: "co-1"})
	ctx, scope := context.Background(), coretenant.Scope{}
	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", strPtr("ws-platform"), "u1"); err == nil {
		t.Fatal("platform workspace must be rejected")
	} else if code, status := appErrCode(t, err); code != "WORKSPACE_NOT_CUSTOMER" || status != http.StatusUnprocessableEntity {
		t.Fatalf("code=%s status=%d", code, status)
	}
	if _, err := svc.SetWorkspaceLink(ctx, scope, "co-1", strPtr("ws-missing"), "u1"); err == nil {
		t.Fatal("unknown workspace must be rejected")
	} else if code, _ := appErrCode(t, err); code != "WORKSPACE_NOT_FOUND" {
		t.Fatalf("code=%s", code)
	}
	if repo.setTo != nil || len(hook.calls) != 0 {
		t.Fatal("nothing may be written for rejected links")
	}
}

func TestSetWorkspaceLinkAlreadyLinkedElsewhere(t *testing.T) {
	svc, repo, hook := newLinkService(domain.Company{ID: "co-1"})
	repo.setErr = repository.ErrWorkspaceAlreadyLinked
	_, err := svc.SetWorkspaceLink(context.Background(), coretenant.Scope{}, "co-1", strPtr("ws-a"), "u1")
	if code, status := appErrCode(t, err); code != "WORKSPACE_ALREADY_LINKED" || status != http.StatusConflict {
		t.Fatalf("code=%s status=%d", code, status)
	}
	if len(hook.calls) != 0 {
		t.Fatal("hook must not run when the link failed")
	}
}
