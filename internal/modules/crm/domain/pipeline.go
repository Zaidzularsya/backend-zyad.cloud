package domain

import "time"

type PipelineStage struct {
	ID             string
	OrganizationID string
	PipelineID     string
	Name           string
	Position       int
	// Probability is kept as a decimal string (not float64), scanned via
	// `::text` cast in the repository — mirrors internal/modules/billing
	// money fields, avoiding float precision issues on NUMERIC columns.
	Probability string
	IsWon       bool
	IsLost      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type Pipeline struct {
	ID             string
	OrganizationID string
	Name           string
	IsDefault      bool
	ArchivedAt     *time.Time
	Stages         []PipelineStage
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
