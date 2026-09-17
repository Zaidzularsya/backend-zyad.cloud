package finance

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const (
	ErrCodeAccountNotFound         = "FINANCE_ACCOUNT_NOT_FOUND"
	ErrCodeAccountCodeTaken        = "FINANCE_ACCOUNT_CODE_TAKEN"
	ErrCodeFiscalYearExists        = "FINANCE_FISCAL_YEAR_EXISTS"
	ErrCodeFiscalYearNotFound      = "FINANCE_FISCAL_YEAR_NOT_FOUND"
	ErrCodeFiscalPeriodNotFound    = "FINANCE_FISCAL_PERIOD_NOT_FOUND"
	ErrCodeNoFiscalPeriod          = "FINANCE_NO_FISCAL_PERIOD"
	ErrCodePeriodClosed            = "FINANCE_PERIOD_CLOSED"
	ErrCodeJournalNotFound         = "FINANCE_JOURNAL_NOT_FOUND"
	ErrCodeJournalUnbalanced       = "FINANCE_JOURNAL_UNBALANCED"
	ErrCodeJournalNotDraft         = "FINANCE_JOURNAL_NOT_DRAFT"
	ErrCodeJournalNotPosted        = "FINANCE_JOURNAL_NOT_POSTED"
	ErrCodeJournalAlreadyReversed  = "FINANCE_JOURNAL_ALREADY_REVERSED"
	ErrCodeValidation              = "FINANCE_VALIDATION_ERROR"
	ErrCodeReconciliationNotFound  = "FINANCE_RECONCILIATION_NOT_FOUND"
	ErrCodePartnerNotFound         = "FINANCE_PARTNER_NOT_FOUND"
	ErrCodeARAPTransactionNotFound = "FINANCE_ARAP_TRANSACTION_NOT_FOUND"
	ErrCodeAssetCategoryNotFound   = "FINANCE_ASSET_CATEGORY_NOT_FOUND"
	ErrCodeFixedAssetNotFound      = "FINANCE_FIXED_ASSET_NOT_FOUND"
)

func AccountNotFoundError() error {
	return coreerrors.New(ErrCodeAccountNotFound, "chart of accounts entry not found", http.StatusNotFound)
}

func AccountCodeTakenError() error {
	return coreerrors.New(ErrCodeAccountCodeTaken, "account code is already in use", http.StatusConflict)
}

func FiscalYearExistsError() error {
	return coreerrors.New(ErrCodeFiscalYearExists, "fiscal year already exists", http.StatusConflict)
}

func FiscalYearNotFoundError() error {
	return coreerrors.New(ErrCodeFiscalYearNotFound, "fiscal year not found", http.StatusNotFound)
}

func FiscalPeriodNotFoundError() error {
	return coreerrors.New(ErrCodeFiscalPeriodNotFound, "fiscal period not found", http.StatusNotFound)
}

func NoFiscalPeriodError() error {
	return coreerrors.New(ErrCodeNoFiscalPeriod, "no fiscal period covers the given date; create the fiscal year first", http.StatusUnprocessableEntity)
}

func PeriodClosedError() error {
	return coreerrors.New(ErrCodePeriodClosed, "fiscal period is closed for posting", http.StatusConflict)
}

func JournalNotFoundError() error {
	return coreerrors.New(ErrCodeJournalNotFound, "journal entry not found", http.StatusNotFound)
}

func JournalUnbalancedError() error {
	return coreerrors.New(ErrCodeJournalUnbalanced, "journal entry debit and credit totals do not balance", http.StatusUnprocessableEntity)
}

func JournalNotDraftError() error {
	return coreerrors.New(ErrCodeJournalNotDraft, "journal entry is not in draft status", http.StatusConflict)
}

func JournalNotPostedError() error {
	return coreerrors.New(ErrCodeJournalNotPosted, "journal entry is not posted", http.StatusConflict)
}

func JournalAlreadyReversedError() error {
	return coreerrors.New(ErrCodeJournalAlreadyReversed, "journal entry is already reversed", http.StatusConflict)
}

func ReconciliationNotFoundError() error {
	return coreerrors.New(ErrCodeReconciliationNotFound, "bank reconciliation not found", http.StatusNotFound)
}

func PartnerNotFoundError() error {
	return coreerrors.New(ErrCodePartnerNotFound, "business partner not found", http.StatusNotFound)
}

func ARAPTransactionNotFoundError() error {
	return coreerrors.New(ErrCodeARAPTransactionNotFound, "AR/AP transaction not found", http.StatusNotFound)
}

func AssetCategoryNotFoundError() error {
	return coreerrors.New(ErrCodeAssetCategoryNotFound, "asset category not found", http.StatusNotFound)
}

func FixedAssetNotFoundError() error {
	return coreerrors.New(ErrCodeFixedAssetNotFound, "fixed asset not found", http.StatusNotFound)
}

func ValidationError(message string) error {
	return coreerrors.New(ErrCodeValidation, message, http.StatusBadRequest)
}
