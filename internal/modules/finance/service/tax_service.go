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

type TaxStore interface {
	ListTypes(ctx context.Context) ([]model.TaxType, error)
	CreateRate(ctx context.Context, params repository.CreateTaxRateParams) (model.TaxRate, error)
	ListRates(ctx context.Context, taxTypeID string) ([]model.TaxRate, error)
	CreateTransaction(ctx context.Context, params repository.CreateTaxTransactionParams) (model.TaxTransaction, error)
	FindTransactionByID(ctx context.Context, id string) (model.TaxTransaction, error)
	ListTransactions(ctx context.Context, filter repository.TaxTransactionListFilter) ([]model.TaxTransaction, int64, error)
	Summary(ctx context.Context, start, end time.Time) ([]model.TaxSummaryRow, error)
}

type TaxService struct {
	store       TaxStore
	entryNumber EntryNumberGenerator
}

func NewTaxService(store TaxStore, entryNumber EntryNumberGenerator) *TaxService {
	return &TaxService{store: store, entryNumber: entryNumber}
}

func (s *TaxService) ListTypes(ctx context.Context) ([]dto.TaxTypeResponse, error) {
	types, err := s.store.ListTypes(ctx)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.TaxTypeResponse, 0, len(types))
	for _, t := range types {
		responses = append(responses, dto.TaxTypeResponse{ID: t.ID, Code: t.Code, Name: t.Name, Category: string(t.Category)})
	}
	return responses, nil
}

func (s *TaxService) CreateRate(ctx context.Context, request dto.CreateTaxRateRequest) (dto.TaxRateResponse, error) {
	rate, err := moneyRat(request.RatePercent)
	if err != nil || rate.Sign() < 0 {
		return dto.TaxRateResponse{}, finance.ValidationError("rate_percent must not be negative")
	}
	effectiveDate, err := parseRequiredDate(request.EffectiveDate)
	if err != nil {
		return dto.TaxRateResponse{}, err
	}
	endDate, err := parseOptionalDate(&request.EndDate)
	if err != nil {
		return dto.TaxRateResponse{}, err
	}

	created, err := s.store.CreateRate(ctx, repository.CreateTaxRateParams{
		TaxTypeID:     strings.TrimSpace(request.TaxTypeID),
		RatePercent:   formatMoney(rate),
		EffectiveDate: effectiveDate,
		EndDate:       endDate,
		Notes:         request.Notes,
	})
	if err != nil {
		return dto.TaxRateResponse{}, mapTaxError(err)
	}
	return taxRateResponse(created), nil
}

func (s *TaxService) ListRates(ctx context.Context, query dto.TaxRateListQuery) ([]dto.TaxRateResponse, error) {
	rates, err := s.store.ListRates(ctx, query.TaxTypeID)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.TaxRateResponse, 0, len(rates))
	for _, r := range rates {
		responses = append(responses, taxRateResponse(r))
	}
	return responses, nil
}

func (s *TaxService) CreateTransaction(ctx context.Context, request dto.CreateTaxTransactionRequest, actorID string) (dto.TaxTransactionResponse, error) {
	direction := model.TaxDirection(strings.TrimSpace(request.Direction))
	if !direction.IsValid() {
		return dto.TaxTransactionResponse{}, finance.ValidationError("direction must be 'increase' or 'decrease'")
	}
	transactionDate, err := parseRequiredDate(request.TransactionDate)
	if err != nil {
		return dto.TaxTransactionResponse{}, err
	}
	amount, err := moneyRat(request.Amount)
	if err != nil || amount.Sign() <= 0 {
		return dto.TaxTransactionResponse{}, finance.ValidationError("amount must be greater than zero")
	}

	entryNumber, err := s.entryNumber.NextEntryNumber(ctx, transactionDate.Year())
	if err != nil {
		return dto.TaxTransactionResponse{}, err
	}

	transaction, err := s.store.CreateTransaction(ctx, repository.CreateTaxTransactionParams{
		TaxTypeID:       strings.TrimSpace(request.TaxTypeID),
		TransactionDate: transactionDate,
		ReferenceNumber: request.ReferenceNumber,
		Amount:          formatMoney(amount),
		Direction:       direction,
		TaxAccountID:    strings.TrimSpace(request.TaxAccountID),
		ContraAccountID: strings.TrimSpace(request.ContraAccountID),
		Description:     request.Description,
		EntryNumber:     entryNumber,
		CreatedBy:       trimmedOrNil(actorID),
	})
	if err != nil {
		return dto.TaxTransactionResponse{}, mapTaxError(err)
	}
	return taxTransactionResponse(transaction), nil
}

