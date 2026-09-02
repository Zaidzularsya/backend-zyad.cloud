package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	corevalidation "zyad.cloud/internal/core/validation"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type fakeSectionService struct {
	replaceItems   []repository.ReplaceSectionItem
	replacePageID  string
	replaceActorID string
	replaceOrgType coretenant.OrganizationType
	replaceResult  []domain.LandingSection
	replaceErr     error
	replaceCalls   int
}

func (s *fakeSectionService) Create(context.Context, coretenant.Scope, coretenant.OrganizationType, repository.CreateSectionParams) (domain.LandingSection, error) {
	return domain.LandingSection{}, nil
}
func (s *fakeSectionService) Get(context.Context, coretenant.Scope, string) (domain.LandingSection, error) {
	return domain.LandingSection{}, nil
}
func (s *fakeSectionService) ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingSection, error) {
	return nil, nil
}
func (s *fakeSectionService) Update(context.Context, coretenant.Scope, coretenant.OrganizationType, string, repository.UpdateSectionParams) (domain.LandingSection, error) {
	return domain.LandingSection{}, nil
}
func (s *fakeSectionService) Delete(context.Context, coretenant.Scope, string) error { return nil }
func (s *fakeSectionService) Toggle(context.Context, coretenant.Scope, string, bool, string) error {
	return nil
}
func (s *fakeSectionService) Reorder(context.Context, coretenant.Scope, string, []repository.SectionReorderParam) error {
	return nil
}
func (s *fakeSectionService) ReplaceAll(
	_ context.Context,
	_ coretenant.Scope,
	orgType coretenant.OrganizationType,
	pageID string,
	items []repository.ReplaceSectionItem,
	actorID string,
) ([]domain.LandingSection, error) {
	s.replaceCalls++
	s.replaceItems = items
	s.replacePageID = pageID
	s.replaceActorID = actorID
	s.replaceOrgType = orgType
	return s.replaceResult, s.replaceErr
}

type fakeRevisionService struct {
	autosaveCalled bool
	autosaveParams service.AutosaveParams
}

func (s *fakeRevisionService) AutosaveDraft(_ context.Context, _ coretenant.Scope, params service.AutosaveParams) (domain.LandingPageRevision, error) {
	s.autosaveCalled = true
	s.autosaveParams = params
	return domain.LandingPageRevision{}, nil
}
func (s *fakeRevisionService) ListRevisions(context.Context, coretenant.Scope, string) ([]domain.LandingPageRevision, error) {
	return nil, nil
}
func (s *fakeRevisionService) GetRevision(context.Context, coretenant.Scope, string) (domain.LandingPageRevision, error) {
	return domain.LandingPageRevision{}, nil
}
func (s *fakeRevisionService) RestoreRevision(context.Context, coretenant.Scope, string, string) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}
func (s *fakeRevisionService) ScheduleAction(context.Context, coretenant.Scope, service.SchedulePublishParams) (domain.LandingPageSchedule, error) {
	return domain.LandingPageSchedule{}, nil
}
func (s *fakeRevisionService) ListSchedules(context.Context, coretenant.Scope, string) ([]domain.LandingPageSchedule, error) {
	return nil, nil
}
func (s *fakeRevisionService) CancelSchedule(context.Context, coretenant.Scope, string) error {
	return nil
}

const testSectionPageID = "11111111-1111-1111-1111-111111111111"

func sectionHandlerRouter(t *testing.T, h *AdminSectionHandler, method, path string, fn func(*gin.Context)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	corevalidation.RegisterGinValidators()

	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) {
		tenantContext := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		fn(c)
	})
	return router
}

func TestAdminSectionHandlerReplaceSectionsPassesItemsToService(t *testing.T) {
	sectionSvc := &fakeSectionService{}
	handler := NewAdminSectionHandler(sectionSvc, &fakeRevisionService{})
	router := sectionHandlerRouter(t, handler, http.MethodPut, "/admin/landing-pages/:id/sections", handler.ReplaceSections)

	body := `{
		"sections": [
			{ "key": "hero-1", "type": "hero", "name": "Hero", "is_enabled": true, "content": {}, "style": {} },
			{ "key": "cta-1", "type": "cta", "name": "CTA", "is_enabled": true, "content": {}, "style": {} }
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/admin/landing-pages/"+testSectionPageID+"/sections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if sectionSvc.replaceCalls != 1 {
		t.Fatalf("expected ReplaceAll called once, got %d", sectionSvc.replaceCalls)
	}
	if sectionSvc.replacePageID != testSectionPageID {
		t.Fatalf("page id = %q", sectionSvc.replacePageID)
	}
	if len(sectionSvc.replaceItems) != 2 || sectionSvc.replaceItems[0].Key != "hero-1" || sectionSvc.replaceItems[1].Type != domain.SectionTypeCTA {
		t.Fatalf("items = %#v", sectionSvc.replaceItems)
	}
	if sectionSvc.replaceActorID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("actor id = %q", sectionSvc.replaceActorID)
	}
}

func TestAdminSectionHandlerReplaceSectionsRejectsInvalidKey(t *testing.T) {
	sectionSvc := &fakeSectionService{}
	handler := NewAdminSectionHandler(sectionSvc, &fakeRevisionService{})
	router := sectionHandlerRouter(t, handler, http.MethodPut, "/admin/landing-pages/:id/sections", handler.ReplaceSections)

	body := `{ "sections": [ { "key": "Hero_1", "type": "hero", "name": "Hero" } ] }`
	req := httptest.NewRequest(http.MethodPut, "/admin/landing-pages/"+testSectionPageID+"/sections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if sectionSvc.replaceCalls != 0 {
		t.Fatal("service should not be called on a validation error")
	}
}

func TestAdminSectionHandlerAutosaveWritesRevisionSnapshot(t *testing.T) {
	sectionSvc := &fakeSectionService{
		replaceResult: []domain.LandingSection{{ID: "s1", Key: "hero-1", Type: domain.SectionTypeHero}},
	}
	revisionSvc := &fakeRevisionService{}
	handler := NewAdminSectionHandler(sectionSvc, revisionSvc)
	router := sectionHandlerRouter(t, handler, http.MethodPost, "/admin/landing-pages/:id/sections/autosave", handler.AutosaveSections)

	body := `{ "sections": [ { "key": "hero-1", "type": "hero", "name": "Hero", "is_enabled": true } ] }`
	req := httptest.NewRequest(http.MethodPost, "/admin/landing-pages/"+testSectionPageID+"/sections/autosave", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if sectionSvc.replaceCalls != 1 {
		t.Fatalf("expected ReplaceAll called once, got %d", sectionSvc.replaceCalls)
	}
	if !revisionSvc.autosaveCalled {
		t.Fatal("expected AutosaveDraft to be called")
	}
	if revisionSvc.autosaveParams.PageID != testSectionPageID {
		t.Fatalf("revision page id = %q", revisionSvc.autosaveParams.PageID)
	}
	if revisionSvc.autosaveParams.ChangeNote != "autosave" {
		t.Fatalf("revision change note = %q", revisionSvc.autosaveParams.ChangeNote)
	}
	if _, ok := revisionSvc.autosaveParams.Snapshot["sections"]; !ok {
		t.Fatalf("snapshot missing sections: %#v", revisionSvc.autosaveParams.Snapshot)
	}
}
