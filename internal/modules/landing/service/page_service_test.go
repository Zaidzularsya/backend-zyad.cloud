package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type pageServicePageRepoStub struct {
	listTotal    int64
	createCalled bool
	listCalled   bool
	findByIDPage landingdomain.LandingPage
}

func (s *pageServicePageRepoStub) Create(
	context.Context,
	coretenant.Scope,
	repository.CreatePageParams,
) (landingdomain.LandingPage, error) {
	s.createCalled = true
	return landingdomain.LandingPage{
		ID:   "page-1",
		Slug: "test-page",
	}, nil
}

func (s *pageServicePageRepoStub) FindByID(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingPage, error) {
	return s.findByIDPage, nil
}

func (s *pageServicePageRepoStub) List(
	context.Context,
	coretenant.Scope,
	repository.PageListFilter,
) ([]landingdomain.LandingPage, int64, error) {
	s.listCalled = true
	return nil, s.listTotal, nil
}

func (s *pageServicePageRepoStub) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdatePageParams,
) (landingdomain.LandingPage, error) {
	return landingdomain.LandingPage{}, nil
}

func (s *pageServicePageRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

type pageServiceSectionRepoStub struct {
	listByPageItems []landingdomain.LandingSection
	createCount     int
}

func (s *pageServiceSectionRepoStub) Create(
	context.Context,
	coretenant.Scope,
	repository.CreateSectionParams,
) (landingdomain.LandingSection, error) {
	s.createCount++
	return landingdomain.LandingSection{}, nil
}

func (s *pageServiceSectionRepoStub) FindByID(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingSection, error) {
	return landingdomain.LandingSection{}, nil
}

func (s *pageServiceSectionRepoStub) ListByPage(
	context.Context,
	coretenant.Scope,
	string,
) ([]landingdomain.LandingSection, error) {
	return s.listByPageItems, nil
}

func (s *pageServiceSectionRepoStub) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateSectionParams,
) (landingdomain.LandingSection, error) {
	return landingdomain.LandingSection{}, nil
}

func (s *pageServiceSectionRepoStub) Reorder(
	context.Context,
	coretenant.Scope,
	string,
	[]repository.SectionReorderParam,
) error {
	return nil
}

