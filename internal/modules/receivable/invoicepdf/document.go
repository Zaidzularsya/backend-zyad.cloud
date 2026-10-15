// Package invoicepdf mengubah invoice receivable menjadi docpdf.Document siap cetak.
package invoicepdf

import (
	"math/big"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
)

// Build memetakan invoice. now dipakai sebagai tanggal dokumen draft (belum punya tanggal terbit).
func Build(inv domain.Invoice, issuer docpdf.Party, now time.Time) docpdf.Document {
	number, filename := inv.InvoiceNumber, inv.InvoiceNumber+".pdf"
	if number == "" {
		number, filename = "-", "invoice-draft.pdf"
	}
	issued, due := docpdf.FormatDateID(now), "-"
	if inv.IssueDate != nil {
		issued = docpdf.FormatDateID(*inv.IssueDate)
	}
	if inv.DueDate != nil {
		due = docpdf.FormatDateID(*inv.DueDate)
	}
	meta := []docpdf.Row{{Label: "Nomor", Value: number}, {Label: "Tanggal", Value: issued}, {Label: "Jatuh tempo", Value: due}}
	if period := periodLabel(inv.PeriodStart, inv.PeriodEnd); period != "" {
		meta = append(meta, docpdf.Row{Label: "Periode", Value: period})
	}

	doc := docpdf.Document{
		Title: "INVOICE", Number: number, Issuer: issuer, CustomerLabel: "Tagihan untuk", Meta: meta,
		Customer: docpdf.Party{Name: inv.Account.Name, Company: inv.Account.CompanyName, Email: inv.Account.Email, Phone: inv.Account.Phone, Address: inv.Account.Address},
		Notes:    inv.Notes, Filename: filename, Stamp: stamp(inv.Status),
		Totals: []docpdf.Row{
			{Label: "Subtotal", Value: docpdf.FormatRupiah(inv.Subtotal)},
			{Label: "Diskon", Value: docpdf.FormatRupiah(inv.DiscountTotal)},
			{Label: "Pajak", Value: docpdf.FormatRupiah(inv.TaxTotal)},
			{Label: "Total", Value: docpdf.FormatRupiah(inv.GrandTotal), Bold: true},
		},
	}
	if paid, ok := new(big.Rat).SetString(inv.AmountPaid); ok && paid.Sign() > 0 {
		doc.Totals = append(doc.Totals,
			docpdf.Row{Label: "Dibayar", Value: docpdf.FormatRupiah(inv.AmountPaid)},
			docpdf.Row{Label: "Sisa", Value: docpdf.FormatRupiah(inv.Balance()), Bold: true})
	}
	for i, it := range inv.Items {
		discount := "-"
		if it.DiscountPercent != "" {
			discount = docpdf.FormatPercent(it.DiscountPercent)
		}
		qty := docpdf.FormatQuantity(it.Quantity)
		if it.Unit != "" {
			qty += " " + it.Unit
		}
		unitPrice := docpdf.FormatRupiah(it.UnitPrice)
		if it.Pricing.ChargeType == pricing.Recurring {
			unitPrice += it.Pricing.Frequency.PriceSuffix()
		}
		doc.Lines = append(doc.Lines, docpdf.Line{
			No: i + 1, Description: it.Description, Billing: billing(it), Quantity: qty, UnitPrice: unitPrice,
			Discount: discount, Tax: docpdf.FormatPercent(it.TaxPercent), Total: docpdf.FormatRupiah(it.LineTotal),
		})
	}
	return doc
}

func stamp(s domain.InvoiceStatus) string {
	switch s {
	case domain.InvoiceDraft:
		return "DRAFT"
	case domain.InvoicePaid:
		return "LUNAS"
	case domain.InvoiceVoid:
		return "DIBATALKAN"
	}
	return ""
}

func periodLabel(start, end *time.Time) string {
	if start == nil || end == nil {
		return ""
	}
	return docpdf.FormatDateID(*start) + " – " + docpdf.FormatDateID(*end)
}

// billing: keterangan kecil di bawah deskripsi. Baris default (sekali bayar + prabayar, tanpa
// periode) tidak diberi keterangan, sama seperti penawaran.
func billing(it domain.InvoiceItem) string {
	period := periodLabel(it.PeriodStart, it.PeriodEnd)
	isDefault := it.Pricing.ChargeType != pricing.Recurring && it.Pricing.PaymentTiming != pricing.Postpaid
	if isDefault && period == "" {
		return ""
	}
	out := it.Pricing.Short()
	if period != "" {
		out += " · " + period
	}
	return out
}
