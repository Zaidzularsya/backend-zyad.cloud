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

type ARAPStore interface {
	CreatePartner(ctx context.Context, params repository.CreateBusinessPartnerParams) (model.BusinessPartner, error)
	UpdatePartner(ctx context.Context, params repository.UpdateBusinessPartnerParams) (model.BusinessPartner, error)
	FindPartnerByID(ctx context.Context, id string) (model.BusinessPartner, error)
	ListPartners(ctx context.Context, filter repository.BusinessPartnerListFilter) ([]model.BusinessPartner, error)
	CreateTransaction(ctx context.Context, params repository.CreateARAPTransactionParams) (model.ARAPTransaction, error)
	FindTransactionByID(ctx context.Context, id string) (model.ARAPTransaction, error)
	ListTransactions(ctx context.Context, filter repository.ARAPTransactionListFilter) ([]model.ARAPTransaction, int64, error)
	CreatePayment(ctx context.Context, params repository.CreateARAPPaymentParams) (model.ARAPPayment, error)
	AgingReport(ctx context.Context, transactionType model.ARAPTransactionType, asOf time.Time) ([]repository.AgingBucketRow, error)
}

type ARAPService struct {
	store       ARAPStore
	entryNumber EntryNumberGenerator
}

func NewARAPService(store ARAPStore, entryNumber EntryNumberGenerator) *ARAPService {
	return &ARAPService{store: store, entryNumber: entryNumber}
}

func (s *ARAPService) CreatePartner(ctx context.Context, request dto.CreateBusinessPartnerRequest) (dto.BusinessPartnerResponse, error) {
	partnerType := model.PartnerType(strings.TrimSpace(request.PartnerType))
	if !partnerType.IsValid() {
		return dto.BusinessPartnerResponse{}, finance.ValidationError("partner_type must be 'customer' or 'vendor'")
	}
	if strings.TrimSpace(request.Code) == "" || strings.TrimSpace(request.Name) == "" {
		return dto.BusinessPartnerResponse{}, finance.ValidationError("code and name are required")
	}
	partner, err := s.store.CreatePartner(ctx, repository.CreateBusinessPartnerParams{
		PartnerType:      partnerType,
		Code:             request.Code,
		Name:             request.Name,
		TaxID:            request.TaxID,
		Address:          request.Address,
		ControlAccountID: request.ControlAccountID,
	})
	if err != nil {
		return dto.BusinessPartnerResponse{}, mapPartnerError(err)
	}
	return businessPartnerResponse(partner), nil
}

func (s *ARAPService) UpdatePartner(ctx context.Context, id string, request dto.UpdateBusinessPartnerRequest) (dto.BusinessPartnerResponse, error) {
	partner, err := s.store.UpdatePartner(ctx, repository.UpdateBusinessPartnerParams{
		ID: strings.TrimSpace(id), Name: request.Name, TaxID: request.TaxID, Address: request.Address, IsActive: request.IsActive,
	})
	if err != nil {
		return dto.BusinessPartnerResponse{}, mapPartnerError(err)
	}
	return businessPartnerResponse(partner), nil
}

