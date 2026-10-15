package receivable

import "errors"

var (
	ErrAccountNotFound  = errors.New("receivable account not found")
	ErrInvoiceNotFound  = errors.New("receivable invoice not found")
	ErrContractNotFound = errors.New("receivable contract not found")
)
