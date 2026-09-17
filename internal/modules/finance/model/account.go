package model

import "time"

type AccountType struct {
	ID                 string
	Code               string
	Name               string
	NormalBalance      string
	FinancialStatement string
	SortOrder          int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AccountCategory struct {
	ID            string
	AccountTypeID string
	Code          string
	Name          string
	ReportSection string
	SortOrder     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Account struct {
	ID                  string
	AccountCode         string
	AccountName         string
	AccountCategoryID   *string
	AccountCategoryCode string
	AccountCategoryName string
	AccountTypeCode     string
	ParentAccountID     *string
	IsHeader            bool
	NormalBalance       string
	IsActive            bool
	OpeningBalance      string
	OpeningBalanceDate  *time.Time
	Description         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}
