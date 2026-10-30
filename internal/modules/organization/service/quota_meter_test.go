package service

import (
	"testing"
	"time"

	"zyad.cloud/internal/core/businesstime"
)

func TestPeriodStartMonthlyUsesBusinessTimezone(t *testing.T) {
	loc := businesstime.Location()

	got, err := PeriodStart("monthly", time.Date(2026, 10, 31, 23, 59, 0, 0, loc))
	if err != nil || got == nil {
		t.Fatalf("PeriodStart = %v, %v", got, err)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// 2026-11-01 00:00 WIB is still Oct 31 17:00 UTC: a UTC-based month would be wrong.
	got, err = PeriodStart("monthly", time.Date(2026, 11, 1, 0, 0, 0, 0, loc).UTC())
	if err != nil || got == nil {
		t.Fatalf("PeriodStart = %v, %v", got, err)
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPeriodStartNeverIsNil(t *testing.T) {
	got, err := PeriodStart("never", time.Now())
	if err != nil || got != nil {
		t.Fatalf("PeriodStart = %v, %v; want nil, nil", got, err)
	}
}

func TestPeriodStartUnsupportedStrategy(t *testing.T) {
	for _, s := range []string{"yearly", "custom", ""} {
		if _, err := PeriodStart(s, time.Now()); err == nil {
			t.Fatalf("strategy %q: want error", s)
		}
	}
}

func TestQuotaMeterRegistryGet(t *testing.T) {
	r := NewQuotaMeterRegistry()
	if _, ok := r.Get("crm.max_leads"); ok {
		t.Fatal("unexpected meter")
	}
	r.Register("crm.max_leads", fakeMeter{used: 3})
	m, ok := r.Get("crm.max_leads")
	if !ok || m == nil {
		t.Fatal("meter not found")
	}
}
