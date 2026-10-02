package quotationpdf

import (
	"strings"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
)

type Issuer struct{ Name, Email, Phone, Address string }
type Customer struct{ Name, Company, Email, Phone string }
type Line struct {
	No                                                           int
	Description, Quantity, Unit, UnitPrice, Discount, Tax, Total string
}
type Document struct {
	Title, Number, StatusLabel, IssuedOn, ValidUntil string
	Issuer                                           Issuer
	Customer                                         Customer
	Lines                                            []Line
	Subtotal, DiscountTotal, TaxTotal, GrandTotal    string
	Notes                                            string
	Draft                                            bool
	Filename                                         string
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
	for i, it := range q.Items {
		discount := "-"
		if it.DiscountPercent != nil {
			discount = FormatPercent(*it.DiscountPercent)
		}
		qty := FormatQuantity(it.Quantity)
		if it.Unit != "" {
			qty += " " + it.Unit
		}
		doc.Lines = append(doc.Lines, Line{
			No: i + 1, Description: it.Description, Quantity: qty, Unit: it.Unit,
			UnitPrice: FormatRupiah(it.UnitPrice), Discount: discount, Tax: FormatPercent(it.TaxPercent),
			Total: FormatRupiah(it.LineTotal),
		})
	}
	return doc
}
