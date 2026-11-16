package service

import (
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/pricing"
)

// d is the value form of the harness helper day().
func d(s string) time.Time { return *day(s) }

func monthlyItem(price, tax string) domain.ContractItem {
	return domain.ContractItem{Description: "Internet", Quantity: "1", UnitPrice: price, TaxPercent: tax,
		Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
}

// forward keeps the periods from start_date on (Index >= 0); the backward ones are tested separately.
func forward(ps []ArrearsPeriod) []ArrearsPeriod {
	out := []ArrearsPeriod{}
	for _, p := range ps {
		if p.Index >= 0 {
			out = append(out, p)
		}
	}
	return out
}

func TestArrearsPeriods_MonthlyUpToToday(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-08-15"), Items: []domain.ContractItem{monthlyItem("300000", "0")}}
	got := forward(ArrearsPeriods(c, d("2026-11-13")))
	want := [][2]string{{"2026-08-15", "2026-09-14"}, {"2026-09-15", "2026-10-14"}, {"2026-10-15", "2026-11-14"}}
	if len(got) != len(want) {
		t.Fatalf("periods = %+v", got)
	}
	for i, w := range want {
		if got[i].Index != i || got[i].Start.Format("2006-01-02") != w[0] || got[i].End.Format("2006-01-02") != w[1] ||
			got[i].DefaultAmount != "300000.00" {
			t.Fatalf("period %d = %+v, want %v", i, got[i], w)
		}
	}
}

func TestArrearsPeriods_StartTodayIncluded(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-11-13"), Items: []domain.ContractItem{monthlyItem("100000", "0")}}
	if got := forward(ArrearsPeriods(c, d("2026-11-13"))); len(got) != 1 {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_ClampsToShortMonth(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-01-31"), Items: []domain.ContractItem{monthlyItem("100000", "0")}}
	got := forward(ArrearsPeriods(c, d("2026-04-01")))
	if len(got) != 3 || got[1].Start.Format("2006-01-02") != "2026-02-28" || got[0].End.Format("2006-01-02") != "2026-02-27" ||
		got[2].Start.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_FutureStartHasNoForwardPeriod(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-12-01"), Items: []domain.ContractItem{monthlyItem("100000", "0")}}
	if got := forward(ArrearsPeriods(c, d("2026-11-13"))); len(got) != 0 {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_DefaultAmountSumsSameFrequencyWithTax(t *testing.T) {
	addon := monthlyItem("50000", "11")
	addon.Quantity = "2"
	annual := domain.ContractItem{Description: "Domain", Quantity: "1", UnitPrice: "200000", TaxPercent: "0",
		Frequency: pricing.Annual, PaymentTiming: pricing.Postpaid}
	c := domain.Contract{StartDate: d("2026-10-01"), Items: []domain.ContractItem{monthlyItem("300000", "11"), addon, annual}}
	got := forward(ArrearsPeriods(c, d("2026-10-20")))
	// 300000×1.11 + 2×50000×1.11 = 333000 + 111000 = 444000; the annual item is excluded.
	if len(got) != 1 || got[0].DefaultAmount != "444000.00" {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_NoItemsIsEmpty(t *testing.T) {
	if got := ArrearsPeriods(domain.Contract{StartDate: d("2026-01-01")}, d("2026-11-13")); len(got) != 0 {
		t.Fatalf("periods = %+v", got)
	}
}

// I1: periods from the billing pointer on are the engine's to bill; offering them double-bills.
func TestArrearsPeriods_StopsAtTheBillingPointer(t *testing.T) {
	it := monthlyItem("300000", "0")
	it.PaymentTiming = "postpaid"
	it.NextPeriodStart = d("2026-10-15") // 15 Okt–14 Nov not billed yet; the run bills it on 15 Nov
	c := domain.Contract{StartDate: d("2026-08-15"), Items: []domain.ContractItem{it}}
	got := forward(ArrearsPeriods(c, d("2026-11-13")))
	if len(got) != 2 || got[1].Start.Format("2006-01-02") != "2026-09-15" {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_PointerIsTheEarliestOfSameFrequencyItems(t *testing.T) {
	a, b := monthlyItem("100000", "0"), monthlyItem("50000", "0")
	a.NextPeriodStart, b.NextPeriodStart = d("2026-11-15"), d("2026-10-15")
	c := domain.Contract{StartDate: d("2026-08-15"), Items: []domain.ContractItem{a, b}}
	if got := forward(ArrearsPeriods(c, d("2026-11-13"))); len(got) != 2 {
		t.Fatalf("periods = %+v", got)
	}
}

// M1 (re-graded): an ended contract offers no period that starts after its end date.
func TestArrearsPeriods_StopsAtContractEnd(t *testing.T) {
	end := d("2026-09-20")
	c := domain.Contract{StartDate: d("2026-08-15"), EndDate: &end, Items: []domain.ContractItem{monthlyItem("300000", "0")}}
	got := forward(ArrearsPeriods(c, d("2026-11-13")))
	if len(got) != 2 || got[1].Start.Format("2006-01-02") != "2026-09-15" {
		t.Fatalf("periods = %+v", got)
	}
}

// Q1 (user decision 13 Nov): an onboarded customer's contract starts at the current cycle, but
// their old arrears are older; up to 24 cycles before start_date can be recorded.
func TestArrearsPeriods_BackwardBeforeStartDate(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-11-05"), Items: []domain.ContractItem{monthlyItem("300000", "0")}}
	got := ArrearsPeriods(c, d("2026-11-13"))
	if len(got) != maxArrearsBackPeriods+1 {
		t.Fatalf("periods = %d, want %d", len(got), maxArrearsBackPeriods+1)
	}
	first, prev, last := got[0], got[len(got)-2], got[len(got)-1]
	if first.Index != -24 || first.Start.Format("2006-01-02") != "2024-11-05" ||
		prev.Index != -1 || prev.Start.Format("2006-01-02") != "2026-10-05" || prev.End.Format("2006-01-02") != "2026-11-04" ||
		last.Index != 0 || last.Start.Format("2006-01-02") != "2026-11-05" || prev.DefaultAmount != "300000.00" {
		t.Fatalf("first=%+v prev=%+v last=%+v", first, prev, last)
	}
}
