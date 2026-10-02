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

func BuildSummary(doc Document, opening string) Summary {
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
	if doc.ValidUntil != "" && doc.ValidUntil != "-" {
		t.WriteString("Berlaku s.d. " + doc.ValidUntil + "\n")
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
	if doc.ValidUntil != "" && doc.ValidUntil != "-" {
		h.WriteString("<p>Berlaku s.d. " + e(doc.ValidUntil) + "</p>")
	}
	h.WriteString("<p>" + e(doc.Issuer.Name) + "</p>")

	return Summary{Subject: "Penawaran " + doc.Number + " – " + doc.Issuer.Name, Text: t.String(), HTML: h.String()}
}
