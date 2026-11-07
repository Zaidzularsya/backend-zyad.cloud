package service

import (
	"html"
	"strings"

	"zyad.cloud/internal/shared/docpdf"
)

// IsolatedMessage builds the notice sent when internet service is suspended
// for unpaid invoices. It carries no tenant-internal or router details. A
// missing entry in payLinks only omits that invoice's link line.
func IsolatedMessage(tenantName, customerName string, invoices []TriggerInvoice, payLinks map[string]string) (text, htmlBody, subject string) {
	subject = "Layanan internet dihentikan sementara"
	if tenantName != "" {
		subject += " – " + tenantName
	}
	lines := []string{
		"Halo " + greetName(customerName) + ", layanan internet Anda dihentikan sementara karena ada tagihan yang belum dibayar:",
	}
	var b strings.Builder
	b.WriteString("<p>" + html.EscapeString(lines[0]) + "</p>")
	for _, inv := range invoices {
		line := "- Invoice " + inv.Number + ", jatuh tempo " + docpdf.FormatDateID(inv.DueDate)
		lines = append(lines, line)
		b.WriteString("<p>" + html.EscapeString(line))
		if link := payLinks[inv.ID]; link != "" {
			lines = append(lines, "  Lihat & bayar: "+link)
			b.WriteString(`<br>Lihat &amp; bayar: <a href="` + html.EscapeString(link) + `">` + html.EscapeString(link) + "</a>")
		}
		b.WriteString("</p>")
	}
	closing := "Layanan akan aktif kembali setelah pembayaran kami terima."
	lines = append(lines, closing)
	b.WriteString("<p>" + html.EscapeString(closing) + "</p>")
	return strings.Join(lines, "\n"), b.String(), subject
}

// RestoredMessage builds the notice sent when service is reactivated.
func RestoredMessage(tenantName, customerName string) (text, htmlBody, subject string) {
	subject = "Layanan internet aktif kembali"
	if tenantName != "" {
		subject += " – " + tenantName
	}
	text = "Halo " + greetName(customerName) + ", layanan internet Anda sudah aktif kembali. Terima kasih atas pembayarannya."
	return text, "<p>" + html.EscapeString(text) + "</p>", subject
}

func greetName(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "Bapak/Ibu"
}
