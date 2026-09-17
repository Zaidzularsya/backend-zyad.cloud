package service

import (
	"context"
	"math/big"
	"time"

	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type LedgerStore interface {
	TrialBalance(ctx context.Context, asOf time.Time) ([]repository.TrialBalanceRow, error)
	GeneralLedger(ctx context.Context, start, end time.Time, accountID string) ([]repository.LedgerLineRow, error)
	AccountOpeningBalanceBefore(ctx context.Context, accountID string, before time.Time) (string, error)
	ProfitLoss(ctx context.Context, start, end time.Time) ([]repository.ReportAccountRow, error)
	BalanceSheet(ctx context.Context, asOf time.Time) ([]repository.ReportAccountRow, error)
}

type AccountLookup interface {
	FindByID(ctx context.Context, id string) (model.Account, error)
}

type LedgerService struct {
	ledger   LedgerStore
	accounts AccountLookup
}

func NewLedgerService(ledger LedgerStore, accounts AccountLookup) *LedgerService {
	return &LedgerService{ledger: ledger, accounts: accounts}
}

func (s *LedgerService) TrialBalance(ctx context.Context, query dto.TrialBalanceQuery) (dto.TrialBalanceResponse, error) {
	asOf, err := parseRequiredDate(query.AsOfDate)
	if err != nil {
		return dto.TrialBalanceResponse{}, err
	}
	rows, err := s.ledger.TrialBalance(ctx, asOf)
	if err != nil {
		return dto.TrialBalanceResponse{}, err
	}

	lines := make([]dto.TrialBalanceLineResponse, 0, len(rows))
	totalDebit, totalCredit := zeroRat(), zeroRat()
	for _, row := range rows {
		debit, _ := moneyRat(row.Debit)
		credit, _ := moneyRat(row.Credit)
		totalDebit.Add(totalDebit, debit)
		totalCredit.Add(totalCredit, credit)
		lines = append(lines, dto.TrialBalanceLineResponse{
			AccountID: row.AccountID, AccountCode: row.AccountCode, AccountName: row.AccountName,
			Debit: row.Debit, Credit: row.Credit,
		})
	}
	return dto.TrialBalanceResponse{
		AsOfDate:    formatDate(asOf),
		Lines:       lines,
		TotalDebit:  formatMoney(totalDebit),
		TotalCredit: formatMoney(totalCredit),
		IsBalanced:  totalDebit.Cmp(totalCredit) == 0,
	}, nil
}

func (s *LedgerService) GeneralLedger(ctx context.Context, query dto.GeneralLedgerQuery) (dto.GeneralLedgerResponse, error) {
	start, err := parseRequiredDate(query.StartDate)
	if err != nil {
		return dto.GeneralLedgerResponse{}, err
	}
	end, err := parseRequiredDate(query.EndDate)
	if err != nil {
		return dto.GeneralLedgerResponse{}, err
	}
	rows, err := s.ledger.GeneralLedger(ctx, start, end, query.AccountID)
	if err != nil {
		return dto.GeneralLedgerResponse{}, err
	}
	lines := make([]dto.GeneralLedgerLineResponse, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, dto.GeneralLedgerLineResponse{
			EntryDate: formatDate(row.EntryDate), EntryNumber: row.EntryNumber,
			AccountID: row.AccountID, AccountCode: row.AccountCode, AccountName: row.AccountName,
			Description: row.Description, Debit: row.Debit, Credit: row.Credit,
		})
	}
	return dto.GeneralLedgerResponse{StartDate: formatDate(start), EndDate: formatDate(end), Lines: lines}, nil
}

func (s *LedgerService) AccountLedger(ctx context.Context, accountID string, query dto.AccountLedgerQuery) (dto.AccountLedgerResponse, error) {
	start, err := parseRequiredDate(query.StartDate)
	if err != nil {
		return dto.AccountLedgerResponse{}, err
	}
	end, err := parseRequiredDate(query.EndDate)
	if err != nil {
		return dto.AccountLedgerResponse{}, err
	}
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return dto.AccountLedgerResponse{}, mapAccountError(err)
	}
	openingRaw, err := s.ledger.AccountOpeningBalanceBefore(ctx, accountID, start)
	if err != nil {
		return dto.AccountLedgerResponse{}, err
	}
	opening, err := moneyRat(openingRaw)
	if err != nil {
		return dto.AccountLedgerResponse{}, err
	}

	rows, err := s.ledger.GeneralLedger(ctx, start, end, accountID)
	if err != nil {
		return dto.AccountLedgerResponse{}, err
	}

	running := new(big.Rat).Set(opening)
	lines := make([]dto.AccountLedgerLineResponse, 0, len(rows))
	for _, row := range rows {
		debit, _ := moneyRat(row.Debit)
		credit, _ := moneyRat(row.Credit)
		if account.NormalBalance == "debit" {
			running.Add(running, debit)
			running.Sub(running, credit)
		} else {
			running.Add(running, credit)
			running.Sub(running, debit)
		}
		lines = append(lines, dto.AccountLedgerLineResponse{
			EntryDate: formatDate(row.EntryDate), EntryNumber: row.EntryNumber, Description: row.Description,
			Debit: row.Debit, Credit: row.Credit, RunningBalance: formatMoney(running),
		})
	}

	return dto.AccountLedgerResponse{
		AccountID: account.ID, AccountCode: account.AccountCode, AccountName: account.AccountName,
		StartDate: formatDate(start), EndDate: formatDate(end),
		OpeningBalance: formatMoney(opening), Lines: lines, ClosingBalance: formatMoney(running),
	}, nil
}
