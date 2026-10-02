package quotationpdf

import (
	"strings"
	"testing"
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
