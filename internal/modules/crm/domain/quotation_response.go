package domain

import "time"

type QuotationResponseAction string

const (
	QuotationResponseApproved          QuotationResponseAction = "approved"
	QuotationResponseRevisionRequested QuotationResponseAction = "revision_requested"
)

type RevisionCategory string

const (
	RevisionPrice         RevisionCategory = "price"
	RevisionQuantity      RevisionCategory = "quantity"
	RevisionItems         RevisionCategory = "items"
	RevisionSpecification RevisionCategory = "specification"
	RevisionSchedule      RevisionCategory = "schedule"
	RevisionPaymentTerms  RevisionCategory = "payment_terms"
	RevisionValidity      RevisionCategory = "validity"
	RevisionOther         RevisionCategory = "other"
)

var revisionCategoryLabels = []struct {
	Code  RevisionCategory
	Label string
}{
	{RevisionPrice, "Harga/diskon"},
	{RevisionQuantity, "Kuantitas"},
	{RevisionItems, "Item/produk"},
	{RevisionSpecification, "Spesifikasi/deskripsi"},
	{RevisionSchedule, "Jadwal pelaksanaan"},
	{RevisionPaymentTerms, "Syarat pembayaran"},
	{RevisionValidity, "Masa berlaku"},
	{RevisionOther, "Lainnya"},
}

// RevisionCategories mengembalikan semua kategori dalam urutan tampilan.
func RevisionCategories() []RevisionCategory {
	out := make([]RevisionCategory, 0, len(revisionCategoryLabels))
	for _, c := range revisionCategoryLabels {
		out = append(out, c.Code)
	}
	return out
}

func (c RevisionCategory) IsValid() bool {
	for _, k := range revisionCategoryLabels {
		if k.Code == c {
			return true
		}
	}
	return false
}

func (c RevisionCategory) Label() string {
	for _, k := range revisionCategoryLabels {
		if k.Code == c {
			return k.Label
		}
	}
	return string(c)
}

// QuotationResponse adalah satu respons customer atas penawaran lewat link publik.
type QuotationResponse struct {
	ID, QuotationID, LinkID string
	Action                  QuotationResponseAction
	Categories              []RevisionCategory
	Note, ResponderName     string
	IP, UserAgent           string
	CreatedAt               time.Time
}
