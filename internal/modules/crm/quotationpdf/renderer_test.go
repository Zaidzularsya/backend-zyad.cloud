package quotationpdf

import (
	"bytes"
	"strings"
	"testing"
)

func sampleDoc(draft bool, lines int) Document {
	d := Document{Title: "PENAWARAN HARGA", Number: "QUO-2026-0001", IssuedOn: "2 Okt 2026", ValidUntil: "30 Nov 2026",
		Issuer:   Issuer{Name: "PT Zyad Technovation", Email: "sales@zyad.cloud", Phone: "0812", Address: "Purbalingga"},
		Customer: Customer{Name: "Budi", Company: "PT Maju Jaya"}, Subtotal: "Rp 1", DiscountTotal: "Rp 0", TaxTotal: "Rp 0", GrandTotal: "Rp 1",
		Notes: "Catatan", Draft: draft, Filename: "QUO-2026-0001.pdf"}
	for i := 0; i < lines; i++ {
		d.Lines = append(d.Lines, Line{No: i + 1, Description: strings.Repeat("Instalasi FO – lantai 2 “gedung B” ", 14),
			Quantity: "1 paket", UnitPrice: "Rp 1.000.000", Discount: "-", Tax: "11%", Total: "Rp 1.000.000"})
	}
	return d
}

func TestRenderProducesPDF(t *testing.T) {
	r := NewRenderer()
	out, err := r.Render(sampleDoc(true, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 2000 {
		t.Fatalf("not a pdf (len %d)", len(out))
	}
}

func TestRenderLongContentAddsPages(t *testing.T) {
	r := NewRenderer()
	short, err := r.Render(sampleDoc(false, 1))
	if err != nil {
		t.Fatal(err)
	}
	long, err := r.Render(sampleDoc(false, 40))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(short, []byte("/Type /Page\n")) >= bytes.Count(long, []byte("/Type /Page\n")) {
		t.Fatal("40 long lines must span more pages than 1 line")
	}
}

func TestRenderDraftDiffersFromFinal(t *testing.T) {
	r := NewRenderer()
	a, _ := r.Render(sampleDoc(true, 1))
	b, _ := r.Render(sampleDoc(false, 1))
	if bytes.Equal(a, b) {
		t.Fatal("draft watermark must change the output")
	}
}
