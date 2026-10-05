package quotationpdf

import (
	"testing"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
)

func TestBuildDocument(t *testing.T) {
	until := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	disc := "10.00"
	q := crmdomain.Quotation{
		QuotationNumber: "QUO-2026-0007-R1", Status: crmdomain.QuotationStatusDraft, ValidUntil: &until,
		Subtotal: "1050000.00", DiscountTotal: "35000.00", TaxTotal: "38500.00", GrandTotal: "1053500.00", Notes: "Harga belum termasuk ongkir.",
		Items: []crmdomain.QuotationItem{
			{Description: "Internet 50 Mbps", Quantity: "1.00", Unit: "bulan", UnitPrice: "350000.00", DiscountPercent: &disc, TaxPercent: "11.00", LineTotal: "315000.00"},
			{Description: "Instalasi", Quantity: "1.00", UnitPrice: "700000.00", TaxPercent: "0.00", LineTotal: "700000.00"},
		},
	}
	doc := BuildDocument(q, Issuer{Name: "PT Zyad"}, Customer{Name: "Budi", Company: "PT Maju"}, time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC))
	if !doc.Draft || doc.Number != "QUO-2026-0007-R1" || doc.Filename != "QUO-2026-0007-R1.pdf" || doc.ValidUntil != "30 Nov 2026" || doc.IssuedOn != "2 Okt 2026" {
		t.Fatalf("header = %+v", doc)
	}
	l := doc.Lines[0]
	if l.No != 1 || l.Quantity != "1 bulan" || l.UnitPrice != "Rp 350.000" || l.Discount != "10%" || l.Tax != "11%" || l.Total != "Rp 315.000" {
		t.Fatalf("line = %+v", l)
	}
	if doc.Lines[1].Discount != "-" || doc.GrandTotal != "Rp 1.053.500" {
		t.Fatalf("doc = %+v", doc)
	}
}
