package domain

import "time"

type SubmissionStatus string

const (
	SubmissionStatusNew       SubmissionStatus = "new"
	SubmissionStatusContacted SubmissionStatus = "contacted"
	SubmissionStatusQualified SubmissionStatus = "qualified"
	SubmissionStatusConverted SubmissionStatus = "converted"
	SubmissionStatusRejected  SubmissionStatus = "rejected"
	SubmissionStatusSpam      SubmissionStatus = "spam"
	SubmissionStatusArchived  SubmissionStatus = "archived"
)

func (s SubmissionStatus) IsValid() bool {
	switch s {
	case SubmissionStatusNew,
		SubmissionStatusContacted,
		SubmissionStatusQualified,
		SubmissionStatusConverted,
		SubmissionStatusRejected,
		SubmissionStatusSpam,
		SubmissionStatusArchived:
		return true
	default:
		return false
	}
}

type LandingSubmission struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	FormID         string
	Reference      string
	Status         SubmissionStatus
	SubmittedData  map[string]any
	SourceURL      string
	Referrer       string
	UTMSource      string
	UTMMedium      string
	UTMCampaign    string
	UTMTerm        string
	UTMContent     string
	IPAddressHash  string
	UserAgent      string
	IdempotencyKey string
	SubmittedAt    time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
