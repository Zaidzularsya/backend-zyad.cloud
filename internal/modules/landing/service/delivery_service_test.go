//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestDeliveryServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	integrationRepo := repository.NewIntegrationRepository(db)
	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)
	submissionRepo := repository.NewSubmissionRepository(db)

	deliveryService := service.NewDeliveryService(integrationRepo, submissionRepo)

	userID := "11111111-1111-1111-1111-111111111111"

	// Mock Webhook Server
	var receivedPayload map[string]any
	var receivedSignature string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Signature")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Pre-requisites: Page and Form
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Test Page",
		Title:      "Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("del-page-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	form, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: page.ID,
		Name:          "Lead Form",
		Key:           "lead-form",
		IsActive:      true,
		CreatedBy:     userID,
	})
	if err != nil {
		t.Fatalf("CreateForm: %v", err)
	}

	submission, err := submissionRepo.Create(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: page.ID,
		FormID:        form.ID,
		Status:        domain.SubmissionStatusNew,
		SubmittedData: map[string]any{"email": "test@example.com"},
		SourceURL:     "http://example.com",
	})
	if err != nil {
		t.Fatalf("CreateSubmission: %v", err)
	}

	var integration domain.LandingLeadIntegration

	t.Run("Create Integration", func(t *testing.T) {
		params := repository.CreateIntegrationParams{
			Name: "Test Webhook",
			Type: domain.IntegrationTypeWebhook,
			Credentials: map[string]any{
				"url":    mockServer.URL,
				"secret": "my-secret-key",
			},
			EventFilters: []any{},
			IsActive:     true,
			CreatedBy:    userID,
		}

		created, err := deliveryService.CreateIntegration(ctx, tenants.A.Scope, params)
		if err != nil {
			t.Fatalf("CreateIntegration: %v", err)
		}
		integration = created
	})

	t.Run("Dispatch Form Submission", func(t *testing.T) {
		err := deliveryService.DispatchFormSubmission(ctx, tenants.A.Scope, submission.ID)
		if err != nil {
			t.Fatalf("DispatchFormSubmission: %v", err)
		}

		// Verify delivery log is created
		logs, err := integrationRepo.ListDeliveryLogs(ctx, tenants.A.Scope, submission.ID)
		if err != nil {
			t.Fatalf("ListDeliveryLogs: %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected 1 delivery log, got %d", len(logs))
		}
		if logs[0].Status != domain.DeliveryStatusPending {
			t.Errorf("expected status pending, got %s", logs[0].Status)
		}
	})

	t.Run("Process Pending Deliveries", func(t *testing.T) {
		err := deliveryService.ProcessPendingDeliveries(ctx, tenants.A.Scope, 10)
		if err != nil {
			t.Fatalf("ProcessPendingDeliveries: %v", err)
		}

		// Allow some time for HTTP request to be processed by httptest server if it was async,
		// but ProcessPendingDeliveries runs it synchronously in this mock.
		time.Sleep(100 * time.Millisecond)

		if receivedSignature == "" {
			t.Errorf("expected signature to be sent")
		}
		if receivedPayload["ID"] != submission.ID {
			t.Errorf("expected payload to contain submission ID")
		}

		// Verify log status updated to success
		logs, err := integrationRepo.ListDeliveryLogs(ctx, tenants.A.Scope, submission.ID)
		if err != nil {
			t.Fatalf("ListDeliveryLogs: %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected 1 delivery log, got %d", len(logs))
		}
		if logs[0].Status != domain.DeliveryStatusSuccess {
			t.Errorf("expected status success, got %s", logs[0].Status)
		}
	})

	t.Run("Delete Integration", func(t *testing.T) {
		err := deliveryService.DeleteIntegration(ctx, tenants.A.Scope, integration.ID)
		if err != nil {
			t.Fatalf("DeleteIntegration: %v", err)
		}
	})
}
