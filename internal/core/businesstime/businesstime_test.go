package businesstime

import (
	"testing"
	"time"
)

func TestDayStartUTCConvertsBusinessMidnightToUTC(t *testing.T) {
	// A calendar date carries only Y/M/D (any location); 00:00 WIB is 17:00Z
	// of the previous day.
	got := DayStartUTC(time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC))
	want := time.Date(2026, 1, 10, 17, 0, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("DayStartUTC = %s (%s), want %s UTC", got, got.Location(), want)
	}
}

func TestDayOfSplitsBusinessDaysAtWIBMidnight(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"23:30 WIB stays on the same day", time.Date(2026, 1, 10, 16, 30, 0, 0, time.UTC), "2026-01-10"},
		{"00:30 WIB is the next day", time.Date(2026, 1, 10, 17, 30, 0, 0, time.UTC), "2026-01-11"},
		{"input in another zone", time.Date(2026, 1, 10, 23, 30, 0, 0, time.FixedZone("WIB", 7*3600)), "2026-01-10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DayOf(tc.at).Format("2006-01-02"); got != tc.want {
				t.Fatalf("DayOf(%s) = %s, want %s", tc.at, got, tc.want)
			}
		})
	}
}
