// Package businesstime holds the business timezone (Asia/Jakarta) used to
// decide which calendar day an instant belongs to.
//
// Timestamps are stored as UTC wall-clock in "timestamp without time zone"
// columns, so a "day" (dashboard ranges, daily buckets, per-day dedup) must
// be resolved here in Go instead of relying on the database session timezone.
package businesstime

import "time"

// Location is Asia/Jakarta. Indonesia has had no DST since 1964, so a fixed
// +07:00 zone is an exact fallback when the OS has no tzdata.
func Location() *time.Location {
	return location
}

var location = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// DayOf returns the business calendar day of t as a date at 00:00 UTC
// (a location-free date, comparable with dates parsed from YYYY-MM-DD).
func DayOf(t time.Time) time.Time {
	local := t.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// DayStartUTC converts a calendar date (only Y/M/D of date are used) to the
// UTC instant at which that business day starts.
func DayStartUTC(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location).UTC()
}
