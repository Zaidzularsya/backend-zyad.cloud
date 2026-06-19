package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database"
)

type defaultAnalyticsService struct {
	repo     repository.AnalyticsRepository
	pageRepo repository.PageRepository
	db       *database.Pool
}

func NewAnalyticsService(
	repo repository.AnalyticsRepository,
	pageRepo repository.PageRepository,
	db *database.Pool,
) AnalyticsService {
	return &defaultAnalyticsService{
		repo:     repo,
		pageRepo: pageRepo,
		db:       db,
	}
}

func (s *defaultAnalyticsService) hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	// Hash IP with a salt or simply hash it to preserve privacy
	hash := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(hash[:])
}

func (s *defaultAnalyticsService) TrackEvent(ctx context.Context, scope coretenant.Scope, params TrackEventParams) error {
	if !params.EventName.IsValid() {
		return errors.New("invalid analytics event name")
	}

	// Validate page exists
	_, err := s.pageRepo.FindByID(ctx, scope, params.LandingPageID)
	if err != nil {
		return fmt.Errorf("verify page exists: %w", err)
	}

	// Hash IP address for privacy (GDPR compliance)
	var ipHash *string
	if params.IPAddress != nil && *params.IPAddress != "" {
		h := s.hashIP(*params.IPAddress)
		ipHash = &h
	}

	repoParams := repository.CreateAnalyticsEventParams{
		LandingPageID: params.LandingPageID,
		Version:       params.Version,
		EventName:     params.EventName,
		SectionKey:    params.SectionKey,
		TargetKey:     params.TargetKey,
		SessionID:     params.SessionID,
		UTMSource:     params.UTMSource,
		UTMMedium:     params.UTMMedium,
		UTMCampaign:   params.UTMCampaign,
		UTMTerm:       params.UTMTerm,
		UTMContent:    params.UTMContent,
		Referrer:      params.Referrer,
		Browser:       params.Browser,
		Device:        params.Device,
		OS:            params.OS,
		IPAddressHash: ipHash,
	}

	return s.repo.RecordEvent(ctx, scope, repoParams)
}

func (s *defaultAnalyticsService) AggregateDailyStats(ctx context.Context, scope coretenant.Scope, pageID string, date string) error {
	// Parse date
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return errors.New("invalid date format, must be YYYY-MM-DD")
	}

	// Define start and end of the day for querying events
	startDate := t.Format("2006-01-02")
	endDate := t.AddDate(0, 0, 1).Format("2006-01-02")

	// Get events for the day
	filter := repository.AnalyticsFilter{
		LandingPageID: pageID,
		StartDate:     startDate,
		EndDate:       endDate,
		Limit:         1000000, // Fetch all for the day
		Offset:        0,
	}

	events, err := s.repo.ListEvents(ctx, scope, filter)
	if err != nil {
		return fmt.Errorf("list events for aggregation: %w", err)
	}

	// Aggregate metrics
	pageViews := 0
	ctaClicks := 0
	formStarts := 0
	submissions := 0
	
	uniqueVisitorsMap := make(map[string]bool)

	for _, e := range events {
		switch e.EventName {
		case domain.AnalyticsEventPageView:
			pageViews++
		case domain.AnalyticsEventCTAClick:
			ctaClicks++
		case domain.AnalyticsEventFormStart:
			formStarts++
		case domain.AnalyticsEventSubmission:
			submissions++
		}

		if e.IPAddressHash != nil && *e.IPAddressHash != "" {
			uniqueVisitorsMap[*e.IPAddressHash] = true
		} else if e.SessionID != nil && *e.SessionID != "" {
			uniqueVisitorsMap[*e.SessionID] = true
		}
	}

	statsParams := repository.RecordDailyStatsParams{
		LandingPageID:  pageID,
		Date:           date,
		PageViews:      pageViews,
		UniqueVisitors: len(uniqueVisitorsMap),
		CTAClicks:      ctaClicks,
		FormStarts:     formStarts,
		Submissions:    submissions,
	}

	return s.repo.UpsertDailyStats(ctx, scope, statsParams)
}

func (s *defaultAnalyticsService) GetEvents(ctx context.Context, scope coretenant.Scope, filter repository.AnalyticsFilter) ([]domain.LandingAnalyticsEvent, int64, error) {
	events, err := s.repo.ListEvents(ctx, scope, filter)
	if err != nil {
		return nil, 0, err
	}

	count, err := s.repo.GetEventCount(ctx, scope, filter)
	if err != nil {
		return nil, 0, err
	}

	return events, count, nil
}

func (s *defaultAnalyticsService) GetDailyStats(ctx context.Context, scope coretenant.Scope, pageID string, startDate string, endDate string) ([]domain.LandingAnalyticsDaily, error) {
	return s.repo.ListDailyStats(ctx, scope, pageID, startDate, endDate)
}
