package domain

import "time"

type LeadStatus string

const (
	LeadStatusNew         LeadStatus = "new"
	LeadStatusAttempting  LeadStatus = "attempting"
	LeadStatusContacted   LeadStatus = "contacted"
	LeadStatusQualified   LeadStatus = "qualified"
	LeadStatusUnqualified LeadStatus = "unqualified"
	LeadStatusConverted   LeadStatus = "converted"
)

func (s LeadStatus) IsValid() bool {
	switch s {
	case LeadStatusNew, LeadStatusAttempting, LeadStatusContacted, LeadStatusQualified, LeadStatusUnqualified, LeadStatusConverted:
		return true
	default:
		return false
	}
}

// Lead represents an unqualified prospect before it is converted into a
// Contact/Company/Deal.
type Lead struct {
	ID             string
	OrganizationID string
	ContactName    string
	CompanyName    string
	Email          string
	Phone          string
	Source         string
	Status         LeadStatus
	Score          int
	OwnerUserID    string
	// OwnerName diturunkan (subquery ke users), tidak disimpan di crm_leads.
	OwnerName string
	Notes     string
	JobTitle  string
	// AnnualRevenue disimpan numeric(18,2); dibawa sebagai string supaya
	// presisi desimal tidak hilang (pola sama seperti Deal.Value).
	AnnualRevenue      *string
	Address            map[string]any
	ConvertedContactID *string
	ConvertedCompanyID *string
	ConvertedDealID    *string
	ConvertedAt        *time.Time
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time

	RequirementSummary string
	BudgetEstimate     *string
	TargetDate         *time.Time
	DecisionMaker      string
	DisqualifyReason   string
	DisqualifyNote     string
	// Playbook diisi service (FindPlaybookSummaries), tidak dari leadColumns.
	Playbook *LeadPlaybookSummary
}

// LeadConversionResult reports the entities created/linked by converting a
// lead into working CRM records.
type LeadConversionResult struct {
	Lead    Lead
	Contact Contact
	Company *Company
	Deal    *Deal
}
