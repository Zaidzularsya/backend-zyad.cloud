//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSubmissionRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active'),
		('22222222-2222-2222-2222-222222222222', 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)
	subRepo := repository.NewSubmissionRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("subrepoa"), ".", "-")

	// Create Page
	pageA, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Page A",
		Title:      "Campaign A",
		Slug:       slugA,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant A: %v", err)
	}

	// Create Form
	formA, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: pageA.ID,
		Name:          "Contact Us",
		Key:           "contact-form-1",
		IsActive:      true,
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create form in page A: %v", err)
	}

	// 1. Create a submission
	subA, err := subRepo.Create(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: pageA.ID,
		FormID:        formA.ID,
		Reference:     "SUB-12345",
		Status:        domain.SubmissionStatusNew,
		SubmittedData: map[string]any{"email": "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Create submission: %v", err)
	}

	// 2. Tenant B cannot read Tenant A's submission
	_, err = subRepo.FindByID(ctx, tenants.B.Scope, subA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's submission, got success")
	}

	// 3. Update Submission Status
	newStatus := domain.SubmissionStatusContacted
	updatedSub, err := subRepo.Update(ctx, tenants.A.Scope, subA.ID, repository.UpdateSubmissionParams{
		Status: &newStatus,
	})
	if err != nil {
		t.Fatalf("Update submission: %v", err)
	}
	if updatedSub.Status != domain.SubmissionStatusContacted {
		t.Errorf("Expected status 'contacted', got %v", updatedSub.Status)
	}

	// 4. Create Note
	err = subRepo.CreateNote(ctx, tenants.A.Scope, repository.CreateSubmissionNoteParams{
		SubmissionID: subA.ID,
		Note:         "Followed up via email",
		CreatedBy:    "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create submission note: %v", err)
	}

	// 5. Delete Submission
	err = subRepo.Delete(ctx, tenants.A.Scope, subA.ID)
	if err != nil {
		t.Fatalf("Delete submission: %v", err)
	}

	_, err = subRepo.FindByID(ctx, tenants.A.Scope, subA.ID)
	if err == nil {
		t.Error("Expected submission to be soft deleted, got success")
	}
}

type submissionIsolationAdapter struct {
	repo       repository.SubmissionRepository
	pageRepo   repository.PageRepository
	formRepo   repository.FormRepository
	ids        map[string]string
	scopePages map[string]string
	scopeForms map[string]string
}

func (a *submissionIsolationAdapter) ensurePageAndForm(ctx context.Context, scope coretenant.Scope) (string, string, error) {
	pageID, ok := a.scopePages[scope.OrganizationID()]
	if ok {
		return pageID, a.scopeForms[scope.OrganizationID()], nil
	}
	page, err := a.pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       "Test Page " + scope.OrganizationID(),
		Title:      "Test",
		Slug:       strings.ReplaceAll("testpage-"+testutil.UniqueCode("sub")+scope.OrganizationID()[:8], ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
	})
	if err != nil {
		return "", "", err
	}
	a.scopePages[scope.OrganizationID()] = page.ID

	form, err := a.formRepo.Create(ctx, scope, repository.CreateFormParams{
		LandingPageID: page.ID,
		Name:          "Test Form",
		Key:           strings.ReplaceAll("testform-"+testutil.UniqueCode("sub")+scope.OrganizationID()[:8], ".", "-"),
		IsActive:      true,
	})
	if err != nil {
		return "", "", err
	}
	a.scopeForms[scope.OrganizationID()] = form.ID

	return page.ID, form.ID, nil
}

func (a *submissionIsolationAdapter) Create(ctx context.Context, tctx coretenant.Context, key string, value string) error {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return err
	}
	pageID, formID, err := a.ensurePageAndForm(ctx, scope)
	if err != nil {
		return err
	}
	sub, err := a.repo.Create(ctx, scope, repository.CreateSubmissionParams{
		LandingPageID: pageID,
		FormID:        formID,
		Reference:     value,
		Status:        domain.SubmissionStatusNew,
		SubmittedData: map[string]any{"key": key},
	})
	if err != nil {
		return err
	}
	a.ids[key] = sub.ID
	return nil
}

func (a *submissionIsolationAdapter) Read(ctx context.Context, tctx coretenant.Context, key string) (string, bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return "", false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return "", false, nil
	}
	sub, err := a.repo.FindByID(ctx, scope, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return "", false, nil
		}
		return "", false, err
	}
	return sub.Reference, true, nil
}

func (a *submissionIsolationAdapter) List(ctx context.Context, tctx coretenant.Context) ([]string, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return nil, err
	}
	_, _, err = a.ensurePageAndForm(ctx, scope)
	if err != nil {
		return nil, err
	}
	subs, err := a.repo.List(ctx, scope, repository.SubmissionFilter{})
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, s := range subs {
		for k, id := range a.ids {
			if id == s.ID {
				keys = append(keys, k)
				break
			}
		}
	}
	return keys, nil
}

func (a *submissionIsolationAdapter) Update(ctx context.Context, tctx coretenant.Context, key string, value string) (bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return false, nil
	}
	status := domain.SubmissionStatus(value)
	if status != domain.SubmissionStatusContacted && status != domain.SubmissionStatusNew {
		status = domain.SubmissionStatusContacted
	}
	_, err = a.repo.Update(ctx, scope, id, repository.UpdateSubmissionParams{
		Status: &status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *submissionIsolationAdapter) Delete(ctx context.Context, tctx coretenant.Context, key string) (bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return false, nil
	}
	err = a.repo.Delete(ctx, scope, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *submissionIsolationAdapter) BulkUpdate(ctx context.Context, tctx coretenant.Context, keys []string, value string) (int64, error) {
	var count int64
	for _, k := range keys {
		updated, err := a.Update(ctx, tctx, k, value)
		if err != nil {
			return count, err
		}
		if updated {
			count++
		}
	}
	return count, nil
}

func (a *submissionIsolationAdapter) Export(ctx context.Context, tctx coretenant.Context) ([]string, error) {
	return a.List(ctx, tctx)
}

func TestSubmissionTenantIsolationSuite(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := repository.NewSubmissionRepository(db)
	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)
	adapter := &submissionIsolationAdapter{
		repo:       repo,
		pageRepo:   pageRepo,
		formRepo:   formRepo,
		ids:        make(map[string]string),
		scopePages: make(map[string]string),
		scopeForms: make(map[string]string),
	}
	testutil.RunTenantIsolationSuite(t, adapter)
}
