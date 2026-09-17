package model

import "time"

type FiscalStatus string

const (
	FiscalStatusOpen   FiscalStatus = "open"
	FiscalStatusClosed FiscalStatus = "closed"
)

type FiscalYear struct {
	ID        string
	Year      int
	StartDate time.Time
	EndDate   time.Time
	Status    FiscalStatus
	ClosedAt  *time.Time
	ClosedBy  *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FiscalPeriod struct {
	ID           string
	FiscalYearID string
	PeriodNumber int
	StartDate    time.Time
	EndDate      time.Time
	Status       FiscalStatus
	ClosedAt     *time.Time
	ClosedBy     *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s FiscalStatus) IsValid() bool {
	return s == FiscalStatusOpen || s == FiscalStatusClosed
}
