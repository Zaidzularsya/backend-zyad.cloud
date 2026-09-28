package service

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

const (
	leadDashboardDateLayout = "2006-01-02"
	// defaultLeadDashboardDays is the range used when from/to are omitted.
	defaultLeadDashboardDays = 30
	// Daily buckets beyond ~1 year are unreadable in a chart; monthly
	// buckets are allowed up to 2 years.
	maxLeadDashboardDailyDays   = 366
	maxLeadDashboardMonthlyDays = 731
	// autoMonthlyAfterDays: without an explicit granularity, ranges longer
	// than this are bucketed per month.
	autoMonthlyAfterDays = 92

	leadDashboardFollowUpLimit       = 8
	leadDashboardRecentActivityLimit = 10
)

var (
	ErrInvalidDashboardDate        = errors.New("from/to must be dates in YYYY-MM-DD format")
	ErrInvalidDashboardRange       = errors.New("from must not be after to")
	ErrDashboardRangeTooLong       = errors.New("date range is too long for the requested granularity (max 366 days per day, 731 days per month)")
	ErrInvalidDashboardGranularity = errors.New("granularity must be 'day' or 'month'")
)

// crm_* timestamps are written in the database timezone (Asia/Jakarta, no
// DST), so "today" is taken in WIB as well.
var leadDashboardLocation = time.FixedZone("WIB", 7*60*60)

type LeadDashboardQuery struct {
	From        string
	To          string
	Granularity string
}

type LeadDashboardService interface {
	Dashboard(context.Context, coretenant.Scope, LeadDashboardQuery) (domain.LeadDashboard, domain.LeadDashboardRange, error)
}

type leadDashboardService struct {
	repo repository.LeadDashboardRepository
	now  func() time.Time
}

func NewLeadDashboardService(repo repository.LeadDashboardRepository) LeadDashboardService {
	return &leadDashboardService{repo: repo, now: time.Now}
}

func (s *leadDashboardService) Dashboard(ctx context.Context, scope coretenant.Scope, q LeadDashboardQuery) (domain.LeadDashboard, domain.LeadDashboardRange, error) {
	rng, err := resolveLeadDashboardRange(q, s.now().In(leadDashboardLocation))
	if err != nil {
		return domain.LeadDashboard{}, domain.LeadDashboardRange{}, err
	}
	d, err := s.repo.Dashboard(ctx, scope, rng, repository.LeadDashboardLimits{
		FollowUps:      leadDashboardFollowUpLimit,
		RecentActivity: leadDashboardRecentActivityLimit,
	})
	if err != nil {
		return domain.LeadDashboard{}, domain.LeadDashboardRange{}, err
	}
	return d, rng, nil
}

// resolveLeadDashboardRange applies defaults and limits. Dates are calendar
// days (location-free, UTC midnight) and both ends are inclusive.
func resolveLeadDashboardRange(q LeadDashboardQuery, now time.Time) (domain.LeadDashboardRange, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	to := today
	if q.To != "" {
		parsed, err := time.Parse(leadDashboardDateLayout, q.To)
		if err != nil {
			return domain.LeadDashboardRange{}, ErrInvalidDashboardDate
		}
		to = parsed
	}
	from := to.AddDate(0, 0, -(defaultLeadDashboardDays - 1))
	if q.From != "" {
		parsed, err := time.Parse(leadDashboardDateLayout, q.From)
		if err != nil {
			return domain.LeadDashboardRange{}, ErrInvalidDashboardDate
		}
		from = parsed
	}
	if from.After(to) {
		return domain.LeadDashboardRange{}, ErrInvalidDashboardRange
	}

	days := int(to.Sub(from).Hours()/24) + 1

	granularity := domain.LeadDashboardGranularity(q.Granularity)
	switch {
	case q.Granularity == "":
		granularity = domain.LeadDashboardDaily
		if days > autoMonthlyAfterDays {
			granularity = domain.LeadDashboardMonthly
		}
	case !granularity.IsValid():
		return domain.LeadDashboardRange{}, ErrInvalidDashboardGranularity
	}

	maxDays := maxLeadDashboardDailyDays
	if granularity == domain.LeadDashboardMonthly {
		maxDays = maxLeadDashboardMonthlyDays
	}
	if days > maxDays {
		return domain.LeadDashboardRange{}, ErrDashboardRangeTooLong
	}

	prevTo := from.AddDate(0, 0, -1)
	return domain.LeadDashboardRange{
		From:         from,
		To:           to,
		PreviousFrom: prevTo.AddDate(0, 0, -(days - 1)),
		PreviousTo:   prevTo,
		Granularity:  granularity,
	}, nil
}

// IsLeadDashboardValidationError reports errors that map to HTTP 422.
func IsLeadDashboardValidationError(err error) bool {
	return errors.Is(err, ErrInvalidDashboardDate) ||
		errors.Is(err, ErrInvalidDashboardRange) ||
		errors.Is(err, ErrDashboardRangeTooLong) ||
		errors.Is(err, ErrInvalidDashboardGranularity)
}
