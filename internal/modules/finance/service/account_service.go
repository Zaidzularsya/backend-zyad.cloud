package service

import (
	"context"
	"strings"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type AccountStore interface {
	ListAccountTypes(ctx context.Context) ([]model.AccountType, error)
	ListAccountCategories(ctx context.Context) ([]model.AccountCategory, error)
	Create(ctx context.Context, params repository.CreateAccountParams) (model.Account, error)
	Update(ctx context.Context, params repository.UpdateAccountParams) (model.Account, error)
	FindByID(ctx context.Context, id string) (model.Account, error)
	List(ctx context.Context, filter repository.AccountListFilter) ([]model.Account, error)
}

type AccountService struct {
	store AccountStore
}

func NewAccountService(store AccountStore) *AccountService {
	return &AccountService{store: store}
}

func (s *AccountService) ListAccountTypes(ctx context.Context) ([]dto.AccountTypeResponse, error) {
	types, err := s.store.ListAccountTypes(ctx)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.AccountTypeResponse, 0, len(types))
	for _, t := range types {
		responses = append(responses, dto.AccountTypeResponse{
			ID: t.ID, Code: t.Code, Name: t.Name,
			NormalBalance: t.NormalBalance, FinancialStatement: t.FinancialStatement, SortOrder: t.SortOrder,
		})
	}
	return responses, nil
}

func (s *AccountService) ListAccountCategories(ctx context.Context) ([]dto.AccountCategoryResponse, error) {
	categories, err := s.store.ListAccountCategories(ctx)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.AccountCategoryResponse, 0, len(categories))
	for _, c := range categories {
		responses = append(responses, dto.AccountCategoryResponse{
			ID: c.ID, AccountTypeID: c.AccountTypeID, Code: c.Code, Name: c.Name,
			ReportSection: c.ReportSection, SortOrder: c.SortOrder,
		})
	}
	return responses, nil
}

func (s *AccountService) Create(ctx context.Context, request dto.CreateAccountRequest) (dto.AccountResponse, error) {
	if strings.TrimSpace(request.AccountCode) == "" {
		return dto.AccountResponse{}, finance.ValidationError("account_code is required")
	}
	if strings.TrimSpace(request.AccountName) == "" {
		return dto.AccountResponse{}, finance.ValidationError("account_name is required")
	}
	if request.NormalBalance != "debit" && request.NormalBalance != "credit" {
		return dto.AccountResponse{}, finance.ValidationError("normal_balance must be 'debit' or 'credit'")
	}
	if !request.IsHeader && strings.TrimSpace(request.AccountCategoryID) == "" {
		return dto.AccountResponse{}, finance.ValidationError("account_category_id is required for non-header accounts")
	}
	openingDate, err := parseOptionalDate(request.OpeningBalanceDate)
	if err != nil {
		return dto.AccountResponse{}, err
	}
	account, err := s.store.Create(ctx, repository.CreateAccountParams{
		AccountCode:        strings.TrimSpace(request.AccountCode),
		AccountName:        strings.TrimSpace(request.AccountName),
		AccountCategoryID:  trimmedOrNil(request.AccountCategoryID),
		ParentAccountID:    trimmedOrNil(request.ParentAccountID),
		IsHeader:           request.IsHeader,
		NormalBalance:      request.NormalBalance,
		OpeningBalance:     request.OpeningBalance,
		OpeningBalanceDate: openingDate,
		Description:        request.Description,
	})
	if err != nil {
		return dto.AccountResponse{}, mapAccountError(err)
	}
	return accountResponse(account), nil
}

func (s *AccountService) Update(ctx context.Context, id string, request dto.UpdateAccountRequest) (dto.AccountResponse, error) {
	if strings.TrimSpace(request.AccountName) == "" {
		return dto.AccountResponse{}, finance.ValidationError("account_name is required")
	}
	openingDate, err := parseOptionalDate(request.OpeningBalanceDate)
	if err != nil {
		return dto.AccountResponse{}, err
	}
	account, err := s.store.Update(ctx, repository.UpdateAccountParams{
		ID:                 strings.TrimSpace(id),
		AccountName:        strings.TrimSpace(request.AccountName),
		AccountCategoryID:  trimmedOrNil(request.AccountCategoryID),
		ParentAccountID:    trimmedOrNil(request.ParentAccountID),
		IsActive:           request.IsActive,
		OpeningBalance:     request.OpeningBalance,
		OpeningBalanceDate: openingDate,
		Description:        request.Description,
	})
	if err != nil {
		return dto.AccountResponse{}, mapAccountError(err)
	}
	return accountResponse(account), nil
}

func (s *AccountService) Get(ctx context.Context, id string) (dto.AccountResponse, error) {
	account, err := s.store.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.AccountResponse{}, mapAccountError(err)
	}
	return accountResponse(account), nil
}

func (s *AccountService) List(ctx context.Context, query dto.AccountListQuery) ([]dto.AccountResponse, error) {
	accounts, err := s.store.List(ctx, repository.AccountListFilter{
		IncludeInactive: query.IncludeInactive,
		CategoryID:      query.CategoryID,
	})
	if err != nil {
		return nil, err
	}
	responses := make([]dto.AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		responses = append(responses, accountResponse(a))
	}
	return responses, nil
}
