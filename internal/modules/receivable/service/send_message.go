package service

import (
	"context"
	"html"
	"log/slog"
	"math/big"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/docpdf"
)

type composed struct {
	subject, text, html string
	attachment          *Attachment
}

// compose menyiapkan isi pesan + lampiran PDF. failure berisi alasan aman bila PDF tidak tersedia.
func (s *sendService) compose(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, opening, senderUserID string) (composed, string) {
	pdf, err := s.Docs.PDF(ctx, scope, inv.ID)
	if err != nil {
		slog.WarnContext(ctx, "receivable: invoice pdf unavailable for sending", "invoice_id", inv.ID, "error", err)
		return composed{}, reasonPDFMissing
	}
	link, _, err := s.Docs.Link(ctx, scope, inv.ID, senderUserID)
	if err != nil {
		slog.WarnContext(ctx, "receivable: invoice link unavailable, sending without it", "invoice_id", inv.ID, "error", err)
		link = ""
	}
	tenantName := ""
	if s.Issuers != nil {
		if party, err := s.Issuers.OrganizationProfile(ctx, scope); err == nil {
			tenantName = party.Name
		}
	}
	canPay := s.Entitlements != nil && s.Entitlements.RequireFeature(ctx, scope.OrganizationID(), "receivable.online_payment") == nil
	subject, text, body := buildInvoiceMessage(inv, tenantName, link, strings.TrimSpace(opening), canPay)
	return composed{
		subject: subject, text: text, html: body,
		attachment: &Attachment{Filename: pdf.Filename, MimeType: "application/pdf", Content: pdf.Content},
	}, ""
}

// buildInvoiceMessage membangun subjek, teks polos (WhatsApp), dan HTML (email, semua nilai di-escape).
// opening kosong → kalimat pembuka standar.
func buildInvoiceMessage(inv domain.Invoice, tenantName, link, opening string, canPay bool) (subject, text, htmlBody string) {
	subject = "Invoice " + inv.InvoiceNumber
	if tenantName != "" {
		subject += " – " + tenantName
	}
	if opening == "" {
		opening = defaultOpening(inv)
	}
	lines := []string{opening}
	if inv.PeriodStart != nil && inv.PeriodEnd != nil {
		lines = append(lines, "Periode: "+docpdf.FormatDateID(*inv.PeriodStart)+" – "+docpdf.FormatDateID(*inv.PeriodEnd))
	}
	if paid, ok := new(big.Rat).SetString(inv.AmountPaid); ok && paid.Sign() > 0 && inv.Status != domain.InvoicePaid {
		lines = append(lines, "Sisa tagihan: "+docpdf.FormatRupiah(inv.Balance()))
	}
	linkLabel := ""
	if link != "" {
		linkLabel = "Lihat invoice: "
		if canPay && inv.Status != domain.InvoicePaid {
			linkLabel = "Lihat & bayar: "
		}
	}

	text = strings.Join(lines, "\n")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(l), "\n", "<br>") + "</p>")
	}
	if linkLabel != "" {
		text += "\n" + linkLabel + link
		b.WriteString("<p>" + html.EscapeString(linkLabel) + `<a href="` + html.EscapeString(link) + `">` + html.EscapeString(link) + "</a></p>")
	}
	return subject, text, b.String()
}

func defaultOpening(inv domain.Invoice) string {
	name := strings.TrimSpace(inv.Account.Name)
	if name == "" {
		name = "Bapak/Ibu"
	}
	amount := docpdf.FormatRupiah(inv.GrandTotal)
	if inv.Status == domain.InvoicePaid {
		return "Halo " + name + ", terima kasih. Invoice " + inv.InvoiceNumber + " sebesar " + amount + " sudah lunas."
	}
	due := ""
	if inv.DueDate != nil {
		due = ", jatuh tempo " + docpdf.FormatDateID(*inv.DueDate)
	}
	return "Halo " + name + ", berikut invoice " + inv.InvoiceNumber + " sebesar " + amount + due + "."
}
