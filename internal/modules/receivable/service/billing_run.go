package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
)

// BillingRunWorkerIdentity adalah identitas layanan billing run (cmd/worker) pada WorkerResolver.
const BillingRunWorkerIdentity = "receivable-billing-run"

const (
	// billingRunMaxRounds membatasi catch-up: satu putaran = satu periode per item.
	billingRunMaxRounds = 24
	billingRunBatch     = 500
)

type BillingRunResult struct {
	Invoices, Advanced, Ended int
	Errors                    []string
}

// BillingRun menerbitkan invoice periode berikutnya untuk contract aktif.
type BillingRun struct {
	contracts repository.ContractRepository
	invoices  InvoiceService
	settings  repository.SettingsRepository
	now       func() time.Time
}

func NewBillingRun(contracts repository.ContractRepository, invoices InvoiceService, settings repository.SettingsRepository, now func() time.Time) *BillingRun {
	if now == nil {
		now = time.Now
	}
	return &BillingRun{contracts: contracts, invoices: invoices, settings: settings, now: now}
}

type billGroup struct {
	contract domain.Contract
	billOn   time.Time
	items    []repository.DueItem
}

// Run memproses satu organisasi. Tiap putaran mengambil item jatuh tempo, menagih satu invoice per
// (contract, tanggal tagih), lalu memajukan item satu periode. Aman dijalankan paralel/berulang:
// invoice idempoten lewat kunci periode dan Advance kondisional pada period_index.
func (r *BillingRun) Run(ctx context.Context, scope coretenant.Scope) (BillingRunResult, error) {
	var res BillingRunResult
	today := businesstime.DayOf(r.now())
	settings, err := r.settings.Get(ctx, scope)
	if err != nil {
		return res, err
	}
	failed := map[string]bool{} // item yang gagal pada run ini; dilewati agar tidak diulang tiap putaran

	for round := 0; round < billingRunMaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		due, err := r.contracts.DueItems(ctx, scope, today, settings.InvoiceLeadDays, billingRunBatch)
		if err != nil {
			return res, err
		}
		groups := groupDue(due, failed)
		if len(groups) == 0 {
			break
		}
		for _, g := range groups {
			r.bill(ctx, scope, g, failed, &res)
		}
	}

	ended, err := r.contracts.EndExpired(ctx, scope, today)
	if err != nil {
		return res, err
	}
	res.Ended = int(ended)
	return res, nil
}

func groupDue(due []repository.DueItem, failed map[string]bool) []*billGroup {
	var groups []*billGroup
	index := map[string]*billGroup{}
	for _, d := range due {
		if failed[d.ItemID] {
			continue
		}
		key := d.ContractID + "|" + d.BillOn.Format("2006-01-02")
		g, ok := index[key]
		if !ok {
			g = &billGroup{contract: d.Contract, billOn: d.BillOn}
			index[key] = g
			groups = append(groups, g)
		}
		g.items = append(g.items, d)
	}
	// DueItems urut next_period_start; invoice harus terbit kronologis menurut tanggal tagih per contract.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].contract.ID != groups[j].contract.ID {
			return groups[i].contract.ID < groups[j].contract.ID
		}
		return groups[i].billOn.Before(groups[j].billOn)
	})
	return groups
}

func (r *BillingRun) bill(ctx context.Context, scope coretenant.Scope, g *billGroup, failed map[string]bool, res *BillingRunResult) {
	fail := func(err error) {
		res.Errors = append(res.Errors, fmt.Sprintf("kontrak %s (%s): %v", g.contract.ContractNumber, g.billOn.Format("2006-01-02"), err))
		for _, it := range g.items {
			failed[it.ItemID] = true
		}
	}
	if _, err := r.invoices.CreateAndIssue(ctx, scope, invoiceInputFor(g), ""); err != nil {
		fail(err)
		return
	}
	res.Invoices++
	for _, d := range g.items {
		nextStart, nextEnd := pricing.PeriodRange(g.contract.StartDate, d.Item.Frequency, d.Item.PeriodIndex+1)
		advanced, err := r.contracts.Advance(ctx, scope, d.ItemID, d.Item.PeriodIndex, nextStart, nextEnd)
		if err != nil {
			fail(err)
			return
		}
		if advanced {
			res.Advanced++
		}
	}
}

