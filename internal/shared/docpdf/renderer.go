package docpdf

import (
	"bytes"
	_ "embed"
	"time"

	"codeberg.org/go-pdf/fpdf"
)

//go:embed fonts/NotoSans-Regular.ttf
var fontRegular []byte

//go:embed fonts/NotoSans-Bold.ttf
var fontBold []byte

type Renderer struct{ now func() time.Time }

func NewRenderer() *Renderer { return &Renderer{now: time.Now} }

const (
	pageW   = 210.0
	margin  = 15.0
	content = pageW - 2*margin
)

// Kolom tabel: No | Deskripsi | Qty | Harga | Diskon | Pajak | Jumlah (total = content).
var cols = []float64{8, 72, 20, 26, 14, 12, 28}

func (r *Renderer) Render(doc Document) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(margin, margin, margin)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddUTF8FontFromBytes("Noto", "", fontRegular)
	pdf.AddUTF8FontFromBytes("Noto", "B", fontBold)
	stamp := r.now()
	pdf.SetCreationDate(stamp)
	pdf.SetModificationDate(stamp)
	pdf.SetTitle(doc.Title+" "+doc.Number, true)
	pdf.AliasNbPages("{nb}")

	pdf.SetHeaderFunc(func() {
		if doc.Stamp != "" {
			watermark(pdf, doc.Stamp)
		}
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Noto", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(content/2, 5, doc.Number, "", 0, "L", false, 0, "")
		pdf.CellFormat(content/2, 5, "Halaman "+itoa(pdf.PageNo())+"/{nb}", "", 0, "R", false, 0, "")
	})
	pdf.AddPage()

	// Penerbit (kiri) & judul (kanan)
	pdf.SetTextColor(20, 20, 20)
	pdf.SetFont("Noto", "B", 13)
	pdf.CellFormat(content*0.6, 7, doc.Issuer.Name, "", 0, "L", false, 0, "")
	pdf.SetFont("Noto", "B", 15)
	pdf.CellFormat(content*0.4, 7, doc.Title, "", 1, "R", false, 0, "")
	pdf.SetFont("Noto", "", 9)
	for _, line := range []string{doc.Issuer.Address, joinNonEmpty(" · ", doc.Issuer.Phone, doc.Issuer.Email)} {
		if line != "" {
			pdf.MultiCell(content*0.6, 4.5, line, "", "L", false)
		}
	}
	pdf.Ln(4)

	// Info dokumen & pelanggan
	top := pdf.GetY()
	pdf.SetFont("Noto", "B", 9)
	customerLabel := doc.CustomerLabel
	if customerLabel == "" {
		customerLabel = "Kepada"
	}
	pdf.CellFormat(content*0.5, 5, customerLabel, "", 1, "L", false, 0, "")
	pdf.SetFont("Noto", "", 9)
	for _, line := range []string{doc.Customer.Name, doc.Customer.Company, joinNonEmpty(" · ", doc.Customer.Phone, doc.Customer.Email)} {
		if line != "" {
			pdf.MultiCell(content*0.5, 4.5, line, "", "L", false)
		}
	}
	leftBottom := pdf.GetY()
	pdf.SetXY(margin+content*0.55, top)
	for _, kv := range doc.Meta {
		pdf.SetX(margin + content*0.55)
		pdf.SetFont("Noto", "", 9)
		pdf.CellFormat(25, 5, kv.Label, "", 0, "L", false, 0, "")
		pdf.SetFont("Noto", "B", 9)
		pdf.CellFormat(content*0.45-25, 5, kv.Value, "", 1, "R", false, 0, "")
	}
	if pdf.GetY() < leftBottom {
		pdf.SetY(leftBottom)
	}
	pdf.Ln(5)

	// Tabel item
	header := []string{"No", "Deskripsi", "Qty", "Harga", "Diskon", "Pajak", "Jumlah"}
	drawHeader := func() {
		pdf.SetFont("Noto", "B", 8.5)
		pdf.SetFillColor(240, 240, 240)
		for i, h := range header {
			align := "L"
			if i >= 2 {
				align = "R"
			}
			pdf.CellFormat(cols[i], 7, h, "B", 0, align, true, 0, "")
		}
		pdf.Ln(-1)
	}
	drawHeader()
	pdf.SetFont("Noto", "", 8.5)
	const lineH = 4.5
	for _, l := range doc.Lines {
		descLines := pdf.SplitText(l.Description, cols[1]-2)
		rowH := float64(len(descLines))*lineH + 2
		if l.Billing != "" {
			rowH += 3.5
		}
		if pdf.GetY()+rowH > 297-20 {
			pdf.AddPage()
			drawHeader()
			pdf.SetFont("Noto", "", 8.5)
		}
		x, y := pdf.GetX(), pdf.GetY()
		values := []string{itoa(l.No), "", l.Quantity, l.UnitPrice, l.Discount, l.Tax, l.Total}
		cx := x
		for i, v := range values {
			if i == 1 {
				pdf.SetXY(cx+1, y+1)
				for _, dl := range descLines {
					pdf.SetX(cx + 1)
					pdf.CellFormat(cols[1]-2, lineH, dl, "", 2, "L", false, 0, "")
				}
				if l.Billing != "" {
					pdf.SetFont("Noto", "", 7)
					pdf.SetTextColor(120, 120, 120)
					pdf.SetX(cx + 1)
					pdf.CellFormat(cols[1]-2, 3.5, l.Billing, "", 2, "L", false, 0, "")
					pdf.SetTextColor(20, 20, 20)
					pdf.SetFont("Noto", "", 8.5)
				}
			} else {
				pdf.SetXY(cx, y+1)
				align := "R"
				if i == 0 {
					align = "L"
				}
				pdf.CellFormat(cols[i], lineH, v, "", 0, align, false, 0, "")
			}
			cx += cols[i]
		}
		pdf.SetDrawColor(225, 225, 225)
		pdf.Line(x, y+rowH, x+content, y+rowH)
		pdf.SetXY(x, y+rowH)
	}
	pdf.Ln(3)

	// Ringkasan
	for _, kv := range doc.Totals {
		pdf.SetX(margin + content - 80)
		style := ""
		if kv.Bold {
			style = "B"
		}
		pdf.SetFont("Noto", style, 9.5)
		pdf.CellFormat(40, 6, kv.Label, "", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, kv.Value, "", 1, "R", false, 0, "")
	}

	if doc.Notes != "" {
		pdf.Ln(4)
		pdf.SetFont("Noto", "B", 9)
		pdf.CellFormat(content, 5, "Catatan", "", 1, "L", false, 0, "")
		pdf.SetFont("Noto", "", 9)
		pdf.MultiCell(content, 4.5, doc.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// watermark mencetak teks diagonal di tengah halaman; font dikecilkan
// untuk teks panjang (mis. "DIBATALKAN") agar tidak keluar halaman.
func watermark(pdf *fpdf.Fpdf, text string) {
	size := 90.0
	pdf.SetFont("Noto", "B", size)
	if w := pdf.GetStringWidth(text); w > 130 {
		size = size * 130 / w
		pdf.SetFont("Noto", "B", size)
	}
	w := pdf.GetStringWidth(text)
	pdf.SetTextColor(235, 235, 235)
	pdf.TransformBegin()
	pdf.TransformRotate(35, 105, 160)
	pdf.Text(105-w/2, 190, text)
	pdf.TransformEnd()
	pdf.SetTextColor(20, 20, 20)
}

func joinNonEmpty(sep string, parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}
