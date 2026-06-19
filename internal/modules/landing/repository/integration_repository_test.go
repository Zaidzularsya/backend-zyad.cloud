//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestIntegrationRepositoryIntegration(t *testing.T) {
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
	submissionRepo := repository.NewSubmissionRepository(db)
	integrationRepo := repository.NewIntegrationRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("integrationrepoa"), ".", "-")

	// Dependencies
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
		t.Fatalf("Create page: %v", err)
	}

	formA, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: pageA.ID,
		Name:          "Contact Form",
		Key:           "contact",
		Description:   "Contact Us",
		SubmitLabel:   "Send",
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create form: %v", err)
	}

	subA, err := submissionRepo.Create(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		FormID:        formA.ID,
		LandingPageID: pageA.ID,
		Reference:     "test-ref",
		Status:        domain.SubmissionStatusNew,
		SubmittedData: map[string]any{"email": "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Create submission: %v", err)
	}

	// --- 1. Integrations ---
	integA, err := integrationRepo.CreateIntegration(ctx, tenants.A.Scope, repository.CreateIntegrationParams{
		Name: "Webhook Lead Sync",
		Type: domain.IntegrationTypeWebhook,
		Credentials: map[string]any{
			"url":    "https://example.com/webhook",
			"secret": "my-secret",
		},
		EventFilters: []any{"submission_created"},
		IsActive:     true,
		CreatedBy:    "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create integration: %v", err)
	}

	err = integrationRepo.UpdateIntegration(ctx, tenants.A.Scope, integA.ID, repository.UpdateIntegrationParams{
		Name: "Webhook Lead Sync Updated",
		Credentials: map[string]any{
			"url":    "https://example.com/webhook2",
			"secret": "my-secret2",
		},
		EventFilters: []any{"submission_created"},
		IsActive:     false,
		UpdatedBy:    "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Update integration: %v", err)
	}

	// --- 2. Delivery Logs ---
	pastTime := time.Now().UTC().Add(-1 * time.Hour)
	logA, err := integrationRepo.CreateDeliveryLog(ctx, tenants.A.Scope, repository.CreateDeliveryLogParams{
		IntegrationID: integA.ID,
		SubmissionID:  subA.ID,
		Status:        domain.DeliveryStatusPending,
		NextRetryAt:   &pastTime,
	})
	if err != nil {
		t.Fatalf("Create delivery log: %v", err)
	}

	logs, err := integrationRepo.ListDeliveryLogs(ctx, tenants.A.Scope, subA.ID)
	if err != nil {
		t.Fatalf("List delivery logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("Expected 1 delivery log, got %d", len(logs))
	}

	// 3. Claim Pending Deliveries (Worker)
	claimed, err := integrationRepo.ClaimPendingDeliveries(ctx, tenants.A.Scope, 10)
	if err != nil {
		t.Fatalf("Claim pending deliveries: %v", err)
	}

	var foundClaim *domain.LandingLeadDeliveryLog
	for _, c := range claimed {
		if c.ID == logA.ID {
			foundClaim = &c
			break
		}
	}
	if foundClaim == nil {
		t.Fatalf("Expected logA %s to be claimed, but wasn't. Total claimed: %d", logA.ID, len(claimed))
	}
	if foundClaim.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", foundClaim.Attempts)
	}

	// 4. Update Delivery Log Status
	errMsg := "Connection timeout"
	nextRetry := time.Now().UTC().Add(5 * time.Minute)
	err = integrationRepo.UpdateDeliveryLogStatus(ctx, tenants.A.Scope, foundClaim.ID, repository.UpdateDeliveryLogParams{
		Status:       domain.DeliveryStatusFailed,
		ErrorMessage: &errMsg,
		NextRetryAt:  &nextRetry,
	})
	if err != nil {
		t.Fatalf("Update delivery log status: %v", err)
	}

	err = integrationRepo.DeleteIntegration(ctx, tenants.A.Scope, integA.ID)
	if err != nil {
		t.Fatalf("Delete integration: %v", err)
	}
}