func invoiceInputFor(g *billGroup) InvoiceInput {
	c := g.contract
	in := InvoiceInput{
		AccountID: c.AccountID, SourceType: domain.SourceContract, SourceID: c.ID, ContractID: c.ID,
		IdempotencyKey: "period:" + g.billOn.Format("2006-01-02"),
		Currency:       c.Currency, PICUserID: c.PICUserID, Channels: c.Channels,
	}
	first := g.items[0].Item
	samePeriod := true
	for _, d := range g.items {
		it := d.Item
		samePeriod = samePeriod && it.NextPeriodStart.Equal(first.NextPeriodStart) && it.NextPeriodEnd.Equal(first.NextPeriodEnd)
		start, end := it.NextPeriodStart, it.NextPeriodEnd
		in.Lines = append(in.Lines, LineInput{
			LineInput: pricing.LineInput{
				Description: fmt.Sprintf("%s (periode %s – %s)", it.Description, docpdf.FormatDateID(start), docpdf.FormatDateID(end)),
				Quantity:    it.Quantity, UnitPrice: it.UnitPrice, DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent,
				Attrs: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: it.Frequency, PaymentTiming: it.PaymentTiming},
			},
			Unit: it.Unit, ProductID: it.ProductID, SKU: it.SKU, ContractItemID: it.ID, SourceLineID: it.ID,
			PeriodStart: &start, PeriodEnd: &end,
		})
	}
	if samePeriod {
		start, end := first.NextPeriodStart, first.NextPeriodEnd
		in.PeriodStart, in.PeriodEnd = &start, &end
	}
	return in
}

type BillingRunnerResult struct {
	Checked, Invoices, Advanced, Ended int
	Failed                             int // organisasi gagal total atau punya error per kelompok
}

// BillingRunner menjalankan BillingRun untuk setiap organisasi aktif (job worker per jam).
type BillingRunner struct {
	orgs     OrganizationLister
	scopes   PublicScopeResolver
	run      *BillingRun
	pageSize int
}

func NewBillingRunner(orgs OrganizationLister, scopes PublicScopeResolver, run *BillingRun) *BillingRunner {
	return &BillingRunner{orgs: orgs, scopes: scopes, run: run, pageSize: overduePageSize}
}

// RunOnce satu putaran. Galat per organisasi dicatat lalu dilanjutkan; hanya pembatalan konteks atau
// galat daftar organisasi yang menghentikan putaran.
func (r *BillingRunner) RunOnce(ctx context.Context) (BillingRunnerResult, error) {
	var res BillingRunnerResult
	for offset := 0; ; offset += r.pageSize {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		ids, err := r.orgs.ListActive(ctx, r.pageSize, offset)
		if err != nil {
			return res, err
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			res.Checked++
			out, err := r.runOrganization(ctx, id)
			res.Invoices += out.Invoices
			res.Advanced += out.Advanced
			res.Ended += out.Ended
			if err != nil || len(out.Errors) > 0 {
				res.Failed++
				slog.WarnContext(ctx, "receivable: billing run failed for organization", "organization_id", id,
					"error", err, "group_errors", out.Errors)
			}
			if out.Invoices > 0 || out.Advanced > 0 || out.Ended > 0 {
				slog.InfoContext(ctx, "receivable: billing run", "organization_id", id,
					"invoices", out.Invoices, "advanced", out.Advanced, "ended", out.Ended, "errors", len(out.Errors))
			}
		}
		if len(ids) < r.pageSize {
			return res, nil
		}
	}
}

func (r *BillingRunner) runOrganization(ctx context.Context, organizationID string) (BillingRunResult, error) {
	tenantCtx, err := r.scopes.ResolveWorkerOrganization(ctx, organizationID, BillingRunWorkerIdentity)
	if err != nil {
		return BillingRunResult{}, err
	}
	scope, err := coretenant.NewScope(tenantCtx)
	if err != nil {
		return BillingRunResult{}, err
	}
	return r.run.Run(ctx, scope)
}
