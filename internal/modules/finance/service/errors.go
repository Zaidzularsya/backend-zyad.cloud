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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
