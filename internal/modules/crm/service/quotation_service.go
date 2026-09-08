package service

import (
	"context"
	"errors"
	"strconv"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var ErrInvalidQuotationAmount = errors.New("invalid quotation amount")

type quotationService struct {
	repo repository.QuotationRepository
}

func NewQuotationService(repo repository.QuotationRepository) QuotationService {
	return &quotationService{repo: repo}
}

// computeTotals converts the caller-submitted decimal-string line items into
// priced repository.QuotationItemInput plus header totals.
//
// This is a deliberate simplification versus the rest of this module (and
// the internal/modules/billing convention), which never does arithmetic on
// money values in Go — only ::text-casts stored/computed-in-SQL values.
// Doing float64 arithmetic here risks penny-level rounding drift on large
// quotations; it was accepted for Fase 4 to avoid pulling in a decimal
// library, and should be revisited before this handles high-value contracts.
func (s *quotationService) computeTotals(items []QuotationLineInput, taxTotalInput string) (subtotal string, discountTotal string, grandTotal string, lineItems []repository.QuotationItemInput, err error) {
	var subtotalValue, discountTotalValue float64

	lineItems = make([]repository.QuotationItemInput, 0, len(items))
	for i, item := range items {
		quantity, parseErr := parseDecimalOrDefault(item.Quantity, 1)
		if parseErr != nil {
			return "", "", "", nil, ErrInvalidQuotationAmount
		}
		unitPrice, parseErr := parseDecimalOrDefault(item.UnitPrice, 0)
		if parseErr != nil {
			return "", "", "", nil, ErrInvalidQuotationAmount
		}
		discountPercent, parseErr := parseDecimalOrDefault(item.DiscountPercent, 0)
		if parseErr != nil {
			return "", "", "", nil, ErrInvalidQuotationAmount
		}

		lineSubtotal := quantity * unitPrice
		discountAmount := lineSubtotal * discountPercent / 100
		lineTotal := lineSubtotal - discountAmount

		subtotalValue += lineSubtotal
		discountTotalValue += discountAmount

		var discountPercentPtr string
		if item.DiscountPercent != "" {
			discountPercentPtr = item.DiscountPercent
		}

		lineItems = append(lineItems, repository.QuotationItemInput{
			Description:     item.Description,
			Quantity:        formatDecimal(quantity),
			UnitPrice:       formatDecimal(unitPrice),
			DiscountPercent: discountPercentPtr,
			LineTotal:       formatDecimal(lineTotal),
			Position:        i,
		})
	}

	taxTotalValue, err := parseDecimalOrDefault(taxTotalInput, 0)
	if err != nil {
		return "", "", "", nil, ErrInvalidQuotationAmount
	}

	grandTotalValue := subtotalValue - discountTotalValue + taxTotalValue

	return formatDecimal(subtotalValue), formatDecimal(discountTotalValue), formatDecimal(grandTotalValue), lineItems, nil
}

func parseDecimalOrDefault(value string, fallback float64) (float64, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseFloat(value, 64)
}

func formatDecimal(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func (s *quotationService) Create(ctx context.Context, scope coretenant.Scope, input CreateQuotationInput) (domain.Quotation, error) {
	subtotal, discountTotal, grandTotal, items, err := s.computeTotals(input.Items, input.TaxTotal)
	if err != nil {
		return domain.Quotation{}, err
	}
	taxTotal := input.TaxTotal
	if taxTotal == "" {
		taxTotal = "0.00"
	}

	return s.repo.Create(ctx, scope, repository.CreateQuotationParams{
		DealID:          input.DealID,
		ContactID:       input.ContactID,
		CompanyID:       input.CompanyID,
		QuotationNumber: input.QuotationNumber,
		ValidUntil:      input.ValidUntil,
		Subtotal:        subtotal,
		DiscountTotal:   discountTotal,
		TaxTotal:        taxTotal,
		GrandTotal:      grandTotal,
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
