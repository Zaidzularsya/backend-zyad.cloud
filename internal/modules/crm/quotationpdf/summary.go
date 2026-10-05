package quotationpdf

import (
	"html"
	"strings"
)

type Summary struct {
	Subject string
	Text    string
	HTML    string
}

func DefaultOpening(doc Document) string {
	name := doc.Customer.Name
	if name == "" {
		name = "Bapak/Ibu"
	}
	return "Halo " + name + ", berikut penawaran " + doc.Number + " dari " + doc.Issuer.Name + "."
}

// totalsBreakdown: baris rincian (label, nilai, tebal) hanya bila ada baris berulang.
func totalsBreakdown(doc Document) [][3]string {
	if !doc.HasRecurring {
		return nil
	}
	rows := [][3]string{{"Sekali bayar", doc.OneTimeTotal, ""}}
	for _, r := range doc.RecurringTotals {
		rows = append(rows, [3]string{"Berulang (" + r.Label + ")", r.Amount, ""})
	}
	return append(rows, [3]string{"Tagihan pertama", doc.FirstInvoiceTotal, "b"})
}

func lineDetail(l Line) string {
	parts := []string{l.Quantity + " × " + l.UnitPrice}
	if l.Discount != "-" && l.Discount != "" {
		parts = append(parts, "diskon "+l.Discount)
	}
	if l.Tax != "-" && l.Tax != "" {
		parts = append(parts, "pajak "+l.Tax)
	}
	return strings.Join(parts, " · ") + " = " + l.Total
}

// BuildSummary menyusun ringkasan teks/HTML; link "" = tanpa baris link customer.
func BuildSummary(doc Document, opening, link string) Summary {
	opening = strings.TrimSpace(opening)
	if opening == "" {
		opening = DefaultOpening(doc)
	}
	var t strings.Builder
	t.WriteString(opening + "\n\n*Penawaran " + doc.Number + "*\n")
	for _, l := range doc.Lines {
		t.WriteString(itoa(l.No) + ". " + l.Description + "\n   " + lineDetail(l) + "\n")
	}
	t.WriteString("\nSubtotal: " + doc.Subtotal + "\n")
	if doc.DiscountTotal != "Rp 0" {
		t.WriteString("Diskon: " + doc.DiscountTotal + "\n")
	}
	if doc.TaxTotal != "Rp 0" {
		t.WriteString("Pajak: " + doc.TaxTotal + "\n")
	}
	t.WriteString("*Total: " + doc.GrandTotal + "*\n")
	if doc.HasRecurring {
		t.WriteString("Sekali bayar: " + doc.OneTimeTotal + "\n")
		for _, r := range doc.RecurringTotals {
			t.WriteString("Berulang: " + r.Amount + "\n")
		}
		t.WriteString("*Tagihan pertama: " + doc.FirstInvoiceTotal + "*\n")
	}
	if doc.ValidUntil != "" && doc.ValidUntil != "-" {
		t.WriteString("Berlaku s.d. " + doc.ValidUntil + "\n")
	}
	if link != "" {
		t.WriteString("\nLihat & setujui penawaran: " + link + "\n")
	}
	t.WriteString("\n" + doc.Issuer.Name)

	e := html.EscapeString
	var h strings.Builder
	h.WriteString("<p>" + e(opening) + "</p>")
	h.WriteString(`<p><strong>Penawaran ` + e(doc.Number) + `</strong></p>`)
	h.WriteString(`<table cellpadding="6" cellspacing="0" border="1" style="border-collapse:collapse;font-size:14px">`)
	h.WriteString("<tr><th>No</th><th>Deskripsi</th><th>Qty</th><th>Harga</th><th>Diskon</th><th>Pajak</th><th>Jumlah</th></tr>")
	for _, l := range doc.Lines {
		h.WriteString("<tr><td>" + itoa(l.No) + "</td><td>" + e(l.Description) + "</td><td>" + e(l.Quantity) + "</td><td>" +
			e(l.UnitPrice) + "</td><td>" + e(l.Discount) + "</td><td>" + e(l.Tax) + `</td><td style="text-align:right">` + e(l.Total) + "</td></tr>")
	}
	h.WriteString("</table>")
	h.WriteString("<p>Subtotal: " + e(doc.Subtotal) + "<br>Diskon: " + e(doc.DiscountTotal) + "<br>Pajak: " + e(doc.TaxTotal) +
		"<br><strong>Total: " + e(doc.GrandTotal) + "</strong></p>")
	if doc.HasRecurring {
		h.WriteString("<p>Sekali bayar: " + e(doc.OneTimeTotal))
		for _, r := range doc.RecurringTotals {
			h.WriteString("<br>Berulang: " + e(r.Amount))
		}
		h.WriteString("<br><strong>Tagihan pertama: " + e(doc.FirstInvoiceTotal) + "</strong></p>")
	}
	if doc.ValidUntil != "" && doc.ValidUntil != "-" {
		h.WriteString("<p>Berlaku s.d. " + e(doc.ValidUntil) + "</p>")
	}
	if link != "" {
		h.WriteString(`<p><a href="` + e(link) + `">Lihat &amp; setujui penawaran</a></p>`)
	}
	h.WriteString("<p>" + e(doc.Issuer.Name) + "</p>")

	return Summary{Subject: "Penawaran " + doc.Number + " – " + doc.Issuer.Name, Text: t.String(), HTML: h.String()}
}
