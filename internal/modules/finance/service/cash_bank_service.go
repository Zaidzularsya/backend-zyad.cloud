package service

import (
	"context"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type CashBankStore interface {
	Create(ctx context.Context, params repository.CreateCashBankAccountParams) (model.CashBankAccount, error)
	Update(ctx context.Context, params repository.UpdateCashBankAccountParams) (model.CashBankAccount, error)
	FindByID(ctx context.Context, id string) (model.CashBankAccount, error)
	List(ctx context.Context, includeInactive bool) ([]model.CashBankAccount, error)
	CreateTransaction(ctx context.Context, params repository.CreateCashTransactionParams) (model.CashTransaction, error)
	FindTransactionByID(ctx context.Context, id string) (model.CashTransaction, error)
	ListTransactions(ctx context.Context, filter repository.CashTransactionListFilter) ([]model.CashTransaction, int64, error)
	CreateReconciliation(ctx context.Context, params repository.CreateBankReconciliationParams) (model.BankReconciliation, error)
	ListReconciliations(ctx context.Context, cashBankAccountID string) ([]model.BankReconciliation, error)
	CompleteReconciliation(ctx context.Context, id string, actorID *string) (model.BankReconciliation, error)
}

type EntryNumberGenerator interface {
	NextEntryNumber(ctx context.Context, year int) (string, error)
}

type BalanceLookup interface {
	AccountBalanceAsOf(ctx context.Context, accountID string, asOf time.Time) (string, error)
}

type CashBankService struct {
	store       CashBankStore
	entryNumber EntryNumberGenerator
	balances    BalanceLookup
}

func NewCashBankService(store CashBankStore, entryNumber EntryNumberGenerator, balances BalanceLookup) *CashBankService {
	return &CashBankService{store: store, entryNumber: entryNumber, balances: balances}
}

func (s *CashBankService) CreateAccount(ctx context.Context, request dto.CreateCashBankAccountRequest) (dto.CashBankAccountResponse, error) {
	accountType := model.CashBankAccountType(strings.TrimSpace(request.Type))
	if !accountType.IsValid() {
		return dto.CashBankAccountResponse{}, finance.ValidationError("type must be 'cash' or 'bank'")
	}
	if strings.TrimSpace(request.AccountID) == "" {
		return dto.CashBankAccountResponse{}, finance.ValidationError("account_id is required")
	}
	account, err := s.store.Create(ctx, repository.CreateCashBankAccountParams{
		AccountID:         request.AccountID,
		Type:              accountType,
		BankName:          request.BankName,
		AccountNumber:     request.AccountNumber,
		AccountHolderName: request.AccountHolderName,
	})
	if err != nil {
		return dto.CashBankAccountResponse{}, mapAccountError(err)
	}
	return cashBankAccountResponse(account), nil
}

func (s *CashBankService) UpdateAccount(ctx context.Context, id string, request dto.UpdateCashBankAccountRequest) (dto.CashBankAccountResponse, error) {
	account, err := s.store.Update(ctx, repository.UpdateCashBankAccountParams{
		ID:                strings.TrimSpace(id),
		BankName:          request.BankName,
		AccountNumber:     request.AccountNumber,
		AccountHolderName: request.AccountHolderName,
		IsActive:          request.IsActive,
	})
	if err != nil {
		return dto.CashBankAccountResponse{}, mapAccountError(err)
	}
	return cashBankAccountResponse(account), nil
}

func (s *CashBankService) ListAccounts(ctx context.Context, includeInactive bool) ([]dto.CashBankAccountResponse, error) {
	accounts, err := s.store.List(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.CashBankAccountResponse, 0, len(accounts))
	for _, a := range accounts {
		responses = append(responses, cashBankAccountResponse(a))
	}
	return responses, nil
}

func (s *CashBankService) CreateTransaction(ctx context.Context, request dto.CreateCashTransactionRequest, actorID string) (dto.CashTransactionResponse, error) {
	transactionType := model.CashTransactionType(strings.TrimSpace(request.TransactionType))
	if !transactionType.IsValid() {
		return dto.CashTransactionResponse{}, finance.ValidationError("transaction_type must be cash_in, cash_out, or transfer")
	}
	transactionDate, err := parseRequiredDate(request.TransactionDate)
	if err != nil {
		return dto.CashTransactionResponse{}, err
	}
	amount, err := moneyRat(request.Amount)
	if err != nil || amount.Sign() <= 0 {
		return dto.CashTransactionResponse{}, finance.ValidationError("amount must be greater than zero")
	}

	counter := trimmedOrNil(request.CounterCashBankAccountID)
	contra := trimmedOrNil(request.ContraAccountID)
	if transactionType == model.CashTransactionTypeTransfer {
		if counter == nil {
			return dto.CashTransactionResponse{}, finance.ValidationError("counter_cash_bank_account_id is required for transfer")
		}
		contra = nil
	} else {
		if contra == nil {
			return dto.CashTransactionResponse{}, finance.ValidationError("contra_account_id is required for cash_in/cash_out")
		}
		counter = nil
	}

	entryNumber, err := s.entryNumber.NextEntryNumber(ctx, transactionDate.Year())
	if err != nil {
		return dto.CashTransactionResponse{}, err
	}

	transaction, err := s.store.CreateTransaction(ctx, repository.CreateCashTransactionParams{
		CashBankAccountID:        strings.TrimSpace(request.CashBankAccountID),
		TransactionDate:          transactionDate,
		TransactionType:          transactionType,
		Amount:                   formatMoney(amount),
		CounterCashBankAccountID: counter,
		ContraAccountID:          contra,
		Reference:                request.Reference,
		Description:              request.Description,
		EntryNumber:              entryNumber,
		CreatedBy:                trimmedOrNil(actorID),
	})
	if err != nil {
		return dto.CashTransactionResponse{}, mapJournalError(err)
	}
	return cashTransactionResponse(transaction), nil
}

func (s *CashBankService) ListTransactions(ctx context.Context, query dto.CashTransactionListQuery) (dto.CashTransactionListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	filter := repository.CashTransactionListFilter{
		CashBankAccountID: query.CashBankAccountID,
		Limit:             perPage,
		Offset:            (page - 1) * perPage,
	}
	if strings.TrimSpace(query.StartDate) != "" {
		start, err := parseRequiredDate(query.StartDate)
		if err != nil {
			return dto.CashTransactionListResponse{}, err
		}
		filter.StartDate = &start
	}
	if strings.TrimSpace(query.EndDate) != "" {
		end, err := parseRequiredDate(query.EndDate)
		if err != nil {
			return dto.CashTransactionListResponse{}, err
		}
		filter.EndDate = &end
	}
	transactions, total, err := s.store.ListTransactions(ctx, filter)
	if err != nil {
		return dto.CashTransactionListResponse{}, err
	}
	responses := make([]dto.CashTransactionResponse, 0, len(transactions))
	for _, t := range transactions {
		responses = append(responses, cashTransactionResponse(t))
	}
	return dto.CashTransactionListResponse{
		Items: responses,
		Meta:  dto.PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPagesOf(perPage, total)},
	}, nil
}

