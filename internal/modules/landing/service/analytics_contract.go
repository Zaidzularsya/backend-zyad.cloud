package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// TrackEventParams holds data to track an interaction.
type TrackEventParams struct {
	LandingPageID string
	Version       int
	EventName     domain.AnalyticsEventName
	SectionKey    *string
	TargetKey     *string
	SessionID     *string
	UTMSource     *string
	UTMMedium     *string
	UTMCampaign   *string
	UTMTerm       *string
	UTMContent    *string
	Referrer      *string
	Browser       *string
	Device        *string
	OS            *string
	IPAddress     *string // Will be hashed inside the service for GDPR compliance
}

// AnalyticsService defines business logic for recording and aggregating analytics.
type AnalyticsService interface {
	// TrackEvent records a single public interaction event.
	TrackEvent(ctx context.Context, scope coretenant.Scope, params TrackEventParams) error

	// AggregateDailyStats performs the aggregation job for a specific date and page.
	AggregateDailyStats(ctx context.Context, scope coretenant.Scope, pageID string, date string) error

	// GetEvents returns raw event logs.
	GetEvents(ctx context.Context, scope coretenant.Scope, filter repository.AnalyticsFilter) ([]domain.LandingAnalyticsEvent, int64, error)

	// GetDailyStats returns aggregated statistics.
	GetDailyStats(ctx context.Context, scope coretenant.Scope, pageID string, startDate string, endDate string) ([]domain.LandingAnalyticsDaily, error)
}
