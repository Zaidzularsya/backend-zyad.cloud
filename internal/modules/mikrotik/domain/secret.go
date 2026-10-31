package domain

import (
	"fmt"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
)

// Isolation states of a PPP secret.
const (
	IsolationNormal   = "normal"
	IsolationIsolated = "isolated"
)

// PPPSecret is the cached copy of a router's /ppp/secret entry, optionally
// paired to a customer account.
type PPPSecret struct {
	ID                   string
	OrganizationID       string
	RouterID             string
	Name                 string
	Profile              string
	OriginalProfile      string
	NeedsOriginalProfile bool
	Disabled             bool
	Comment              string
	Present              bool
	LastSeenAt           *time.Time
	IsolationState       string
	AccountID            *string
	PairedBy             *string
	PairedAt             *time.Time
	ImportedAt           time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CustomerPolicy is the per-customer isolir exemption.
type CustomerPolicy struct {
	OrganizationID string
	AccountID      string
	Exempt         bool
	Reason         string
	SetBy          *string
	SetAt          time.Time
}

// ProductTrigger marks a product as a PPPoE service whose overdue invoices
// trigger isolir from EffectiveFrom.
type ProductTrigger struct {
	OrganizationID string
	ProductID      string
	Enabled        bool
	EffectiveFrom  time.Time
	SetBy          *string
	SetAt          time.Time
}

// Pairable reports whether the secret may be paired to a customer. A secret
// sitting on the isolir profile with no recorded original profile is refused,
// because it could never be restored.
func (s PPPSecret) Pairable(isolirProfile string) error {
	switch {
	case !s.Present:
		return fmt.Errorf("%w: secret is no longer on the router", mikrotik.ErrSecretNotPairable)
	case s.NeedsOriginalProfile:
		return fmt.Errorf("%w: original profile must be set first", mikrotik.ErrSecretNotPairable)
	case isolirProfile != "" && s.Profile == isolirProfile && s.OriginalProfile == "":
		return fmt.Errorf("%w: secret is on the isolir profile without an original profile", mikrotik.ErrSecretNotPairable)
	}
	return nil
}
