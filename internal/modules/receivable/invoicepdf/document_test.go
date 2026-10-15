package invoicepdf

import (
	"bytes"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
)

func date(s string) *time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return &d
}

func sample(status domain.InvoiceStatus, paid string) domain.Invoice {
	return domain.Invoice{
		InvoiceNumber: "INV-2026-0001", Status: status, Account: domain.Account{Name: "Budi", CompanyName: "PT Maju", Email: "b@x.id"},
		IssueDate: date("2026-10-05"), DueDate: date("2026-10-12"), PeriodStart: date("2026-10-01"), PeriodEnd: date("2026-10-31"),
		Subtotal: "333000.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "333000.00", AmountPaid: paid, Notes: "Terima kasih",
		Items: []domain.InvoiceItem{
			{Description: "Instalasi", Quantity: "1.00", UnitPrice: "33000.00", TaxPercent: "0.00", LineTotal: "33000.00", Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}},
			{Description: "Internet 50 Mbps", Quantity: "1.00", Unit: "bulan", UnitPrice: "300000.00", DiscountPercent: "", TaxPercent: "0.00", LineTotal: "300000.00",
				Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}, PeriodStart: date("2026-10-01"), PeriodEnd: date("2026-10-31")},
		},
	}
}

func rowValue(rows []docpdf.Row, label string) (string, bool) {
	for _, r := range rows {
		if r.Label == label {
			return r.Value, true
		}
	}
	return "", false
}

func TestBuildPaidWithPeriod(t *testing.T) {
	doc := Build(sample(domain.InvoicePaid, "333000.00"), docpdf.Party{Name: "PT Zyad"}, time.Now())
	if doc.Stamp != "LUNAS" || doc.Title != "INVOICE" || doc.CustomerLabel != "Tagihan untuk" || doc.Number != "INV-2026-0001" || doc.Filename != "INV-2026-0001.pdf" {
		t.Fatalf("header = %+v", doc)
	}
	if v, ok := rowValue(doc.Meta, "Periode"); !ok || v != "1 Okt 2026 – 31 Okt 2026" {
		t.Fatalf("meta = %+v", doc.Meta)
	}
	if v, _ := rowValue(doc.Meta, "Jatuh tempo"); v != "12 Okt 2026" {
		t.Fatalf("due = %q", v)
	}
	if doc.Customer.Name != "Budi" || doc.Customer.Company != "PT Maju" {
		t.Fatalf("customer = %+v", doc.Customer)
	}
	if doc.Lines[0].Billing != "" {
		t.Fatalf("default one-time prepaid line has no billing note, got %q", doc.Lines[0].Billing)
	}
	if got := doc.Lines[1]; got.Billing != "Bulanan · Prabayar · 1 Okt 2026 – 31 Okt 2026" || got.UnitPrice != "Rp 300.000/bulan" || got.Quantity != "1 bulan" || got.Discount != "-" {
		t.Fatalf("line 2 = %+v", got)
	}
	out, err := docpdf.NewRenderer().Render(doc)
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("render err = %v", err)
	}
}

func TestBuildTotalsShowPaidAndBalanceOnlyWhenPaid(t *testing.T) {
	unpaid := Build(sample(domain.InvoiceIssued, "0.00"), docpdf.Party{}, time.Now())
	if _, ok := rowValue(unpaid.Totals, "Dibayar"); ok || unpaid.Stamp != "" {
		t.Fatalf("unpaid totals = %+v stamp=%q", unpaid.Totals, unpaid.Stamp)
	}
	partial := Build(sample(domain.InvoiceIssued, "100000.00"), docpdf.Party{}, time.Now())
	if v, _ := rowValue(partial.Totals, "Dibayar"); v != "Rp 100.000" {
		t.Fatalf("paid row = %q", v)
	}
	if v, _ := rowValue(partial.Totals, "Sisa"); v != "Rp 233.000" {
		t.Fatalf("balance row = %q", v)
	}
	if total, _ := rowValue(partial.Totals, "Total"); total != "Rp 333.000" {
		t.Fatalf("total = %q", total)
	}
}

func TestBuildStampsAndDraftDefaults(t *testing.T) {
	now := time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)
	draft := sample(domain.InvoiceDraft, "0.00")
	draft.InvoiceNumber, draft.IssueDate, draft.DueDate = "", nil, nil
	d := Build(draft, docpdf.Party{}, now)
	if d.Stamp != "DRAFT" || d.Filename != "invoice-draft.pdf" {
		t.Fatalf("draft = stamp %q file %q", d.Stamp, d.Filename)
	}
	if v, _ := rowValue(d.Meta, "Tanggal"); v != "5 Okt 2026" {
		t.Fatalf("draft date must default to now, got %q", v)
	}
	if v, _ := rowValue(d.Meta, "Jatuh tempo"); v != "-" {
		t.Fatalf("draft due = %q", v)
	}
	if Build(sample(domain.InvoiceVoid, "0.00"), docpdf.Party{}, now).Stamp != "DIBATALKAN" {
		t.Fatal("void stamp")
	}
	noPeriod := sample(domain.InvoiceIssued, "0.00")
	noPeriod.PeriodStart, noPeriod.PeriodEnd = nil, nil
	if _, ok := rowValue(Build(noPeriod, docpdf.Party{}, now).Meta, "Periode"); ok {
		t.Fatal("no period row without period")
	}
}
