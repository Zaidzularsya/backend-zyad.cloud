package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

// ActivationStatus is the stored lifecycle of a page activation invoice
// (landing_page_activations.status).
type ActivationStatus string

const (
	ActivationStatusOpen ActivationStatus = "open"
	ActivationStatusPaid ActivationStatus = "paid"
	ActivationStatusVoid ActivationStatus = "void"
)

func (s ActivationStatus) IsValid() bool {
	switch s {
	case ActivationStatusOpen, ActivationStatusPaid, ActivationStatusVoid:
		return true
	default:
		return false
	}
}

// ActivationDurationUnit is the unit of the publish period bought by an activation.
type ActivationDurationUnit string

const (
	ActivationDurationDay   ActivationDurationUnit = "day"
	ActivationDurationWeek  ActivationDurationUnit = "week"
	ActivationDurationMonth ActivationDurationUnit = "month"
)

func (u ActivationDurationUnit) IsValid() bool {
	switch u {
	case ActivationDurationDay, ActivationDurationWeek, ActivationDurationMonth:
		return true
	default:
		return false
	}
}

// Derived activation states of a page, as shown to the reseller.
const (
	ActivationStateNone    = "none"
	ActivationStatePreview = "preview"
	ActivationStatePublish = "publish"
	ActivationStateExpired = "expired"
)

// LandingPageActivation is one "sell this page to a client" invoice. Only the
// sha256 of the preview token is stored; the raw token leaves the server once.
type LandingPageActivation struct {
	ID               string
	OrganizationID   string
	PageID           string
	InvoiceID        *string
	BuyerName        string
	BuyerContact     string
	BuyerEmail       *string
	Price            string // numeric(20,2) kept as decimal text, never float
	DurationValue    int
	DurationUnit     ActivationDurationUnit
	PreviewTokenHash string
	InvoiceDueAt     time.Time
	Status           ActivationStatus
	PaidAt           *time.Time
	CreatedBy        *string
	CreatedAt        time.Time
}

// PreviewableAt reports whether the activation's preview link may be served:
// still open and its invoice not yet due. Due exactly at now is closed.
func (a LandingPageActivation) PreviewableAt(now time.Time) bool {
	return a.Status == ActivationStatusOpen && a.InvoiceDueAt.After(now)
}

// PageActivationExpired reports whether the page's paid publish period is over.
// Pages that were never activated (active_until NULL) never expire.
func PageActivationExpired(p LandingPage, now time.Time) bool {
	return p.ActiveUntil != nil && !p.ActiveUntil.After(now)
}

// ActivationState derives the reseller-facing state of a page:
//
//   - publish: status published and active_until in the future;
//   - preview: otherwise, an open activation whose invoice is not yet due
//     (covers both a first sale and a renewal of an expired page);
//   - expired: otherwise, active_until set and not in the future;
//   - none: everything else, including regular pages (active_until NULL).
func ActivationState(p LandingPage, openActivation *LandingPageActivation, now time.Time) string {
	if p.Status == PageStatusPublished && p.ActiveUntil != nil && p.ActiveUntil.After(now) {
		return ActivationStatePublish
	}
	if openActivation != nil && openActivation.PreviewableAt(now) {
		return ActivationStatePreview
	}
	if PageActivationExpired(p, now) {
		return ActivationStateExpired
	}
	return ActivationStateNone
}

// activationPreviewTokenBytes is the entropy of a preview token (256 bits).
const activationPreviewTokenBytes = 32

// NewActivationPreviewToken returns a random url-safe preview token and the
// sha256 hex digest that is the only form persisted.
func NewActivationPreviewToken() (raw string, hash string, err error) {
	buf := make([]byte, activationPreviewTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashActivationPreviewToken(raw), nil
}

// HashActivationPreviewToken is the lookup key of a raw preview token.
func HashActivationPreviewToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ActivationPeriodEnd is paidAt plus the purchased duration. Months are
// calendar months (time.AddDate), weeks are 7 days.
func ActivationPeriodEnd(paidAt time.Time, value int, unit ActivationDurationUnit) time.Time {
	paidAt = paidAt.UTC()
	switch unit {
	case ActivationDurationMonth:
		return paidAt.AddDate(0, value, 0)
	case ActivationDurationWeek:
		return paidAt.AddDate(0, 0, 7*value)
	default:
		return paidAt.AddDate(0, 0, value)
	}
}

// PageSettlement is the page-side effect of a paid activation.
type PageSettlement struct {
	// Publish is false for a deleted or archived page: the money is still
	// recorded on the activation, but the page is left untouched.
	Publish     bool
	PublishedAt time.Time
	ActiveUntil time.Time
}

// SettleActivationPayment decides how a payment changes the page. active_until
// = GREATEST(current active_until when still in the future, paidAt + duration),
// so a second payment can extend but never shorten the paid period; an expired
// page restarts from the payment time. published_at is only set when the page
// was not already published.
func SettleActivationPayment(p LandingPage, a LandingPageActivation, paidAt time.Time) PageSettlement {
	if p.DeletedAt != nil || p.Status == PageStatusArchived {
		return PageSettlement{}
	}
	paidAt = paidAt.UTC()
	until := ActivationPeriodEnd(paidAt, a.DurationValue, a.DurationUnit)
	if p.ActiveUntil != nil && p.ActiveUntil.After(paidAt) && p.ActiveUntil.After(until) {
		until = p.ActiveUntil.UTC()
	}
	publishedAt := paidAt
	if p.Status == PageStatusPublished && p.PublishedAt != nil {
		publishedAt = p.PublishedAt.UTC()
	}
	return PageSettlement{Publish: true, PublishedAt: publishedAt, ActiveUntil: until}
}
