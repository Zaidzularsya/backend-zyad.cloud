package domain

import "time"

type LeadStatus string

const (
	LeadStatusNew         LeadStatus = "new"
	LeadStatusContacted   LeadStatus = "contacted"
	LeadStatusQualified   LeadStatus = "qualified"
	LeadStatusUnqualified LeadStatus = "unqualified"
	LeadStatusConverted   LeadStatus = "converted"
)

func (s LeadStatus) IsValid() bool {
	switch s {
	case LeadStatusNew, LeadStatusContacted, LeadStatusQualified, LeadStatusUnqualified, LeadStatusConverted:
		return true
	default:
		return false
	}
}

// Lead represents an unqualified prospect before it is converted into a
// Contact/Company/Deal.
type Lead struct {
	ID                 string
	OrganizationID     string
	ContactName        string
	CompanyName        string
	Email              string
	Phone              string
	Source             string
	Status             LeadStatus
	Score              int
	OwnerUserID        string
	Notes              string
	ConvertedContactID *string
	ConvertedCompanyID *string
	ConvertedDealID    *string
	ConvertedAt        *time.Time
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

// LeadConversionResult reports the entities created/linked by converting a
// lead into working CRM records.
type LeadConversionResult struct {
	Lead    Lead
	Contact Contact
	Company *Company
}
