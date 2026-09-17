package model

import "time"

type AssetStatus string

const (
	AssetStatusActive           AssetStatus = "active"
	AssetStatusFullyDepreciated AssetStatus = "fully_depreciated"
)

func (s AssetStatus) IsValid() bool {
	return s == AssetStatusActive || s == AssetStatusFullyDepreciated
}

type DepreciationStatus string

const (
	DepreciationStatusPending DepreciationStatus = "pending"
	DepreciationStatusPosted  DepreciationStatus = "posted"
)

type AssetCategory struct {
	ID                                 string
	Code                               string
	Name                               string
	AssetAccountID                     string
	AssetAccountCode                   string
	AssetAccountName                   string
	AccumulatedDepreciationAccountID   string
	AccumulatedDepreciationAccountCode string
	AccumulatedDepreciationAccountName string
	DepreciationExpenseAccountID       string
	DepreciationExpenseAccountCode     string
	DepreciationExpenseAccountName     string
	DefaultUsefulLifeMonths            *int
	IsActive                           bool
	CreatedAt                          time.Time
	UpdatedAt                          time.Time
}

type FixedAsset struct {
	ID                        string
	AssetCategoryID           string
	AssetCategoryCode         string
	AssetCategoryName         string
	AssetCode                 string
	AssetName                 string
	AcquisitionDate           time.Time
	AcquisitionCost           string
	SalvageValue              string
	UsefulLifeMonths          int
	Description               string
	ContraAccountID           string
	AcquisitionJournalEntryID string
	Status                    AssetStatus
	AccumulatedDepreciation   string
	BookValue                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type DepreciationSchedule struct {
	ID                 string
	FixedAssetID       string
	AssetCode          string
	AssetName          string
	SequenceNumber     int
	PeriodDate         time.Time
	DepreciationAmount string
	Status             DepreciationStatus
	JournalEntryID     *string
	PostedAt           *time.Time
	CreatedAt          time.Time
}
