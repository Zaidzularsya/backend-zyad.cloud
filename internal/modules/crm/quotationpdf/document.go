package quotationpdf

import (
	"strings"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
)

type Issuer struct{ Name, Email, Phone, Address string }
type Customer struct{ Name, Company, Email, Phone string }
type Line struct {
	No                                                           int
	Description, Quantity, Unit, UnitPrice, Discount, Tax, Total string
	// Billing: "" untuk sekali bayar + prabayar (default); selain itu ringkasan atribut.
	Billing string
	// Features: label fitur yang terlihat (snapshot baris), urut seperti di katalog.
	Features []string
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
		Title: "PENAWARAN HARGA", Number: q.QuotationNumber, IssuedOn: docpdf.FormatDateID(issuedAt), ValidUntil: "-",
		Issuer: issuer, Customer: customer, Notes: strings.TrimSpace(q.Notes),
		Subtotal: docpdf.FormatRupiah(q.Subtotal), DiscountTotal: docpdf.FormatRupiah(q.DiscountTotal),
		TaxTotal: docpdf.FormatRupiah(q.TaxTotal), GrandTotal: docpdf.FormatRupiah(q.GrandTotal),
		Draft: q.Status == crmdomain.QuotationStatusDraft, Filename: q.QuotationNumber + ".pdf",
	}
	if q.ValidUntil != nil {
		doc.ValidUntil = docpdf.FormatDateID(*q.ValidUntil)
	}
	if doc.Draft {
		doc.StatusLabel = "DRAFT"
	}
	for _, f := range pricing.Frequencies() {
		if amount, ok := q.RecurringTotals[f]; ok {
			doc.RecurringTotals = append(doc.RecurringTotals, RecurringTotal{Label: f.Label(), Amount: docpdf.FormatRupiah(amount) + f.PriceSuffix()})
		}
	}
	if doc.HasRecurring = len(doc.RecurringTotals) > 0; doc.HasRecurring {
		doc.OneTimeTotal = docpdf.FormatRupiah(q.OneTimeTotal)
		doc.FirstInvoiceTotal = docpdf.FormatRupiah(q.FirstInvoiceTotal)
	}
	for i, it := range q.Items {
		discount := "-"
		if it.DiscountPercent != nil {
			discount = docpdf.FormatPercent(*it.DiscountPercent)
		}
		qty := docpdf.FormatQuantity(it.Quantity)
		if it.Unit != "" {
			qty += " " + it.Unit
		}
		unitPrice, billing := docpdf.FormatRupiah(it.UnitPrice), ""
		if it.Pricing.ChargeType == pricing.Recurring {
			unitPrice += it.Pricing.Frequency.PriceSuffix()
		}
		// Default (kosong/one_time + prepaid) tidak diberi keterangan agar dokumen lama tetap sama.
		if it.Pricing.ChargeType == pricing.Recurring || it.Pricing.PaymentTiming == pricing.Postpaid {
			billing = it.Pricing.Short()
		}
		doc.Lines = append(doc.Lines, Line{
			No: i + 1, Description: it.Description, Quantity: qty, Unit: it.Unit,
			UnitPrice: unitPrice, Discount: discount, Tax: docpdf.FormatPercent(it.TaxPercent),
			Total: docpdf.FormatRupiah(it.LineTotal), Billing: billing, Features: visibleFeatures(it.Features),
		})
	}
	return doc
}

func visibleFeatures(in []crmdomain.FeatureSnapshot) []string {
	var out []string
	for _, f := range in {
		if f.Label != "" {
			out = append(out, f.Label)
		}
	}
	return out
}

// PDF memetakan view-model penawaran ke dokumen generik untuk docpdf.Renderer.
func (d Document) PDF() docpdf.Document {
	out := docpdf.Document{
		Title: d.Title, Number: d.Number, CustomerLabel: "Kepada", Notes: d.Notes, Filename: d.Filename,
		Issuer:   docpdf.Party{Name: d.Issuer.Name, Email: d.Issuer.Email, Phone: d.Issuer.Phone, Address: d.Issuer.Address},
		Customer: docpdf.Party{Name: d.Customer.Name, Company: d.Customer.Company, Email: d.Customer.Email, Phone: d.Customer.Phone},
		Meta:     []docpdf.Row{{Label: "Nomor", Value: d.Number}, {Label: "Tanggal", Value: d.IssuedOn}, {Label: "Berlaku s.d.", Value: d.ValidUntil}},
		Totals: []docpdf.Row{
			{Label: "Subtotal", Value: d.Subtotal}, {Label: "Diskon", Value: d.DiscountTotal},
			{Label: "Pajak", Value: d.TaxTotal}, {Label: "Total", Value: d.GrandTotal, Bold: true},
		},
	}
	if d.Draft {
		out.Stamp = "DRAFT"
	}
	for _, r := range totalsBreakdown(d) {
		out.Totals = append(out.Totals, docpdf.Row{Label: r[0], Value: r[1], Bold: r[2] != ""})
	}
	for _, l := range d.Lines {
		out.Lines = append(out.Lines, docpdf.Line{
			No: l.No, Description: l.Description, Billing: l.Billing, Quantity: l.Quantity,
			UnitPrice: l.UnitPrice, Discount: l.Discount, Tax: l.Tax, Total: l.Total, Details: l.Features,
		})
	}
	return out
}
