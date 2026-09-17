package service

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/repository"
)

func mapAccountError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.AccountNotFoundError()
	}
	if isUniqueViolation(err) {
		return finance.AccountCodeTakenError()
	}
	return err
}

func mapFiscalYearError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.FiscalYearNotFoundError()
	}
	if isUniqueViolation(err) {
		return finance.FiscalYearExistsError()
	}
	return err
}

func mapFiscalPeriodError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.FiscalPeriodNotFoundError()
	}
	return err
}

func mapReconciliationError(err error) error {
	if errors.Is(err, repository.ErrBankReconciliationNotFound) {
		return finance.ReconciliationNotFoundError()
	}
	return err
}

func mapPartnerError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.PartnerNotFoundError()
	}
	if isUniqueViolation(err) {
		return finance.ValidationError("partner code is already in use")
	}
	return err
}

func mapARAPError(err error) error {
	if errors.Is(err, repository.ErrARAPTransactionTypeMismatch) {
		return finance.ValidationError("transaction_type does not match the partner's type (customer=receivable, vendor=payable)")
	}
	if errors.Is(err, repository.ErrARAPOverpayment) {
		return finance.ValidationError("payment amount exceeds the transaction's outstanding balance")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.ARAPTransactionNotFoundError()
	}
	return err
}

func mapAssetCategoryError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.AssetCategoryNotFoundError()
	}
	if isUniqueViolation(err) {
		return finance.ValidationError("asset category code is already in use")
	}
	return err
}

func mapFixedAssetError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.FixedAssetNotFoundError()
	}
	if isUniqueViolation(err) {
		return finance.ValidationError("asset code is already in use")
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
