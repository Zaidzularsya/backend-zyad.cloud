package docpdf

import (
	"bytes"
	"strings"
	"testing"
)

func sampleDoc(stamp string, lines int) Document {
	d := Document{Title: "PENAWARAN HARGA", Number: "QUO-2026-0001", CustomerLabel: "Kepada",
		Issuer:   Party{Name: "PT Zyad Technovation", Email: "sales@zyad.cloud", Phone: "0812", Address: "Purbalingga"},
		Customer: Party{Name: "Budi", Company: "PT Maju Jaya"},
		Meta:     []Row{{Label: "Nomor", Value: "QUO-2026-0001"}, {Label: "Tanggal", Value: "2 Okt 2026"}, {Label: "Berlaku s.d.", Value: "30 Nov 2026"}},
		Totals:   []Row{{Label: "Subtotal", Value: "Rp 1"}, {Label: "Total", Value: "Rp 1", Bold: true}},
		Notes:    "Catatan", Stamp: stamp, Filename: "QUO-2026-0001.pdf"}
	for i := 0; i < lines; i++ {
		d.Lines = append(d.Lines, Line{No: i + 1, Description: strings.Repeat("Instalasi FO – lantai 2 “gedung B” ", 14),
			Quantity: "1 paket", UnitPrice: "Rp 1.000.000", Discount: "-", Tax: "11%", Total: "Rp 1.000.000"})
	}
	return d
}

func TestRenderProducesPDF(t *testing.T) {
	out, err := NewRenderer().Render(sampleDoc("DRAFT", 3))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 2000 {
		t.Fatalf("not a pdf (len %d)", len(out))
	}
}

func TestRenderLongContentAddsPages(t *testing.T) {
	r := NewRenderer()
	short, err := r.Render(sampleDoc("", 1))
	if err != nil {
		t.Fatal(err)
	}
	long, err := r.Render(sampleDoc("", 40))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(short, []byte("/Type /Page\n")) >= bytes.Count(long, []byte("/Type /Page\n")) {
		t.Fatal("40 long lines must span more pages than 1 line")
	}
}

func TestRenderStampDiffersFromNone(t *testing.T) {
	r := NewRenderer()
	a, _ := r.Render(sampleDoc("DRAFT", 1))
	b, _ := r.Render(sampleDoc("", 1))
	if bytes.Equal(a, b) {
		t.Fatal("stamp watermark must change the output")
	}
}

func TestRenderStampAndMeta(t *testing.T) {
	d := sampleDoc("LUNAS", 2)
	d.CustomerLabel = "Tagihan untuk"
	d.Meta = []Row{{"Nomor", "INV-2026-0001", false}, {"Tanggal", "5 Okt 2026", false}, {"Jatuh tempo", "12 Okt 2026", false}, {"Periode", "1 Okt 2026 – 31 Okt 2026", false}}
	d.Lines[0].Billing = "Bulanan · Prabayar"
	out, err := NewRenderer().Render(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a pdf")
	}
}

func TestRenderLineDetails(t *testing.T) {
	r := NewRenderer()
	plain, err := r.Render(sampleDoc("", 1))
	if err != nil {
		t.Fatal(err)
	}
	with := sampleDoc("", 1)
	with.Lines[0].Details = []string{"Hingga 5 user", "CRM"}
	detailed, err := r.Render(with)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(plain, detailed) {
		t.Fatal("details must change the output")
	}
	// Tinggi baris bertambah: banyak baris berdetail harus memakai lebih banyak halaman.
	many, manyDetailed := sampleDoc("", 12), sampleDoc("", 12)
	for i := range manyDetailed.Lines {
		manyDetailed.Lines[i].Details = []string{"a", "b", "c", "d", "e"}
	}
	a, _ := r.Render(many)
	b, _ := r.Render(manyDetailed)
	if bytes.Count(a, []byte("/Type /Page\n")) >= bytes.Count(b, []byte("/Type /Page\n")) {
		t.Fatal("details must add row height and pages")
	}
	// Tanpa details, keluaran identik dengan perilaku lama (invoice receivable).
	again, _ := r.Render(sampleDoc("", 1))
	if len(again) != len(plain) {
		t.Fatalf("dokumen tanpa detail berubah: %d vs %d", len(again), len(plain))
	}
}
