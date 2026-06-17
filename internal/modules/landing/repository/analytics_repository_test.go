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

func TestAnalyticsRepositoryIntegration(t *testing.T) {
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
	analyticsRepo := repository.NewAnalyticsRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("analyticsrepoa"), ".", "-")

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

	sessionID := "sess-12345"

	// 1. Record Event
	err = analyticsRepo.RecordEvent(ctx, tenants.A.Scope, repository.CreateAnalyticsEventParams{
		LandingPageID: pageA.ID,
		Version:       1,
		EventName:     domain.AnalyticsEventPageView,
		SessionID:     &sessionID,
	})
	if err != nil {
		t.Fatalf("Record event: %v", err)
	}

	// 2. Tenant B cannot list Tenant A's events
	events, err := analyticsRepo.ListEvents(ctx, tenants.B.Scope, repository.AnalyticsFilter{
		LandingPageID: pageA.ID,
	})
	if err != nil {
		t.Fatalf("List events Tenant B: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events for Tenant B, got %d", len(events))
	}

	// 3. Upsert Daily Stats
	dateStr := time.Now().Format("2006-01-02")
	err = analyticsRepo.UpsertDailyStats(ctx, tenants.A.Scope, repository.RecordDailyStatsParams{
		LandingPageID:  pageA.ID,
		Date:           dateStr,
		PageViews:      1,
		UniqueVisitors: 1,
	})
	if err != nil {
		t.Fatalf("Upsert daily stats: %v", err)
	}

	// 4. Upsert again to test increment
	err = analyticsRepo.UpsertDailyStats(ctx, tenants.A.Scope, repository.RecordDailyStatsParams{
		LandingPageID:  pageA.ID,
		Date:           dateStr,
		PageViews:      2, // increment by 2
		UniqueVisitors: 0,
	})
	if err != nil {
		t.Fatalf("Upsert daily stats increment: %v", err)
	}

	stats, err := analyticsRepo.ListDailyStats(ctx, tenants.A.Scope, pageA.ID, dateStr, dateStr)
	if err != nil {
		t.Fatalf("List daily stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("Expected 1 daily stat record, got %d", len(stats))
	}
	if stats[0].PageViews != 3 {
		t.Errorf("Expected 3 page views, got %d", stats[0].PageViews)
	}
	if stats[0].UniqueVisitors != 1 {
		t.Errorf("Expected 1 unique visitor, got %d", stats[0].UniqueVisitors)
	}
}