func (s *ARAPService) ListPartners(ctx context.Context, query dto.BusinessPartnerListQuery) ([]dto.BusinessPartnerResponse, error) {
	partners, err := s.store.ListPartners(ctx, repository.BusinessPartnerListFilter{
		PartnerType:     model.PartnerType(strings.TrimSpace(query.PartnerType)),
		IncludeInactive: query.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	responses := make([]dto.BusinessPartnerResponse, 0, len(partners))
	for _, p := range partners {
		responses = append(responses, businessPartnerResponse(p))
	}
	return responses, nil
}

func (s *ARAPService) CreateTransaction(ctx context.Context, request dto.CreateARAPTransactionRequest, actorID string) (dto.ARAPTransactionResponse, error) {
	transactionType := model.ARAPTransactionType(strings.TrimSpace(request.TransactionType))
	if !transactionType.IsValid() {
		return dto.ARAPTransactionResponse{}, finance.ValidationError("transaction_type must be 'receivable' or 'payable'")
	}
	transactionDate, err := parseRequiredDate(request.TransactionDate)
	if err != nil {
		return dto.ARAPTransactionResponse{}, err
	}
	dueDate, err := parseRequiredDate(request.DueDate)
	if err != nil {
		return dto.ARAPTransactionResponse{}, err
	}
	amount, err := moneyRat(request.Amount)
	if err != nil || amount.Sign() <= 0 {
		return dto.ARAPTransactionResponse{}, finance.ValidationError("amount must be greater than zero")
	}

	entryNumber, err := s.entryNumber.NextEntryNumber(ctx, transactionDate.Year())
	if err != nil {
		return dto.ARAPTransactionResponse{}, err
	}

	transaction, err := s.store.CreateTransaction(ctx, repository.CreateARAPTransactionParams{
		PartnerID:       strings.TrimSpace(request.PartnerID),
		TransactionType: transactionType,
		TransactionDate: transactionDate,
		DueDate:         dueDate,
		ReferenceNumber: request.ReferenceNumber,
		Amount:          formatMoney(amount),
		ContraAccountID: strings.TrimSpace(request.ContraAccountID),
		Description:     request.Description,
		EntryNumber:     entryNumber,
		CreatedBy:       trimmedOrNil(actorID),
	})
	if err != nil {
		return dto.ARAPTransactionResponse{}, mapARAPError(err)
	}
	return arapTransactionResponse(transaction), nil
}

func (s *ARAPService) ListTransactions(ctx context.Context, query dto.ARAPTransactionListQuery) (dto.ARAPTransactionListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	transactions, total, err := s.store.ListTransactions(ctx, repository.ARAPTransactionListFilter{
		PartnerID:       query.PartnerID,
		TransactionType: model.ARAPTransactionType(strings.TrimSpace(query.TransactionType)),
		Status:          model.ARAPTransactionStatus(strings.TrimSpace(query.Status)),
		Limit:           perPage,
		Offset:          (page - 1) * perPage,
	})
	if err != nil {
		return dto.ARAPTransactionListResponse{}, err
	}
	responses := make([]dto.ARAPTransactionResponse, 0, len(transactions))
	for _, t := range transactions {
		responses = append(responses, arapTransactionResponse(t))
	}
	return dto.ARAPTransactionListResponse{
		Items: responses,
		Meta:  dto.PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPagesOf(perPage, total)},
	}, nil
}

func (s *ARAPService) CreatePayment(ctx context.Context, request dto.CreateARAPPaymentRequest, actorID string) (dto.ARAPPaymentResponse, error) {
	paymentDate, err := parseRequiredDate(request.PaymentDate)
	if err != nil {
		return dto.ARAPPaymentResponse{}, err
	}
	amount, err := moneyRat(request.Amount)
	if err != nil || amount.Sign() <= 0 {
		return dto.ARAPPaymentResponse{}, finance.ValidationError("amount must be greater than zero")
	}

	entryNumber, err := s.entryNumber.NextEntryNumber(ctx, paymentDate.Year())
	if err != nil {
		return dto.ARAPPaymentResponse{}, err
	}

	payment, err := s.store.CreatePayment(ctx, repository.CreateARAPPaymentParams{
		ARAPTransactionID: strings.TrimSpace(request.ARAPTransactionID),
		PaymentDate:       paymentDate,
		Amount:            formatMoney(amount),
		CashBankAccountID: strings.TrimSpace(request.CashBankAccountID),
		Notes:             request.Notes,
		EntryNumber:       entryNumber,
		CreatedBy:         trimmedOrNil(actorID),
	})
	if err != nil {
		return dto.ARAPPaymentResponse{}, mapARAPError(err)
	}
	return arapPaymentResponse(payment), nil
}

func (s *ARAPService) AgingReport(ctx context.Context, query dto.AgingReportQuery) (dto.AgingReportResponse, error) {
	transactionType := model.ARAPTransactionType(strings.TrimSpace(query.TransactionType))
	if !transactionType.IsValid() {
		return dto.AgingReportResponse{}, finance.ValidationError("transaction_type must be 'receivable' or 'payable'")
	}
	asOf, err := parseRequiredDate(query.AsOfDate)
	if err != nil {
		return dto.AgingReportResponse{}, err
	}
	rows, err := s.store.AgingReport(ctx, transactionType, asOf)
	if err != nil {
		return dto.AgingReportResponse{}, err
	}
	return buildAgingReport(rows, asOf), nil
}
