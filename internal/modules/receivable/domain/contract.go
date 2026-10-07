package domain

import (
	"encoding/json"
	"errors"
	"time"

	"zyad.cloud/internal/shared/pricing"
)

type ContractStatus string

const (
	ContractActive    ContractStatus = "active"
	ContractEnded     ContractStatus = "ended"
	ContractCancelled ContractStatus = "cancelled"
)

var ErrContractNotActive = errors.New("contract is not active")

type ContractItem struct {
	ID, Description, Quantity, Unit, UnitPrice, DiscountPercent, TaxPercent, ProductID, SKU, SourceLineID string
	Frequency                                                                                             pricing.Frequency
	PaymentTiming                                                                                         pricing.PaymentTiming
	PeriodIndex                                                                                           int
	NextPeriodStart                                                                                       time.Time // awal periode yang belum ditagih
	NextPeriodEnd                                                                                         time.Time // PeriodRange(start, f, PeriodIndex).to — dipakai billing run S5
	Position                                                                                              int
	// Features: snapshot fitur produk (JSON array), tidak ditafsirkan receivable; tidak pernah nil dari repository.
	Features json.RawMessage
}

type Contract struct {
	ID, AccountID, ContractNumber                   string
	Account                                         Account // join read-only
	Status                                          ContractStatus
	SourceType                                      SourceType
	SourceID, Currency, Notes, PICUserID, EndReason string
	StartDate                                       time.Time
	EndDate, EndedAt                                *time.Time
	Channels                                        []string
	Items                                           []ContractItem
	CreatedBy                                       string
	CreatedAt, UpdatedAt                            time.Time
}
