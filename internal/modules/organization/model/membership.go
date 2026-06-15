package model

import "time"

type MembershipStatus string

const (
	MembershipStatusInvited   MembershipStatus = "invited"
	MembershipStatusActive    MembershipStatus = "active"
	MembershipStatusSuspended MembershipStatus = "suspended"
	MembershipStatusRemoved   MembershipStatus = "removed"
)

func (s MembershipStatus) IsValid() bool {
	switch s {
	case MembershipStatusInvited,
		MembershipStatusActive,
		MembershipStatusSuspended,
		MembershipStatusRemoved:
		return true
	default:
		return false
	}
}

type Membership struct {
	ID                  string
	OrganizationID      string
	UserID              string
	Status              MembershipStatus
	IsOwner             bool
	Version             int64
	InvitedBy           string
	InvitedEmail        string
	InvitationTokenHash string
	InvitationExpiresAt *time.Time
	InvitedAt           *time.Time
	AcceptedAt          *time.Time
	SuspendedAt         *time.Time
	RemovedAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (m Membership) IsActive() bool {
	return m.Status == MembershipStatusActive && m.RemovedAt == nil
}
