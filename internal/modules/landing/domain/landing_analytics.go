package domain

import "time"

type AnalyticsEventName string

const (
	AnalyticsEventPageView   AnalyticsEventName = "page_view"
	AnalyticsEventCTAClick   AnalyticsEventName = "cta_click"
	AnalyticsEventFormView   AnalyticsEventName = "form_view"
	AnalyticsEventFormStart  AnalyticsEventName = "form_start"
	AnalyticsEventSubmission AnalyticsEventName = "submission"
)

func (e AnalyticsEventName) IsValid() bool {
	switch e {
	case AnalyticsEventPageView, AnalyticsEventCTAClick, AnalyticsEventFormView, AnalyticsEventFormStart, AnalyticsEventSubmission:
		return true
	default:
		return false
	}
}

type LandingAnalyticsEvent struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	Version        int
	EventName      AnalyticsEventName
	SectionKey     *string
	TargetKey      *string
	SessionID      *string
	UTMSource      *string
	UTMMedium      *string
	UTMCampaign    *string
	UTMTerm        *string
	UTMContent     *string
	Referrer       *string
	Browser        *string
	Device         *string
	OS             *string
	IPAddressHash  *string
	CreatedAt      time.Time
}

type LandingAnalyticsDaily struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	Date           time.Time
	PageViews      int
	UniqueVisitors int
	CTAClicks      int
	FormStarts     int
	Submissions    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