func (s *CashBankService) CreateReconciliation(ctx context.Context, request dto.CreateBankReconciliationRequest) (dto.BankReconciliationResponse, error) {
	statementDate, err := parseRequiredDate(request.StatementDate)
	if err != nil {
		return dto.BankReconciliationResponse{}, err
	}
	statementBalance, err := moneyRat(request.StatementEndingBalance)
	if err != nil {
		return dto.BankReconciliationResponse{}, finance.ValidationError("statement_ending_balance is invalid")
	}
	bookBalance, err := s.bookBalanceForAccount(ctx, request.CashBankAccountID, statementDate)
	if err != nil {
		return dto.BankReconciliationResponse{}, err
	}
	rec, err := s.store.CreateReconciliation(ctx, repository.CreateBankReconciliationParams{
		CashBankAccountID:      strings.TrimSpace(request.CashBankAccountID),
		StatementDate:          statementDate,
		StatementEndingBalance: formatMoney(statementBalance),
		BookEndingBalance:      bookBalance,
		Notes:                  request.Notes,
	})
	if err != nil {
		return dto.BankReconciliationResponse{}, err
	}
	return bankReconciliationResponse(rec), nil
}

func (s *CashBankService) ListReconciliations(ctx context.Context, cashBankAccountID string) ([]dto.BankReconciliationResponse, error) {
	items, err := s.store.ListReconciliations(ctx, strings.TrimSpace(cashBankAccountID))
	if err != nil {
		return nil, err
	}
	responses := make([]dto.BankReconciliationResponse, 0, len(items))
	for _, r := range items {
		responses = append(responses, bankReconciliationResponse(r))
	}
	return responses, nil
}

func (s *CashBankService) CompleteReconciliation(ctx context.Context, id string, actorID string) (dto.BankReconciliationResponse, error) {
	rec, err := s.store.CompleteReconciliation(ctx, strings.TrimSpace(id), trimmedOrNil(actorID))
	if err != nil {
		return dto.BankReconciliationResponse{}, mapReconciliationError(err)
	}
	return bankReconciliationResponse(rec), nil
}

// bookBalanceForAccount resolves the cash/bank account's linked GL account
// balance as of the statement date, used as the book-side figure a
// reconciliation compares against the bank statement.
func (s *CashBankService) bookBalanceForAccount(ctx context.Context, cashBankAccountID string, asOf time.Time) (string, error) {
	account, err := s.store.FindByID(ctx, strings.TrimSpace(cashBankAccountID))
	if err != nil {
		return "", mapAccountError(err)
	}
	balance, err := s.balances.AccountBalanceAsOf(ctx, account.AccountID, asOf)
	if err != nil {
		return "", err
	}
	return balance, nil
}
