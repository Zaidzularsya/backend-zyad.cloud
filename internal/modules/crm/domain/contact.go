package domain

import "time"

// ContactLifecycleStage tracks where a contact sits between prospect and
// active customer. It is intentionally kept on a single crm_contacts table
// (rather than a separate crm_customers table) — see docs/reference-crm.md
// "Naming Conflict" section for the rationale.
type ContactLifecycleStage string

const (
	ContactLifecycleLead     ContactLifecycleStage = "lead"
	ContactLifecycleContact  ContactLifecycleStage = "contact"
	ContactLifecycleCustomer ContactLifecycleStage = "customer"
	ContactLifecycleChurned  ContactLifecycleStage = "churned"
)

func (s ContactLifecycleStage) IsValid() bool {
	switch s {
	case ContactLifecycleLead, ContactLifecycleContact, ContactLifecycleCustomer, ContactLifecycleChurned:
		return true
	default:
		return false
	}
}

// Contact represents a person tracked by a tenant — a prospect ("contact")
// or an active customer ("customer"), distinguished by IsCustomer/LifecycleStage
// rather than by a separate table.
type Contact struct {
	ID             string
	OrganizationID string
	CompanyID      *string
	FirstName      string
	LastName       string
	Email          string
	Phone          string
	JobTitle       string
	Address        map[string]any
	Tags           []string
	Source         string
	OwnerUserID    string
	IsCustomer     bool
	LifecycleStage ContactLifecycleStage
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
