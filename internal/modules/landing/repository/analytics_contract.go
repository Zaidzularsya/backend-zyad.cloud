package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateAnalyticsEventParams struct {
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
	IPAddressHash *string
}

type RecordDailyStatsParams struct {
	LandingPageID  string
	Date           string // format YYYY-MM-DD
	PageViews      int
	UniqueVisitors int
	CTAClicks      int
	FormStarts     int
	Submissions    int
}

type AnalyticsFilter struct {
	LandingPageID string
	EventName     domain.AnalyticsEventName
	StartDate     string
	EndDate       string
	Limit         int
	Offset        int
}

type AnalyticsRepository interface {
	// Event Operations
	RecordEvent(context.Context, coretenant.Scope, CreateAnalyticsEventParams) error
	ListEvents(context.Context, coretenant.Scope, AnalyticsFilter) ([]domain.LandingAnalyticsEvent, error)
	GetEventCount(context.Context, coretenant.Scope, AnalyticsFilter) (int64, error)

	// Daily Aggregation Operations
	UpsertDailyStats(context.Context, coretenant.Scope, RecordDailyStatsParams) error
	ListDailyStats(context.Context, coretenant.Scope, string, string, string) ([]domain.LandingAnalyticsDaily, error) // pageID, startDate, endDate
}
