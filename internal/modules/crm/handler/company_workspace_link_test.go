package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
)

type linkStubService struct {
	service.CompanyService
	company     domain.Company
	linkErr     error
	linkCalls   []*string
	updateCalls int
}

func (s *linkStubService) SetWorkspaceLink(_ context.Context, _ coretenant.Scope, _ string, tenantOrgID *string, _ string) (domain.Company, error) {
	s.linkCalls = append(s.linkCalls, tenantOrgID)
	if s.linkErr != nil {
		return domain.Company{}, s.linkErr
	}
	c := s.company
	c.TenantOrganizationID = tenantOrgID
	return c, nil
}

func (s *linkStubService) Update(context.Context, coretenant.Scope, string, repository.UpdateCompanyParams) (domain.Company, error) {
	s.updateCalls++
	return s.company, nil
}

func (s *linkStubService) Get(context.Context, coretenant.Scope, string) (domain.Company, error) {
	return s.company, nil
}

func (s *linkStubService) Workspace(_ context.Context, id string) *service.WorkspaceInfo {
	return &service.WorkspaceInfo{ID: id, Name: "Workspace Satu", Slug: "satu", Status: "active", Customer: true}
}

// grantChecker memberi izin hanya untuk permission yang terdaftar.
type grantChecker map[string]bool

func (g grantChecker) allowed(required []string) error {
	for _, p := range required {
		if !g[p] {
			return errors.New("denied")
		}
	}
	return nil
}
func (g grantChecker) Can(_ context.Context, _ string, required []string) error {
	return g.allowed(required)
}
func (g grantChecker) CanOrganization(_ context.Context, _, _ string, required []string) error {
	return g.allowed(required)
}

func newLinkRouter(t *testing.T, orgType coretenant.OrganizationType, svc service.CompanyService, checker permissionmiddleware.CombinedPermissionChecker) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "org", OrganizationType: orgType,
		OrganizationStatus: coretenant.OrganizationStatusActive, MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		MembershipStatus: "active", MembershipVersion: 1, ResolutionSource: coretenant.ResolutionSourceSession,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		permissionmiddleware.SetUserID(c, "u1")
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		c.Next()
	})
	NewCompanyHandler(svc).RegisterRoutes(r.Group(""), checker)
	return r
}

const linkBody = `{"tenant_organization_id":"22222222-2222-2222-2222-222222222222"}`

func TestCompanyPatchWorkspaceLinkRejectsNonPlatformOrg(t *testing.T) {
	svc := &linkStubService{}
	r := newLinkRouter(t, coretenant.OrganizationTypeCustomer, svc, grantChecker{"company.update": true, "company.link_workspace": true})
	w := performJSON(r, http.MethodPatch, "/companies/c1", linkBody)
	if w.Code != http.StatusUnprocessableEntity || len(svc.linkCalls) != 0 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, len(svc.linkCalls), w.Body)
	}
}

func TestCompanyPatchWorkspaceLinkRequiresPermission(t *testing.T) {
	svc := &linkStubService{}
	r := newLinkRouter(t, coretenant.OrganizationTypePlatform, svc, grantChecker{"company.update": true})
	for _, body := range []string{linkBody, `{"tenant_organization_id":null}`} {
		w := performJSON(r, http.MethodPatch, "/companies/c1", body)
		if w.Code != http.StatusForbidden || len(svc.linkCalls) != 0 {
			t.Fatalf("body=%s status=%d calls=%d", body, w.Code, len(svc.linkCalls))
		}
	}
	// PATCH biasa tanpa field tautan tidak butuh company.link_workspace.
	if w := performJSON(r, http.MethodPatch, "/companies/c1", `{"name":"Baru"}`); w.Code != http.StatusOK || svc.updateCalls != 1 {
		t.Fatalf("plain update status=%d updates=%d", w.Code, svc.updateCalls)
	}
}

func TestCompanyPatchWorkspaceLinkLinkAndUnlink(t *testing.T) {
	svc := &linkStubService{}
	r := newLinkRouter(t, coretenant.OrganizationTypePlatform, svc, grantChecker{"company.update": true, "company.link_workspace": true})

	w := performJSON(r, http.MethodPatch, "/companies/c1", linkBody)
	if w.Code != http.StatusOK || len(svc.linkCalls) != 1 || svc.linkCalls[0] == nil || svc.updateCalls != 0 {
		t.Fatalf("link status=%d calls=%v updates=%d body=%s", w.Code, svc.linkCalls, svc.updateCalls, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"slug":"satu"`) {
		t.Fatalf("response lacks tenant_organization summary: %s", w.Body)
	}

	w = performJSON(r, http.MethodPatch, "/companies/c1", `{"tenant_organization_id":null}`)
	if w.Code != http.StatusOK || len(svc.linkCalls) != 2 || svc.linkCalls[1] != nil {
		t.Fatalf("unlink status=%d calls=%v body=%s", w.Code, svc.linkCalls, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"tenant_organization":null`) {
		t.Fatalf("unlinked response must carry tenant_organization:null: %s", w.Body)
	}
}

func TestCompanyPatchWorkspaceLinkConflictPassesThrough(t *testing.T) {
	svc := &linkStubService{linkErr: coreerrors.New("WORKSPACE_ALREADY_LINKED", "taken", http.StatusConflict)}
	r := newLinkRouter(t, coretenant.OrganizationTypePlatform, svc, grantChecker{"company.update": true, "company.link_workspace": true})
	w := performJSON(r, http.MethodPatch, "/companies/c1", linkBody)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "WORKSPACE_ALREADY_LINKED") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}

func TestCompanyGetHidesWorkspaceSummaryFromNonPlatformOrg(t *testing.T) {
	ws := "22222222-2222-2222-2222-222222222222"
	svc := &linkStubService{company: domain.Company{ID: "c1", TenantOrganizationID: &ws}}
	r := newLinkRouter(t, coretenant.OrganizationTypeCustomer, svc, grantChecker{"company.read": true})
	w := performJSON(r, http.MethodGet, "/companies/c1", "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"slug":"satu"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}
