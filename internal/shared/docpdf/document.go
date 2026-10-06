// Package docpdf merender dokumen bisnis (penawaran, invoice) ke PDF dari
// view-model generik yang sudah berisi teks siap cetak.
package docpdf

type Party struct{ Name, Company, Email, Phone, Address string }

type Line struct {
	No                                                  int
	Description, Billing, Quantity, UnitPrice, Discount string
	Tax, Total                                          string
}

type Row struct {
	Label, Value string
	Bold         bool
}

type Document struct {
	Title, Number string
	Issuer        Party
	Customer      Party
	CustomerLabel string // "Kepada" | "Tagihan untuk"
	Meta          []Row  // kanan atas, mis. Nomor/Tanggal/Berlaku s.d.
	Lines         []Line
	Totals        []Row
	Notes         string
	Stamp         string // "" | "DRAFT" | "LUNAS" | "DIBATALKAN" — watermark diagonal
	Filename      string
}
