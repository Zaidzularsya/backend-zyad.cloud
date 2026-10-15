// Package playbook holds the pure (database-free) rules of the CRM lead
// playbook: business-hours due dates and outcome transitions.
package playbook

import (
	"strconv"
	"strings"
	"time"
)

// BusinessLocation is WIB. Indonesia has no DST, so a fixed zone avoids a
// tzdata dependency on the server.
var BusinessLocation = time.FixedZone("WIB", 7*60*60)

const (
	workStartHour = 8
	workEndHour   = 17
)

func isWorkday(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func atClock(day time.Time, hour, minute int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, BusinessLocation)
}

// nextWorkdayStart returns 08:00 of the first workday strictly after day.
func nextWorkdayStart(day time.Time) time.Time {
	d := atClock(day, workStartHour, 0).AddDate(0, 0, 1)
	for !isWorkday(d) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// normalize moves t into working hours: before 08:00 on a workday → 08:00;
// at/after 17:00 or on a weekend → 08:00 next workday.
func normalize(t time.Time) time.Time {
	if !isWorkday(t) {
		return nextWorkdayStart(t)
	}
	start := atClock(t, workStartHour, 0)
	end := atClock(t, workEndHour, 0)
	if t.Before(start) {
		return start
	}
	if !t.Before(end) {
		return nextWorkdayStart(t)
	}
	return t
}

// AddBusinessMinutes adds working minutes (Mon–Fri 08:00–17:00 WIB).
func AddBusinessMinutes(t time.Time, minutes int) time.Time {
	cur := normalize(t.In(BusinessLocation))
	remaining := time.Duration(minutes) * time.Minute
	for {
		end := atClock(cur, workEndHour, 0)
		if left := end.Sub(cur); remaining <= left {
			return cur.Add(remaining).UTC()
		} else {
			remaining -= left
		}
		cur = nextWorkdayStart(cur)
	}
}

// BusinessDaysAt returns the clock time `at` ("HH:MM") on the days-th
// workday after t's calendar day (WIB).
func BusinessDaysAt(t time.Time, days int, at string) time.Time {
	hour, minute := parseClock(at)
	d := t.In(BusinessLocation)
	for counted := 0; counted < days; {
		d = d.AddDate(0, 0, 1)
		if isWorkday(d) {
			counted++
		}
	}
	return atClock(d, hour, minute).UTC()
}

func parseClock(at string) (int, int) {
	parts := strings.SplitN(at, ":", 2)
	if len(parts) != 2 {
		return 9, 0
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 9, 0
	}
	return h, m
}
