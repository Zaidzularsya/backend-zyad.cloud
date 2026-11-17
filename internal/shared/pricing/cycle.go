package pricing

import "time"

// CurrentCycleStart returns the start of the billing cycle running on today
// for a monthly billing day (1–31): the latest date ≤ today whose day is
// min(billingDay, last day of that month). Result is UTC midnight; the time of
// day of today is ignored. billingDay outside 1–31 is clamped into range.
func CurrentCycleStart(billingDay int, today time.Time) time.Time {
	billingDay = max(1, min(billingDay, 31))
	y, m, d := today.Date()
	candidate := clampedDay(y, m, billingDay)
	if candidate.Day() <= d {
		return candidate
	}
	prev := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	return clampedDay(prev.Year(), prev.Month(), billingDay)
}

func clampedDay(y int, m time.Month, day int) time.Time {
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(y, m, min(day, last), 0, 0, 0, 0, time.UTC)
}

// FirstPeriodIndex is the period a new contract item starts billing from when
// no initial invoice was issued: 0 while the issue moment of period 0 is still
// ahead (prepaid: start − leadDays; postpaid: end of period 0 + 1 day), so the
// billing run issues it on time; 1 once that moment is today or past, so no
// back-dated invoice is issued and billing continues with the next cycle.
// The moments match billOnExpr in the receivable contract repository.
func FirstPeriodIndex(timing PaymentTiming, f Frequency, start, today time.Time, leadDays int) int {
	y, m, d := today.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	var issueOn time.Time
	if timing == Postpaid {
		_, end := PeriodRange(start, f, 0)
		issueOn = end.AddDate(0, 0, 1)
	} else {
		issueOn = AddPeriod(start, f, 0).AddDate(0, 0, -leadDays)
	}
	if issueOn.After(day) {
		return 0
	}
	return 1
}
