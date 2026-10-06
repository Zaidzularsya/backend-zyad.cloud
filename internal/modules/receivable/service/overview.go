package service

import (
	"context"
	"math/big"
	"sort"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

const (
	overviewHorizonDays = 30
	overviewMaxUpcoming = 100
	overviewMaxFailed   = 50
	overviewMaxItems    = 5000
	upcomingMaxPeriods  = 100 // pagar loop untuk item harian
)

type RecurringTotal struct {
	Frequency     pricing.Frequency
	Label, Amount string
}

// UpcomingBilling: satu tagihan yang akan terbit (belum ditagih).
type UpcomingBilling struct {
	ContractID, ContractNumber, AccountName, ItemID, Description, Amount string
	BillOn, PeriodStart, PeriodEnd                                       time.Time
	PaymentTiming                                                        pricing.PaymentTiming
}

type Overview struct {
	ActiveContracts int
	Recurring       []RecurringTotal
	Upcoming        []UpcomingBilling
	Unpaid          repository.UnpaidSummary
	FailedSends     []repository.FailedSend
}

type OverviewService interface {
	Get(ctx context.Context, scope coretenant.Scope) (Overview, error)
	// ContractUpcoming: perItem tanggal tagih berikutnya untuk tiap item contract (urut tanggal tagih).
	ContractUpcoming(ctx context.Context, scope coretenant.Scope, c domain.Contract, perItem int) ([]UpcomingBilling, error)
}

type overviewService struct {
	repo     repository.OverviewRepository
	settings repository.SettingsRepository
	now      func() time.Time
}

func NewOverviewService(repo repository.OverviewRepository, settings repository.SettingsRepository, now func() time.Time) OverviewService {
	if now == nil {
		now = time.Now
	}
	return &overviewService{repo: repo, settings: settings, now: now}
}

func (s *overviewService) Get(ctx context.Context, scope coretenant.Scope) (Overview, error) {
	today := businesstime.DayOf(s.now())
	settings, err := s.settings.Get(ctx, scope)
	if err != nil {
		return Overview{}, err
	}
	items, err := s.repo.ActiveItems(ctx, scope, overviewMaxItems)
	if err != nil {
		return Overview{}, err
	}
	unpaid, err := s.repo.Unpaid(ctx, scope, today)
	if err != nil {
		return Overview{}, err
	}
	failed, err := s.repo.FailedSends(ctx, scope, overviewMaxFailed)
	if err != nil {
		return Overview{}, err
	}

	o := Overview{Unpaid: unpaid, FailedSends: failed, Recurring: []RecurringTotal{}, Upcoming: []UpcomingBilling{}}
	contracts := map[string]bool{}
	sums := map[pricing.Frequency]*big.Rat{}
	horizon := today.AddDate(0, 0, overviewHorizonDays)
	for _, a := range items {
		contracts[a.ContractID] = true
		amount, ok := itemAmount(a.Item)
		if !ok {
			continue
		}
		if sums[a.Item.Frequency] == nil {
			sums[a.Item.Frequency] = new(big.Rat)
		}
		sums[a.Item.Frequency].Add(sums[a.Item.Frequency], amount)
		o.Upcoming = append(o.Upcoming, upcomingFor(a, settings.InvoiceLeadDays, 0, &horizon)...)
	}
	o.ActiveContracts = len(contracts)
	for _, f := range pricing.Frequencies() {
		if sum := sums[f]; sum != nil {
			o.Recurring = append(o.Recurring, RecurringTotal{Frequency: f, Label: f.Label(), Amount: sum.FloatString(2)})
		}
	}
	sortUpcoming(o.Upcoming)
	if len(o.Upcoming) > overviewMaxUpcoming {
		o.Upcoming = o.Upcoming[:overviewMaxUpcoming]
	}
	return o, nil
}

func (s *overviewService) ContractUpcoming(ctx context.Context, scope coretenant.Scope, c domain.Contract, perItem int) ([]UpcomingBilling, error) {
	if c.Status != domain.ContractActive {
		return []UpcomingBilling{}, nil
	}
	settings, err := s.settings.Get(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := []UpcomingBilling{}
	for _, it := range c.Items {
		a := repository.ActiveItem{ContractID: c.ID, ContractNumber: c.ContractNumber, AccountName: c.Account.Name,
			StartDate: c.StartDate, EndDate: c.EndDate, Item: it}
		out = append(out, upcomingFor(a, settings.InvoiceLeadDays, perItem, nil)...)
	}
	sortUpcoming(out)
	return out, nil
}

func sortUpcoming(u []UpcomingBilling) {
	sort.SliceStable(u, func(i, j int) bool {
		if !u[i].BillOn.Equal(u[j].BillOn) {
			return u[i].BillOn.Before(u[j].BillOn)
		}
		return u[i].ContractNumber < u[j].ContractNumber
	})
}

// upcomingFor menurunkan tanggal tagih periode-periode berikutnya sebuah item dengan rumus yang sama
// dengan billing run. limit > 0 membatasi jumlah; horizon != nil membatasi tanggal tagih.
func upcomingFor(a repository.ActiveItem, leadDays, limit int, horizon *time.Time) []UpcomingBilling {
	amount, ok := itemAmount(a.Item)
	if !ok {
		return nil
	}
	var out []UpcomingBilling
	for k := 0; k < upcomingMaxPeriods && (limit == 0 || len(out) < limit); k++ {
		start, end := pricing.PeriodRange(a.StartDate, a.Item.Frequency, a.Item.PeriodIndex+k)
		if a.EndDate != nil && start.After(*a.EndDate) {
			break
		}
		billOn := end.AddDate(0, 0, 1)
		if a.Item.PaymentTiming == pricing.Prepaid {
			billOn = start.AddDate(0, 0, -leadDays)
		}
		if horizon != nil && billOn.After(*horizon) {
			break
		}
		out = append(out, UpcomingBilling{
			ContractID: a.ContractID, ContractNumber: a.ContractNumber, AccountName: a.AccountName,
			ItemID: a.Item.ID, Description: a.Item.Description, Amount: amount.FloatString(2),
			BillOn: billOn, PeriodStart: start, PeriodEnd: end, PaymentTiming: a.Item.PaymentTiming,
		})
	}
	return out
}

// itemAmount: nilai satu periode item = qty×harga − diskon + pajak (pembulatan sama dengan invoice).
func itemAmount(it domain.ContractItem) (*big.Rat, bool) {
	totals, _, err := pricing.PriceLines([]pricing.LineInput{{
		Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice, DiscountPercent: it.DiscountPercent,
		TaxPercent: it.TaxPercent,
		Attrs:      pricing.Attributes{ChargeType: pricing.Recurring, Frequency: it.Frequency, PaymentTiming: it.PaymentTiming},
	}})
	if err != nil {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(totals.GrandTotal)
	return r, ok
}
