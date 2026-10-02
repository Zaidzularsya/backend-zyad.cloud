package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var ErrInvalidQuotationAmount = errors.New("invalid quotation amount")

type quotationService struct {
	repo       repository.QuotationRepository
	numberRepo repository.DocumentCounterRepository
}

func NewQuotationService(repo repository.QuotationRepository, numberRepo repository.DocumentCounterRepository) QuotationService {
	return &quotationService{repo: repo, numberRepo: numberRepo}
}

// formatDocumentNumber renders a human-readable document number like
// QUO-2026-0001. Shared by quotationService and invoiceService (same
// package).
func formatDocumentNumber(prefix string, year int, seq int) string {
	return fmt.Sprintf("%s-%d-%04d", prefix, year, seq)
}

func parseDecimalOrDefault(value string, fallback string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if value == "" {
		return new(big.Rat), nil
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, ErrInvalidQuotationAmount
	}
	return parsed, nil
}

func formatDecimal(value *big.Rat) string {
	if value == nil {
		return "0.00"
	}
	return value.FloatString(2)
}

func (s *quotationService) Create(ctx context.Context, scope coretenant.Scope, input CreateQuotationInput) (domain.Quotation, error) {
	totals, items, err := priceQuotationLines(input.Items, input.TaxTotal)
	if err != nil {
		return domain.Quotation{}, err
	}

	quotationNumber := strings.TrimSpace(input.QuotationNumber)
	if quotationNumber == "" {
		seq, numErr := s.numberRepo.NextNumber(ctx, scope, "quotation")
		if numErr != nil {
			return domain.Quotation{}, numErr
		}
		quotationNumber = formatDocumentNumber("QUO", time.Now().UTC().Year(), seq)
	}

	return s.repo.Create(ctx, scope, repository.CreateQuotationParams{
		DealID:          input.DealID,
		ContactID:       input.ContactID,
		CompanyID:       input.CompanyID,
		QuotationNumber: quotationNumber,
		ValidUntil:      input.ValidUntil,
		Subtotal:        totals.Subtotal,
		DiscountTotal:   totals.DiscountTotal,
		TaxTotal:        totals.TaxTotal,
		GrandTotal:      totals.GrandTotal,
		Currency:        input.Currency,
		Notes:           input.Notes,
		Items:           items,
		CreatedBy:       input.CreatedBy,
	})
}

func (s *quotationService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Quotation, error) {
	quotation, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
	}
	return quotation, nil
}

func (s *quotationService) List(ctx context.Context, scope coretenant.Scope, filter repository.QuotationListFilter) ([]domain.Quotation, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *quotationService) Update(ctx context.Context, scope coretenant.Scope, id string, input UpdateQuotationInput) (domain.Quotation, error) {
	quotation, err := s.repo.Update(ctx, scope, id, repository.UpdateQuotationParams{
		DealID:     input.DealID,
		ContactID:  input.ContactID,
		CompanyID:  input.CompanyID,
		ValidUntil: input.ValidUntil,
		Notes:      input.Notes,
		UpdatedBy:  input.UpdatedBy,
	})
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
	}
	return quotation, nil
}

func (s *quotationService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
}

func (s *quotationService) Send(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	quotation, err := s.repo.Send(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_DRAFT", "quotation not found, already deleted, or not draft")
	}
	return quotation, nil
}

func (s *quotationService) Approve(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	quotation, err := s.repo.Approve(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_SENT", "quotation not found, already deleted, or not sent")
	}
	return quotation, nil
}

func (s *quotationService) Reject(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	quotation, err := s.repo.Reject(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_SENT", "quotation not found, already deleted, or not sent")
	}
	return quotation, nil
}
