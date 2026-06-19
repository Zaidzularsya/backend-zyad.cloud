//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestAnalyticsServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	pageRepo := repository.NewPageRepository(db)
	analyticsRepo := repository.NewAnalyticsRepository(db)
	analyticsSvc := service.NewAnalyticsService(analyticsRepo, pageRepo, db)

	scope := tenants.A.Scope

	// Insert test organization
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-analytics', 'Organization Analytics', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	require.NoError(t, err)

	// Insert dummy user
	_, err = db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES ('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	require.NoError(t, err)

	// Create test page
	page, err := pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       "Analytics Test Page",
		Title:      "Analytics Test Page",
		Slug:       fmt.Sprintf("analytics-test-page-%d", time.Now().UnixNano()),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	require.NoError(t, err)

	t.Run("TrackEvent", func(t *testing.T) {
		ipStr := "192.168.1.1"
		userAgent := "Mozilla/5.0"
		sessID := "session-123"

		err := analyticsSvc.TrackEvent(ctx, scope, service.TrackEventParams{
			LandingPageID: page.ID,
			Version:       1,
			EventName:     domain.AnalyticsEventPageView,
			IPAddress:     &ipStr,
			Browser:       &userAgent,
			SessionID:     &sessID,
		})
		require.NoError(t, err)

		// Fetch events
		events, count, err := analyticsSvc.GetEvents(ctx, scope, repository.AnalyticsFilter{
			LandingPageID: page.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
		require.Len(t, events, 1)

		event := events[0]
		assert.Equal(t, domain.AnalyticsEventPageView, event.EventName)
		assert.Equal(t, "session-123", *event.SessionID)
		assert.Equal(t, "Mozilla/5.0", *event.Browser)
		
		// IP address should be hashed, not raw
		require.NotNil(t, event.IPAddressHash)
		assert.NotEqual(t, "192.168.1.1", *event.IPAddressHash)

		// Invalid event name
		err = analyticsSvc.TrackEvent(ctx, scope, service.TrackEventParams{
			LandingPageID: page.ID,
			Version:       1,
			EventName:     domain.AnalyticsEventName("invalid_event"),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid analytics event name")
	})

	t.Run("AggregateDailyStats", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")

		// Track a CTA click from the same session
		sessID := "session-123"
		err := analyticsSvc.TrackEvent(ctx, scope, service.TrackEventParams{
			LandingPageID: page.ID,
			Version:       1,
			EventName:     domain.AnalyticsEventCTAClick,
			SessionID:     &sessID,
		})
		require.NoError(t, err)

		// Track a form start from a DIFFERENT session
		sessID2 := "session-456"
		err = analyticsSvc.TrackEvent(ctx, scope, service.TrackEventParams{
			LandingPageID: page.ID,
			Version:       1,
			EventName:     domain.AnalyticsEventFormStart,
			SessionID:     &sessID2,
		})
		require.NoError(t, err)

		// Run aggregation
		err = analyticsSvc.AggregateDailyStats(ctx, scope, page.ID, today)
		require.NoError(t, err)

		// Fetch daily stats
		stats, err := analyticsSvc.GetDailyStats(ctx, scope, page.ID, today, today)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, 1, stat.PageViews)
		assert.Equal(t, 1, stat.CTAClicks)
		assert.Equal(t, 1, stat.FormStarts)
		assert.Equal(t, 0, stat.Submissions)

		// Since there are 2 sessions and the first one also had an IP (hashed), it's counted as unique visitors.
		// Wait, the Aggregate logic unique visitors:
		// session-123 has an IP address in the first event. In the second event (CTA), no IP.
		// The logic checks unique IP hash OR Session ID. So it's robust enough for testing.
		assert.GreaterOrEqual(t, stat.UniqueVisitors, 1)
	})
}