func (s *TaxService) ListTransactions(ctx context.Context, query dto.TaxTransactionListQuery) (dto.TaxTransactionListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	startDate, err := parseOptionalDate(&query.StartDate)
	if err != nil {
		return dto.TaxTransactionListResponse{}, err
	}
	endDate, err := parseOptionalDate(&query.EndDate)
	if err != nil {
		return dto.TaxTransactionListResponse{}, err
	}

	transactions, total, err := s.store.ListTransactions(ctx, repository.TaxTransactionListFilter{
		TaxTypeID: query.TaxTypeID,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     perPage,
		Offset:    (page - 1) * perPage,
	})
	if err != nil {
		return dto.TaxTransactionListResponse{}, err
	}
	responses := make([]dto.TaxTransactionResponse, 0, len(transactions))
	for _, t := range transactions {
		responses = append(responses, taxTransactionResponse(t))
	}
	return dto.TaxTransactionListResponse{
		Items: responses,
		Meta:  dto.PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPagesOf(perPage, total)},
	}, nil
}

func (s *TaxService) Summary(ctx context.Context, query dto.TaxSummaryQuery) (dto.TaxSummaryResponse, error) {
	start, err := parseRequiredDate(query.StartDate)
	if err != nil {
		return dto.TaxSummaryResponse{}, err
	}
	end, err := parseRequiredDate(query.EndDate)
	if err != nil {
		return dto.TaxSummaryResponse{}, err
	}
	rows, err := s.store.Summary(ctx, start, end)
	if err != nil {
		return dto.TaxSummaryResponse{}, err
	}
	responseRows := make([]dto.TaxSummaryRowResponse, 0, len(rows))
	for _, r := range rows {
		responseRows = append(responseRows, dto.TaxSummaryRowResponse{
			TaxTypeID: r.TaxTypeID, TaxTypeCode: r.TaxTypeCode, TaxTypeName: r.TaxTypeName,
			Increase: r.Increase, Decrease: r.Decrease, Net: r.Net,
		})
	}
	return dto.TaxSummaryResponse{StartDate: formatDate(start), EndDate: formatDate(end), Rows: responseRows}, nil
}

func taxRateResponse(r model.TaxRate) dto.TaxRateResponse {
	return dto.TaxRateResponse{
		ID: r.ID, TaxTypeID: r.TaxTypeID, TaxTypeCode: r.TaxTypeCode, RatePercent: r.RatePercent,
		EffectiveDate: formatDate(r.EffectiveDate), EndDate: formatOptionalDate(r.EndDate),
		Notes: r.Notes, CreatedAt: formatTime(r.CreatedAt),
	}
}

func taxTransactionResponse(t model.TaxTransaction) dto.TaxTransactionResponse {
	return dto.TaxTransactionResponse{
		ID: t.ID, TaxTypeID: t.TaxTypeID, TaxTypeCode: t.TaxTypeCode, TaxTypeName: t.TaxTypeName,
		TransactionDate: formatDate(t.TransactionDate), ReferenceNumber: t.ReferenceNumber,
		Amount: t.Amount, Direction: string(t.Direction),
		TaxAccountID: t.TaxAccountID, TaxAccountCode: t.TaxAccountCode, TaxAccountName: t.TaxAccountName,
		ContraAccountID: t.ContraAccountID, ContraAccountCode: t.ContraAccountCode, ContraAccountName: t.ContraAccountName,
		Description: t.Description, JournalEntryID: t.JournalEntryID,
		CreatedAt: formatTime(t.CreatedAt), UpdatedAt: formatTime(t.UpdatedAt),
	}
}
