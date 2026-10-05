package quotationpdf

import (
	"strings"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/pricing"
)

type Issuer struct{ Name, Email, Phone, Address string }
type Customer struct{ Name, Company, Email, Phone string }
type Line struct {
	No                                                           int
	Description, Quantity, Unit, UnitPrice, Discount, Tax, Total string
	// Billing: "" untuk sekali bayar + prabayar (default); selain itu ringkasan atribut.
	Billing string
}

// RecurringTotal adalah satu baris total berulang, mis. {"Bulanan", "Rp 333.000/bulan"}.
type RecurringTotal struct{ Label, Amount string }
type Document struct {
	Title, Number, StatusLabel, IssuedOn, ValidUntil string
	Issuer                                           Issuer
	Customer                                         Customer
	Lines                                            []Line
	Subtotal, DiscountTotal, TaxTotal, GrandTotal    string
	Notes                                            string
	// Rincian harga; hanya bermakna bila HasRecurring.
	OneTimeTotal      string
	RecurringTotals   []RecurringTotal // urut pricing.Frequencies()
	FirstInvoiceTotal string
	HasRecurring      bool
	Draft             bool
	Filename          string
}

// BuildDocument mengubah quotation menjadi teks siap cetak. issuedAt =
// sent_at untuk dokumen final, waktu render untuk draft.
func BuildDocument(q crmdomain.Quotation, issuer Issuer, customer Customer, issuedAt time.Time) Document {
	doc := Document{
		Title: "PENAWARAN HARGA", Number: q.QuotationNumber, IssuedOn: FormatDateID(issuedAt), ValidUntil: "-",
		Issuer: issuer, Customer: customer, Notes: strings.TrimSpace(q.Notes),
		Subtotal: FormatRupiah(q.Subtotal), DiscountTotal: FormatRupiah(q.DiscountTotal),
		TaxTotal: FormatRupiah(q.TaxTotal), GrandTotal: FormatRupiah(q.GrandTotal),
		Draft: q.Status == crmdomain.QuotationStatusDraft, Filename: q.QuotationNumber + ".pdf",
	}
	if q.ValidUntil != nil {
		doc.ValidUntil = FormatDateID(*q.ValidUntil)
	}
	if doc.Draft {
		doc.StatusLabel = "DRAFT"
	}
	for _, f := range pricing.Frequencies() {
		if amount, ok := q.RecurringTotals[f]; ok {
			doc.RecurringTotals = append(doc.RecurringTotals, RecurringTotal{Label: f.Label(), Amount: FormatRupiah(amount) + f.PriceSuffix()})
		}
	}
	if doc.HasRecurring = len(doc.RecurringTotals) > 0; doc.HasRecurring {
		doc.OneTimeTotal = FormatRupiah(q.OneTimeTotal)
		doc.FirstInvoiceTotal = FormatRupiah(q.FirstInvoiceTotal)
	}
	for i, it := range q.Items {
		discount := "-"
		if it.DiscountPercent != nil {
			discount = FormatPercent(*it.DiscountPercent)
		}
		qty := FormatQuantity(it.Quantity)
		if it.Unit != "" {
			qty += " " + it.Unit
		}
		unitPrice, billing := FormatRupiah(it.UnitPrice), ""
		if it.Pricing.ChargeType == pricing.Recurring {
			unitPrice += it.Pricing.Frequency.PriceSuffix()
		}
		// Default (kosong/one_time + prepaid) tidak diberi keterangan agar dokumen lama tetap sama.
		if it.Pricing.ChargeType == pricing.Recurring || it.Pricing.PaymentTiming == pricing.Postpaid {
			billing = it.Pricing.Short()
		}
		doc.Lines = append(doc.Lines, Line{
			No: i + 1, Description: it.Description, Quantity: qty, Unit: it.Unit,
			UnitPrice: unitPrice, Discount: discount, Tax: FormatPercent(it.TaxPercent),
			Total: FormatRupiah(it.LineTotal), Billing: billing,
		})
	}
	return doc
}
