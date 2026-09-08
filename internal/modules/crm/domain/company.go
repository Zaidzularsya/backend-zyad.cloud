package domain

import "time"

// Company represents a tenant's prospect or customer organization.
type Company struct {
	ID             string
	OrganizationID string
	Name           string
	Industry       string
	Website        string
	Phone          string
	Email          string
	Address        map[string]any
	SizeRange      string
	Notes          string
	Tags           []string
	OwnerUserID    string
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
