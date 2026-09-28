package service

import (
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/modules/crm/domain"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolveLeadDashboardRangeDefaults(t *testing.T) {
	// 23:30 UTC on 27 Sep is already 28 Sep in WIB.
	now := time.Date(2026, 9, 27, 23, 30, 0, 0, time.UTC).In(leadDashboardLocation)

	rng, err := resolveLeadDashboardRange(LeadDashboardQuery{}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rng.To.Equal(mustDate(t, "2026-09-28")) || !rng.From.Equal(mustDate(t, "2026-08-30")) {
		t.Errorf("range = %s..%s, want 2026-08-30..2026-09-28", rng.From.Format("2006-01-02"), rng.To.Format("2006-01-02"))
	}
	if !rng.PreviousTo.Equal(mustDate(t, "2026-08-29")) || !rng.PreviousFrom.Equal(mustDate(t, "2026-07-31")) {
		t.Errorf("previous = %s..%s, want 2026-07-31..2026-08-29", rng.PreviousFrom.Format("2006-01-02"), rng.PreviousTo.Format("2006-01-02"))
	}
	if rng.Granularity != domain.LeadDashboardDaily {
		t.Errorf("granularity = %s, want day", rng.Granularity)
	}
}

func TestResolveLeadDashboardRangeAutoMonthly(t *testing.T) {
	rng, err := resolveLeadDashboardRange(LeadDashboardQuery{From: "2025-10-01", To: "2026-09-28"}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rng.Granularity != domain.LeadDashboardMonthly {
		t.Errorf("granularity = %s, want month", rng.Granularity)
	}
}

func TestResolveLeadDashboardRangeSingleDay(t *testing.T) {
	rng, err := resolveLeadDashboardRange(LeadDashboardQuery{From: "2026-09-28", To: "2026-09-28"}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rng.PreviousFrom.Equal(mustDate(t, "2026-09-27")) || !rng.PreviousTo.Equal(mustDate(t, "2026-09-27")) {
		t.Errorf("previous = %s..%s, want 2026-09-27", rng.PreviousFrom.Format("2006-01-02"), rng.PreviousTo.Format("2006-01-02"))
	}
}

func TestResolveLeadDashboardRangeValidation(t *testing.T) {
	cases := []struct {
		name string
		q    LeadDashboardQuery
		want error
	}{
		{"bad date", LeadDashboardQuery{From: "28-09-2026"}, ErrInvalidDashboardDate},
		{"from after to", LeadDashboardQuery{From: "2026-09-10", To: "2026-09-01"}, ErrInvalidDashboardRange},
		{"bad granularity", LeadDashboardQuery{Granularity: "week"}, ErrInvalidDashboardGranularity},
		{"daily too long", LeadDashboardQuery{From: "2025-01-01", To: "2026-09-28", Granularity: "day"}, ErrDashboardRangeTooLong},
		{"monthly too long", LeadDashboardQuery{From: "2023-01-01", To: "2026-09-28"}, ErrDashboardRangeTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveLeadDashboardRange(tc.q, time.Now())
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !IsLeadDashboardValidationError(err) {
				t.Error("expected validation error")
			}
		})
	}
}
