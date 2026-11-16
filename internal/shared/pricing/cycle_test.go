package pricing

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCurrentCycleStart(t *testing.T) {
	cases := []struct {
		billingDay  int
		today, want string
	}{
		{31, "2026-02-15", "2026-01-31"},
		{31, "2026-03-01", "2026-02-28"},
		{31, "2028-03-01", "2028-02-29"}, // leap year
		{31, "2026-03-31", "2026-03-31"},
		{5, "2026-11-12", "2026-11-05"},
		{5, "2026-11-05", "2026-11-05"}, // billing day itself is the cycle start
		{25, "2026-11-12", "2026-10-25"},
		{25, "2027-01-10", "2026-12-25"}, // crosses the year boundary
		{1, "2026-01-01", "2026-01-01"},
		{30, "2026-03-29", "2026-02-28"},
	}
	for _, c := range cases {
		// The time of day must not matter.
		got := CurrentCycleStart(c.billingDay, day(c.today).Add(15*time.Hour))
		if !got.Equal(day(c.want)) {
			t.Errorf("CurrentCycleStart(%d, %s) = %s, want %s", c.billingDay, c.today, got.Format("2006-01-02"), c.want)
		}
	}
}

func TestFirstPeriodIndex(t *testing.T) {
	today := day("2026-11-12")
	cases := []struct {
		name   string
		timing PaymentTiming
		start  string
		lead   int
		want   int
	}{
		{"prepaid, cycle already started", Prepaid, "2026-11-05", 7, 1},
		{"prepaid, start is today", Prepaid, "2026-11-12", 0, 1},
		{"prepaid, issue moment still ahead", Prepaid, "2026-11-25", 7, 0},
		{"prepaid, issue moment is today", Prepaid, "2026-11-19", 7, 1},
		{"postpaid, period 0 still running", Postpaid, "2026-10-25", 7, 0},
		{"postpaid, period 0 ended yesterday", Postpaid, "2026-10-12", 7, 1},
		{"postpaid, period 0 ends today", Postpaid, "2026-10-13", 7, 0},
	}
	for _, c := range cases {
		if got := FirstPeriodIndex(c.timing, Monthly, day(c.start), today, c.lead); got != c.want {
			t.Errorf("%s: FirstPeriodIndex = %d, want %d", c.name, got, c.want)
		}
	}
}
