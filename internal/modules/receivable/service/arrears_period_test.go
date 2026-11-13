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

func TestArrearsPeriods_MonthlyUpToToday(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-08-15"), Items: []domain.ContractItem{monthlyItem("300000", "0")}}
	got := ArrearsPeriods(c, d("2026-11-13"))
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
	if got := ArrearsPeriods(c, d("2026-11-13")); len(got) != 1 {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_ClampsToShortMonth(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-01-31"), Items: []domain.ContractItem{monthlyItem("100000", "0")}}
	got := ArrearsPeriods(c, d("2026-04-01"))
	if len(got) != 3 || got[1].Start.Format("2006-01-02") != "2026-02-28" || got[0].End.Format("2006-01-02") != "2026-02-27" ||
		got[2].Start.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_FutureStartIsEmpty(t *testing.T) {
	c := domain.Contract{StartDate: d("2026-12-01"), Items: []domain.ContractItem{monthlyItem("100000", "0")}}
	if got := ArrearsPeriods(c, d("2026-11-13")); len(got) != 0 {
		t.Fatalf("periods = %+v", got)
	}
}

func TestArrearsPeriods_DefaultAmountSumsSameFrequencyWithTax(t *testing.T) {
	addon := monthlyItem("50000", "11")
	addon.Quantity = "2"
	annual := domain.ContractItem{Description: "Domain", Quantity: "1", UnitPrice: "200000", TaxPercent: "0",
		Frequency: pricing.Annual, PaymentTiming: pricing.Postpaid}
	c := domain.Contract{StartDate: d("2026-10-01"), Items: []domain.ContractItem{monthlyItem("300000", "11"), addon, annual}}
	got := ArrearsPeriods(c, d("2026-10-20"))
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