func (s *pageServiceSectionRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

type pageServiceQuotaGuardStub struct {
	organizationID string
	featureKey     string
	limitKey       string
	usedValue      int64
	delta          int64
	err            error
}

func (s *pageServiceQuotaGuardStub) RequireQuotaValue(
	_ context.Context,
	organizationID string,
	featureKey string,
	limitKey string,
	usedValue int64,
	delta int64,
) error {
	s.organizationID = organizationID
	s.featureKey = featureKey
	s.limitKey = limitKey
	s.usedValue = usedValue
	s.delta = delta
	return s.err
}

func TestPageServiceCreateUsesBillingQuotaGuard(t *testing.T) {
	pageRepo := &pageServicePageRepoStub{listTotal: 3}
	guard := &pageServiceQuotaGuardStub{}
	service := NewPageService(
		pageRepo,
		&pageServiceSectionRepoStub{},
		WithLandingPageQuotaGuard(guard),
	)

	_, err := service.Create(context.Background(), mustLandingScope(t), repository.CreatePageParams{
		Title:      "My Page",
		Visibility: landingdomain.PageVisibilityPublic,
		Status:     landingdomain.PageStatusDraft,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !pageRepo.listCalled || !pageRepo.createCalled {
		t.Fatalf("repo calls list=%v create=%v", pageRepo.listCalled, pageRepo.createCalled)
	}
	if guard.organizationID != landingServiceOrganizationID ||
		guard.featureKey != landingdomain.FeatureLandingMaxPages ||
		guard.limitKey != "limit" ||
		guard.usedValue != 3 ||
		guard.delta != 1 {
		t.Fatalf("guard = %#v", guard)
	}
}

func TestPageServiceCreateStopsWhenQuotaExceeded(t *testing.T) {
	pageRepo := &pageServicePageRepoStub{listTotal: 3}
	guardErr := errors.New("quota exceeded")
	service := NewPageService(
		pageRepo,
		&pageServiceSectionRepoStub{},
		WithLandingPageQuotaGuard(&pageServiceQuotaGuardStub{err: guardErr}),
	)

	_, err := service.Create(context.Background(), mustLandingScope(t), repository.CreatePageParams{
		Title:      "My Page",
		Visibility: landingdomain.PageVisibilityPublic,
		Status:     landingdomain.PageStatusDraft,
	})
	if !errors.Is(err, guardErr) {
		t.Fatalf("Create() error = %v, want %v", err, guardErr)
	}
	if pageRepo.createCalled {
		t.Fatal("Create() should not persist page when quota guard fails")
	}
}

func TestPageServiceDuplicateUsesSectionQuotaGuard(t *testing.T) {
	pageRepo := &pageServicePageRepoStub{
		findByIDPage: landingdomain.LandingPage{
			ID:   "page-source",
			Name: "Source",
			Slug: "source-page",
		},
	}
	sectionRepo := &pageServiceSectionRepoStub{
		listByPageItems: []landingdomain.LandingSection{
			{ID: "section-1"},
			{ID: "section-2"},
		},
	}
	guard := &pageServiceQuotaGuardStub{}
	service := NewPageService(
		pageRepo,
		sectionRepo,
		WithLandingPageQuotaGuard(guard),
	)

	_, err := service.Duplicate(context.Background(), mustLandingScope(t), DuplicatePageParams{
		PageID: "page-source",
		UserID: "user-1",
	})
	if err != nil {
		t.Fatalf("Duplicate() error = %v", err)
	}
	if !pageRepo.createCalled {
		t.Fatal("Duplicate() should create a new page")
	}
	if sectionRepo.createCount != 2 {
		t.Fatalf("section create count = %d, want 2", sectionRepo.createCount)
	}
	if guard.organizationID != landingServiceOrganizationID ||
		guard.featureKey != landingdomain.FeatureLandingMaxSections ||
		guard.limitKey != "limit" ||
		guard.usedValue != 0 ||
		guard.delta != 2 {
		t.Fatalf("guard = %#v", guard)
	}
}

func TestPageServiceDuplicateStopsWhenSectionQuotaExceeded(t *testing.T) {
	pageRepo := &pageServicePageRepoStub{
		findByIDPage: landingdomain.LandingPage{
			ID:   "page-source",
			Name: "Source",
			Slug: "source-page",
		},
	}
	sectionRepo := &pageServiceSectionRepoStub{
		listByPageItems: []landingdomain.LandingSection{
			{ID: "section-1"},
		},
	}
	guardErr := errors.New("quota exceeded")
	service := NewPageService(
		pageRepo,
		sectionRepo,
		WithLandingPageQuotaGuard(&pageServiceQuotaGuardStub{err: guardErr}),
	)

	_, err := service.Duplicate(context.Background(), mustLandingScope(t), DuplicatePageParams{
		PageID: "page-source",
		UserID: "user-1",
	})
	if !errors.Is(err, guardErr) {
		t.Fatalf("Duplicate() error = %v, want %v", err, guardErr)
	}
	if pageRepo.createCalled {
		t.Fatal("Duplicate() should not create page when section quota guard fails")
	}
	if sectionRepo.createCount != 0 {
		t.Fatalf("section create count = %d, want 0", sectionRepo.createCount)
	}
}

func TestMapPagePersistenceErrorDuplicateSlug(t *testing.T) {
	err := mapPagePersistenceError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_landing_pages_organization_slug_active_unique",
	})

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "PAGE_SLUG_ALREADY_EXISTS" {
		t.Fatalf("expected PAGE_SLUG_ALREADY_EXISTS, got %s", appErr.Code)
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d", appErr.Status)
	}
}

func TestMapPagePersistenceErrorNoRows(t *testing.T) {
	err := mapPagePersistenceError(pgx.ErrNoRows)

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "PAGE_NOT_FOUND" {
		t.Fatalf("expected PAGE_NOT_FOUND, got %s", appErr.Code)
	}
	if appErr.Status != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", appErr.Status)
	}
}

const landingServiceOrganizationID = "11111111-1111-1111-1111-111111111111"

func mustLandingScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     landingServiceOrganizationID,
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}
