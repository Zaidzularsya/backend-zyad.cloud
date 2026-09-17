package model

import "time"

type TaxCategory string

const (
	TaxCategoryOutputVAT       TaxCategory = "output_vat"
	TaxCategoryInputVAT        TaxCategory = "input_vat"
	TaxCategoryWithholding     TaxCategory = "withholding"
	TaxCategoryCorporateIncome TaxCategory = "corporate_income"
)

type TaxDirection string

const (
	TaxDirectionIncrease TaxDirection = "increase"
	TaxDirectionDecrease TaxDirection = "decrease"
)

func (d TaxDirection) IsValid() bool {
	return d == TaxDirectionIncrease || d == TaxDirectionDecrease
}

type TaxType struct {
	ID        string
	Code      string
	Name      string
	Category  TaxCategory
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TaxRate struct {
	ID            string
	TaxTypeID     string
	TaxTypeCode   string
	RatePercent   string
	EffectiveDate time.Time
	EndDate       *time.Time
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type TaxTransaction struct {
	ID                string
	TaxTypeID         string
	TaxTypeCode       string
	TaxTypeName       string
	TransactionDate   time.Time
	ReferenceNumber   string
	Amount            string
	Direction         TaxDirection
	TaxAccountID      string
	TaxAccountCode    string
	TaxAccountName    string
	ContraAccountID   string
	ContraAccountCode string
	ContraAccountName string
	Description       string
	JournalEntryID    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TaxSummaryRow struct {
	TaxTypeID   string
	TaxTypeCode string
	TaxTypeName string
	Increase    string
	Decrease    string
	Net         string
}
