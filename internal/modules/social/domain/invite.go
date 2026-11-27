package domain

import (
	"errors"
	"time"
)

// InviteTTL is how long a staff-sent claim invite stays usable.
const InviteTTL = 7 * 24 * time.Hour

var (
	// ErrInviteInvalid: the token is unknown, already used, or was superseded by a newer invite.
	ErrInviteInvalid = errors.New("customer claim invite invalid")
	// ErrInviteExpired: the invite was not used within InviteTTL.
	ErrInviteExpired = errors.New("customer claim invite expired")
	// ErrContactEmailMissing: the contact has no email to send the invite to.
	ErrContactEmailMissing = errors.New("contact has no email")
)

// ClaimInvite is one customer_claim_invites row. Only the token hash is stored.
type ClaimInvite struct {
	ID             string
	OrganizationID string
	ContactID      string
	// Email is the normalized address the invite was sent to.
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedBy string
	CreatedAt time.Time
}

// Check reports why the invite cannot be used at now, or nil when it can.
func (i ClaimInvite) Check(now time.Time) error {
	if i.UsedAt != nil {
		return ErrInviteInvalid
	}
	if !now.Before(i.ExpiresAt) {
		return ErrInviteExpired
	}
	return nil
}
