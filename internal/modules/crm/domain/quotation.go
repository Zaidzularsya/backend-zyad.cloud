package domain

import (
	"encoding/json"
	"time"

	"zyad.cloud/internal/shared/pricing"
)

type QuotationStatus string

const (
	QuotationStatusDraft    QuotationStatus = "draft"
	QuotationStatusSent     QuotationStatus = "sent"
	QuotationStatusApproved QuotationStatus = "approved"
	QuotationStatusRejected QuotationStatus = "rejected"
	QuotationStatusExpired  QuotationStatus = "expired"
	// Superseded: versi lama yang sudah digantikan revisi.
	QuotationStatusSuperseded QuotationStatus = "superseded"
	// Customer meminta revisi lewat link publik; sales memutuskan revisi/tolak.
	QuotationStatusRevisionRequested QuotationStatus = "revision_requested"
)

func (s QuotationStatus) IsValid() bool {
	switch s {
	case QuotationStatusDraft, QuotationStatusSent, QuotationStatusApproved, QuotationStatusRejected, QuotationStatusExpired, QuotationStatusSuperseded, QuotationStatusRevisionRequested:
		return true
	default:
		return false
	}
}

// Item hanya bisa diganti selama quotation berstatus draft (QuotationRepository.ReplaceItems).
// FeatureSnapshot adalah fitur produk yang disalin ke baris quotation saat baris
// dibuat; berubahnya fitur di katalog tidak memengaruhi baris yang sudah ada.
type FeatureSnapshot struct {
	FeatureKey string          `json:"feature_key"`
	Value      json.RawMessage `json:"value"`
	Label      string          `json:"label"` // "" = tidak ditampilkan
}

type QuotationItem struct {
	ID              string
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent *string
	LineTotal       string
	Position        int
	// Snapshot katalog saat baris ditambahkan; ProductID nil = baris bebas
	// atau produk sudah dihapus.
	ProductID  *string
	SKU        string
	Unit       string
	TaxPercent string
	TaxAmount  string
	// Pricing: snapshot atribut harga baris (default one_time + prepaid).
	Pricing pricing.Attributes
	// Features: snapshot fitur produk (kosong untuk baris bebas); tidak pernah nil dari repository.
	Features []FeatureSnapshot
}

// Quotation money fields (Subtotal/DiscountTotal/TaxTotal/GrandTotal) are
// decimal strings, computed once at creation time by
// QuotationService.computeTotals from Items — see that method's doc comment
// for the simplification this implies (float64 arithmetic, not a decimal
// library) versus the ::text-cast-only convention used elsewhere in this
// module for stored values.
type Quotation struct {
	ID              string
	OrganizationID  string
	DealID          *string
	ContactID       *string
	CompanyID       *string
	QuotationNumber string
	Status          QuotationStatus
	ValidUntil      *time.Time
	Subtotal        string
	DiscountTotal   string
	TaxTotal        string
	GrandTotal      string
	// Rincian turunan dari item (line_total + pajak per baris). RecurringTotals
	// kosong = tidak ada baris berulang.
	OneTimeTotal      string
	FirstInvoiceTotal string
	RecurringTotals   map[pricing.Frequency]string
	Currency          string
	Notes             string
	SentAt            *time.Time
	ApprovedAt        *time.Time
	RejectedAt        *time.Time
	Items             []QuotationItem
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
	RevisionOfID      *string
	RevisionNo        int
	PDFAssetID        *string
	PDFGeneratedAt    *time.Time
	// Channel: "" = dibuat sales; QuotationChannelSelfServe = checkout self-serve.
	Channel string
}

const QuotationChannelSelfServe = "self_serve"
