package quotationpdf

import (
	"strings"
	"testing"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/pricing"
)

func summaryDoc() Document {
	return Document{
		Number: "QUO-2026-0001", ValidUntil: "30 Nov 2026", Issuer: Issuer{Name: "PT Zyad"}, Customer: Customer{Name: "Budi"},
		Lines: []Line{
			{No: 1, Description: "Internet <b>50</b> Mbps & instalasi", Quantity: "2 bulan", UnitPrice: "Rp 350.000", Discount: "10%", Tax: "11%", Total: "Rp 630.000"},
			{No: 2, Description: "Router", Quantity: "1", UnitPrice: "Rp 500.000", Discount: "-", Tax: "-", Total: "Rp 500.000"},
		},
		Subtotal: "Rp 1.200.000", DiscountTotal: "Rp 70.000", TaxTotal: "Rp 69.300", GrandTotal: "Rp 1.199.300",
	}
}

func TestBuildSummaryText(t *testing.T) {
	s := BuildSummary(summaryDoc(), "")
	want := []string{
		"Halo Budi, berikut penawaran QUO-2026-0001 dari PT Zyad.",
		"1. Internet <b>50</b> Mbps & instalasi",
		"   2 bulan × Rp 350.000 · diskon 10% · pajak 11% = Rp 630.000",
		"2. Router",
		"   1 × Rp 500.000 = Rp 500.000",
		"*Total: Rp 1.199.300*",
		"Berlaku s.d. 30 Nov 2026",
	}
	for _, w := range want {
		if !strings.Contains(s.Text, w) {
			t.Errorf("text missing %q\n---\n%s", w, s.Text)
		}
	}
	if s.Subject != "Penawaran QUO-2026-0001 – PT Zyad" {
		t.Errorf("subject = %q", s.Subject)
	}
}

func TestBuildSummaryHTMLEscapes(t *testing.T) {
	s := BuildSummary(summaryDoc(), `Halo "Pak" Budi <script>`)
	if strings.Contains(s.HTML, "<b>50</b>") || strings.Contains(s.HTML, "<script>") {
		t.Fatalf("html not escaped: %s", s.HTML)
	}
	for _, w := range []string{"Internet &lt;b&gt;50&lt;/b&gt; Mbps &amp; instalasi", "Halo &#34;Pak&#34; Budi &lt;script&gt;", "<table", "Rp 1.199.300"} {
		if !strings.Contains(s.HTML, w) {
			t.Errorf("html missing %q", w)
		}
	}
}

func TestBuildDocumentRecurring(t *testing.T) {
	q := crmdomain.Quotation{QuotationNumber: "QUO-1", Status: crmdomain.QuotationStatusSent,
		Subtotal: "800000.00", DiscountTotal: "0.00", TaxTotal: "33000.00", GrandTotal: "833000.00",
		OneTimeTotal: "500000.00", FirstInvoiceTotal: "833000.00",
		RecurringTotals: map[pricing.Frequency]string{pricing.Monthly: "333000.00"},
		Items: []crmdomain.QuotationItem{
			{Description: "Instalasi", Quantity: "1", UnitPrice: "500000.00", LineTotal: "500000.00", TaxPercent: "0", Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}},
			{Description: "Internet", Quantity: "1", UnitPrice: "300000.00", LineTotal: "300000.00", TaxPercent: "11", Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}},
		}}
	doc := BuildDocument(q, Issuer{Name: "PT Zyad"}, Customer{Name: "Budi"}, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	if doc.Lines[1].UnitPrice != "Rp 300.000/bulan" || doc.Lines[1].Billing != "Bulanan · Prabayar" || doc.Lines[0].Billing != "" {
		t.Fatalf("lines %+v", doc.Lines)
	}
	if !doc.HasRecurring || doc.FirstInvoiceTotal != "Rp 833.000" || doc.OneTimeTotal != "Rp 500.000" || doc.RecurringTotals[0] != (RecurringTotal{"Bulanan", "Rp 333.000/bulan"}) {
		t.Fatalf("totals %+v", doc)
	}
	s := BuildSummary(doc, "")
	for _, w := range []string{"Berulang: Rp 333.000/bulan", "*Tagihan pertama: Rp 833.000*", "Sekali bayar: Rp 500.000"} {
		if !strings.Contains(s.Text, w) {
			t.Errorf("text missing %q", w)
		}
	}
	if !strings.Contains(s.HTML, "Tagihan pertama") {
		t.Error("html missing Tagihan pertama")
	}
	if _, err := NewRenderer().Render(doc); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDocumentMixedFrequenciesOrdered(t *testing.T) {
	q := crmdomain.Quotation{QuotationNumber: "QUO-2", Status: crmdomain.QuotationStatusSent, Subtotal: "0", DiscountTotal: "0", TaxTotal: "0", GrandTotal: "0",
		RecurringTotals: map[pricing.Frequency]string{pricing.Annual: "200000.00", pricing.Monthly: "333000.00"}}
	doc := BuildDocument(q, Issuer{}, Customer{}, time.Now())
	if len(doc.RecurringTotals) != 2 || doc.RecurringTotals[0].Label != "Bulanan" || doc.RecurringTotals[1].Label != "Tahunan" {
		t.Fatalf("urutan %+v", doc.RecurringTotals)
	}
}

func TestSummaryWithoutRecurringUnchanged(t *testing.T) {
	s := BuildSummary(summaryDoc(), "")
	if strings.Contains(s.Text, "Tagihan pertama") || strings.Contains(s.HTML, "Tagihan pertama") {
		t.Fatal("one-time only quotation must not show breakdown")
	}
}

func TestBuildDocumentLegacyQuotationHasNoBreakdown(t *testing.T) {
	q := crmdomain.Quotation{QuotationNumber: "QUO-3", Status: crmdomain.QuotationStatusSent, Subtotal: "100.00", DiscountTotal: "0", TaxTotal: "0", GrandTotal: "100.00",
		Items: []crmdomain.QuotationItem{{Description: "Lama", Quantity: "1", UnitPrice: "100.00", LineTotal: "100.00"}}} // Pricing nol
	doc := BuildDocument(q, Issuer{}, Customer{}, time.Now())
	if doc.HasRecurring || doc.Lines[0].Billing != "" || doc.Lines[0].UnitPrice != "Rp 100" {
		t.Fatalf("legacy doc %+v", doc)
	}
}
