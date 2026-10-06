package service

import (
	"context"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

type fakeOverviewRepo struct {
	items  []repository.ActiveItem
	unpaid repository.UnpaidSummary
	failed []repository.FailedSend
}

func (f fakeOverviewRepo) ActiveItems(context.Context, coretenant.Scope, int) ([]repository.ActiveItem, error) {
	return f.items, nil
}
func (f fakeOverviewRepo) Unpaid(context.Context, coretenant.Scope, time.Time) (repository.UnpaidSummary, error) {
	return f.unpaid, nil
}
func (f fakeOverviewRepo) FailedSends(context.Context, coretenant.Scope, int) ([]repository.FailedSend, error) {
	return f.failed, nil
}

func activeItem(contractID, number string, start string, it domain.ContractItem) repository.ActiveItem {
	it.NextPeriodStart, it.NextPeriodEnd = pricing.PeriodRange(date(start), it.Frequency, it.PeriodIndex)
	return repository.ActiveItem{ContractID: contractID, ContractNumber: number, AccountName: "Budi", StartDate: date(start), Item: it}
}

func overviewFixture(today string) (*overviewService, fakeOverviewRepo) {
	monthly := item("A", pricing.Monthly, pricing.Prepaid, 1)
	monthly.TaxPercent = "11"
	annual := item("B", pricing.Annual, pricing.Postpaid, 0)
	annual.UnitPrice = "200000"
	repo := fakeOverviewRepo{
		items: []repository.ActiveItem{
			// sengaja dibalik: ringkasan harus urut harian→tahunan, bukan urutan data
			activeItem("c2", "CTR-2", "2026-10-05", annual),
			activeItem("c1", "CTR-1", "2026-10-05", monthly),
		},
		unpaid: repository.UnpaidSummary{Count: 3, OverdueCount: 1, TotalBalance: "999000.00"},
		failed: []repository.FailedSend{{InvoiceID: "i1", InvoiceNumber: "INV-1", Channel: "email", Error: "smtp down"}},
	}
	now := date(today).Add(3 * time.Hour) // 10:00 WIB
	s := NewOverviewService(repo, &fakeSettings{domain.Settings{InvoiceLeadDays: 7}}, func() time.Time { return now }).(*overviewService)
	return s, repo
}

func TestOverviewRecurringByFrequencyAndSummary(t *testing.T) {
	s, _ := overviewFixture("2026-10-20")
	o, err := s.Get(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if o.ActiveContracts != 2 {
		t.Fatalf("active contracts = %d", o.ActiveContracts)
	}
	if len(o.Recurring) != 2 || o.Recurring[0].Frequency != pricing.Monthly || o.Recurring[0].Amount != "333000.00" ||
		o.Recurring[1].Frequency != pricing.Annual || o.Recurring[1].Amount != "200000.00" || o.Recurring[0].Label != "Bulanan" {
		t.Fatalf("recurring = %+v", o.Recurring)
	}
	if o.Unpaid.Count != 3 || o.Unpaid.OverdueCount != 1 || len(o.FailedSends) != 1 {
		t.Fatalf("unpaid/failed = %+v %+v", o.Unpaid, o.FailedSends)
	}
}

func TestOverviewUpcomingOnlyWithin30Days(t *testing.T) {
	s, _ := overviewFixture("2026-10-20")
	o, _ := s.Get(ctx, scope)
	// bulanan: tanggal tagih 29 Okt (9 hari lagi); berikutnya 28 Nov (39 hari) di luar jendela.
	// tahunan pascabayar: tagih 5 Okt 2027 → di luar jendela.
	if len(o.Upcoming) != 1 {
		t.Fatalf("upcoming = %+v", o.Upcoming)
	}
	u := o.Upcoming[0]
	if u.ContractNumber != "CTR-1" || u.BillOn.Format("2006-01-02") != "2026-10-29" || u.PeriodStart.Format("2006-01-02") != "2026-11-05" ||
		u.PeriodEnd.Format("2006-01-02") != "2026-12-04" || u.Amount != "333000.00" || u.PaymentTiming != pricing.Prepaid {
		t.Fatalf("upcoming[0] = %+v", u)
	}
}

func TestOverviewUpcomingRespectsEndDate(t *testing.T) {
	s, repo := overviewFixture("2026-10-20")
	end := date("2026-11-04") // periode berikutnya mulai 5 Nov → tidak ditagih
	repo.items[1].EndDate = &end
	s.repo = repo
	o, _ := s.Get(ctx, scope)
	if len(o.Upcoming) != 0 {
		t.Fatalf("upcoming after end_date = %+v", o.Upcoming)
	}
}

func TestContractUpcomingThreePerItem(t *testing.T) {
	s, _ := overviewFixture("2026-10-20")
	c := domain.Contract{ID: "c1", Status: domain.ContractActive, StartDate: date("2026-10-05"), Items: []domain.ContractItem{
		activeItem("c1", "CTR-1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1)).Item,
	}}
	got, err := s.ContractUpcoming(ctx, scope, c, 3)
	if err != nil {
		t.Fatal(err)
	}
	keys := ""
	for _, u := range got {
		keys += u.BillOn.Format("2006-01-02") + ","
	}
	if len(got) != 3 || keys != "2026-10-29,2026-11-28,2026-12-29," {
		t.Fatalf("upcoming = %s", keys)
	}
}
