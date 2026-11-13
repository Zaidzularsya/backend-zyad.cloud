package service

import (
	"math/big"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/pricing"
)

// maxArrearsPeriods caps how many past periods ArrearsPeriods lists (the most recent ones).
const maxArrearsPeriods = 120

// maxArrearsBackPeriods: how many cycles before start_date can be recorded (Q1, 13 Nov 2026).
const maxArrearsBackPeriods = 24

// ArrearsPeriod is one billing cycle of a contract that may be recorded as arrears.
type ArrearsPeriod struct {
	Index         int
	Start, End    time.Time
	DefaultAmount string // gross (incl. tax) total of the contract items of the reference frequency
}

// ArrearsPeriods lists the contract's billing cycles, from maxArrearsBackPeriods cycles before
// start_date (negative Index: service delivered before the customer entered the system, e.g. an
// onboarded old customer whose contract starts at the current cycle), whose start is on or
// before today (calendar dates), before the billing pointer (earliest next_period_start of the
// reference-frequency items) and not after the contract end date, following pricing.PeriodRange. The reference frequency is
// the first item's; DefaultAmount sums every item of that frequency (qty, discount, tax),
// rounded to 2 decimals. Only the latest maxArrearsPeriods are returned.
func ArrearsPeriods(c domain.Contract, today time.Time) []ArrearsPeriod {
	out := []ArrearsPeriod{}
	if len(c.Items) == 0 {
		return out
	}
	freq := c.Items[0].Frequency
	amount := arrearsDefaultAmount(c.Items, freq)
	y, m, d := today.Date()
	limit := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	// Periods from the billing pointer on belong to the billing run (it bills them from
	// next_period_start); recording them as arrears would bill the customer twice.
	var pointer time.Time
	for _, it := range c.Items {
		if it.Frequency == freq && !it.NextPeriodStart.IsZero() && (pointer.IsZero() || it.NextPeriodStart.Before(pointer)) {
			pointer = it.NextPeriodStart
		}
	}
	for n := -maxArrearsBackPeriods; ; n++ {
		from, to := pricing.PeriodRange(c.StartDate, freq, n)
		if from.After(limit) || (!pointer.IsZero() && !from.Before(pointer)) || (c.EndDate != nil && from.After(*c.EndDate)) {
			break
		}
		out = append(out, ArrearsPeriod{Index: n, Start: from, End: to, DefaultAmount: amount})
	}
	if len(out) > maxArrearsPeriods {
		out = out[len(out)-maxArrearsPeriods:]
	}
	return out
}

func arrearsDefaultAmount(items []domain.ContractItem, freq pricing.Frequency) string {
	lines := []pricing.LineInput{}
	for _, it := range items {
		if it.Frequency != freq {
			continue
		}
		lines = append(lines, pricing.LineInput{
			Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice,
			DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent,
		})
	}
	totals, _, err := pricing.PriceLines(lines)
	if err != nil {
		return "0.00"
	}
	grand, ok := new(big.Rat).SetString(totals.GrandTotal)
	if !ok {
		return "0.00"
	}
	return grand.FloatString(2)
}
