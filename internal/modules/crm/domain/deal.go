package domain

import "time"

type DealStatus string

const (
	DealStatusOpen DealStatus = "open"
	DealStatusWon  DealStatus = "won"
	DealStatusLost DealStatus = "lost"
)

func (s DealStatus) IsValid() bool {
	switch s {
	case DealStatusOpen, DealStatusWon, DealStatusLost:
		return true
	default:
		return false
	}
}

type Deal struct {
	ID             string
	OrganizationID string
	PipelineID     string
	StageID        string
	CompanyID      *string
	ContactID      *string
	Title          string
	// Value/DiscountPercent are decimal strings (see PipelineStage.Probability
	// doc comment for why — matches internal/modules/billing convention).
	Value              string
	Currency           string
	ExpectedCloseDate  *time.Time
	Status             DealStatus
	LostReason         string
	OwnerUserID        string
	DiscountPercent    *string
	DiscountApprovedBy string
	DiscountApprovedAt *time.Time
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}
