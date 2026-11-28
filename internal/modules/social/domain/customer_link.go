package domain

import (
	"errors"
	"strings"
	"time"
)

// customer_links.status values.
const (
	LinkStatusPendingUser = "pending_user"
	LinkStatusActive      = "active"
	LinkStatusRevoked     = "revoked"
	LinkStatusDeclined    = "declined"
)

// customer_links.verified_via values.
const (
	VerifiedViaEmail  = "email"
	VerifiedViaInvite = "invite"
	VerifiedViaStaff  = "staff"
)

var (
	// ErrClaimNotAvailable: the contact cannot be claimed by this user (unknown,
	// deleted, not a billing/PPPoE customer, email mismatch, tenant opted out,
	// already declined).
	ErrClaimNotAvailable = errors.New("customer claim not available")
	// ErrCustomerLinkExists: the contact, or the user within that organization,
	// already has a live (pending_user/active) link.
	ErrCustomerLinkExists = errors.New("customer link already exists")
	// ErrCustomerLinkNotFound: no live link with that id visible to the actor.
	ErrCustomerLinkNotFound = errors.New("customer link not found")
	// ErrEmailNotVerified: claims need proof of email ownership.
	ErrEmailNotVerified = errors.New("email not verified")
)

// CustomerLink is one customer_links row: a user linked to a CRM contact of an organization.
type CustomerLink struct {
	ID             string
	UserID         string
	OrganizationID string
	ContactID      string
	Status         string
	VerifiedVia    string
	CreatedAt      time.Time
	ActivatedAt    *time.Time
	RevokedAt      *time.Time
}

// IsLive reports whether the link occupies the unique "live" slot.
func (l CustomerLink) IsLive() bool {
	return l.Status == LinkStatusPendingUser || l.Status == LinkStatusActive
}

// NormalizeEmail is the canonical form used for every email comparison (lower(btrim(email))).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
