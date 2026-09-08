package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type invoiceService struct {
	repo          repository.InvoiceRepository
	quotationRepo repository.QuotationRepository
}

func NewInvoiceService(repo repository.InvoiceRepository, quotationRepo repository.QuotationRepository) InvoiceService {
	return &invoiceService{repo: repo, quotationRepo: quotationRepo}
}

// computeInvoiceTotals mirrors quotationService.computeTotals but without a
// separate discount_total header field (crm_invoices doesn't have one —
// discounts are only tracked per line item, already folded into LineTotal).
// Same float64-arithmetic caveat applies.
func (s *invoiceService) computeInvoiceTotals(items []InvoiceLineInput, taxTotalInput string) (subtotal string, grandTotal string, lineItems []repository.InvoiceItemInput, err error) {
	var subtotalValue float64

	lineItems = make([]repository.InvoiceItemInput, 0, len(items))
	for i, item := range items {
		quantity, parseErr := parseDecimalOrDefault(item.Quantity, 1)
		if parseErr != nil {
			return "", "", nil, ErrInvalidQuotationAmount
		}
		unitPrice, parseErr := parseDecimalOrDefault(item.UnitPrice, 0)
		if parseErr != nil {
			return "", "", nil, ErrInvalidQuotationAmount
		}
		discountPercent, parseErr := parseDecimalOrDefault(item.DiscountPercent, 0)
		if parseErr != nil {
			return "", "", nil, ErrInvalidQuotationAmount
		}

		lineSubtotal := quantity * unitPrice
		lineTotal := lineSubtotal - (lineSubtotal * discountPercent / 100)
		subtotalValue += lineSubtotal

		lineItems = append(lineItems, repository.InvoiceItemInput{
			Description:     item.Description,
			Quantity:        formatDecimal(quantity),
			UnitPrice:       formatDecimal(unitPrice),
			DiscountPercent: item.DiscountPercent,
			LineTotal:       formatDecimal(lineTotal),
			Position:        i,
		})
	}

	taxTotalValue, err := parseDecimalOrDefault(taxTotalInput, 0)
	if err != nil {
		return "", "", nil, ErrInvalidQuotationAmount
	}

	var lineTotalSum float64
	for _, item := range lineItems {
		v, _ := parseDecimalOrDefault(item.LineTotal, 0)
		lineTotalSum += v
	}
	grandTotalValue := lineTotalSum + taxTotalValue

	return formatDecimal(subtotalValue), formatDecimal(grandTotalValue), lineItems, nil
}

func (s *invoiceService) Create(ctx context.Context, scope coretenant.Scope, input CreateInvoiceInput) (domain.Invoice, error) {
	params := repository.CreateInvoiceParams{
		QuotationID:   input.QuotationID,
		DealID:        input.DealID,
		ContactID:     input.ContactID,
		CompanyID:     input.CompanyID,
		InvoiceNumber: input.InvoiceNumber,
		IssueDate:     input.IssueDate,
		DueDate:       input.DueDate,
		Currency:      input.Currency,
		CreatedBy:     input.CreatedBy,
	}

	if input.QuotationID != "" && len(input.Items) == 0 {
		// Copy items/totals from an existing quotation rather than
		// recomputing — the quotation was already priced (and may have been
		// approved by the customer at those exact figures).
		quotation, err := s.quotationRepo.FindByID(ctx, scope, input.QuotationID)
		if err != nil {
			return domain.Invoice{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
		}
		if params.DealID == "" && quotation.DealID != nil {
			params.DealID = *quotation.DealID
		}
		if params.ContactID == "" && quotation.ContactID != nil {
			params.ContactID = *quotation.ContactID
		}
		if params.CompanyID == "" && quotation.CompanyID != nil {
			params.CompanyID = *quotation.CompanyID
		}
		if params.Currency == "" {
			params.Currency = quotation.Currency
		}
		params.Subtotal = quotation.Subtotal
		params.TaxTotal = quotation.TaxTotal
		params.GrandTotal = quotation.GrandTotal
		items := make([]repository.InvoiceItemInput, 0, len(quotation.Items))
		for _, item := range quotation.Items {
			items = append(items, repository.InvoiceItemInput{
				Description:     item.Description,
				Quantity:        item.Quantity,
				UnitPrice:       item.UnitPrice,
				DiscountPercent: derefString(item.DiscountPercent),
				LineTotal:       item.LineTotal,
				Position:        item.Position,
			})
		}
		params.Items = items
	} else {
		subtotal, grandTotal, items, err := s.computeInvoiceTotals(input.Items, input.TaxTotal)
		if err != nil {
			return domain.Invoice{}, err
		}
		taxTotal := input.TaxTotal
		if taxTotal == "" {
			taxTotal = "0.00"
		}
		params.Subtotal = subtotal
		params.TaxTotal = taxTotal
		params.GrandTotal = grandTotal
		params.Items = items
	}

	return s.repo.Create(ctx, scope, params)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *invoiceService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error) {
	invoice, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Invoice{}, crmmodule.MapNotFound(err, "INVOICE_NOT_FOUND", "invoice not found or already deleted")
	}
	return invoice, nil
}

func (s *invoiceService) List(ctx context.Context, scope coretenant.Scope, filter repository.InvoiceListFilter) ([]domain.Invoice, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *invoiceService) Update(ctx context.Context, scope coretenant.Scope, id string, input UpdateInvoiceInput) (domain.Invoice, error) {
	invoice, err := s.repo.Update(ctx, scope, id, repository.UpdateInvoiceParams{
		QuotationID: input.QuotationID,
		DealID:      input.DealID,
		ContactID:   input.ContactID,
		CompanyID:   input.CompanyID,
		IssueDate:   input.IssueDate,
		DueDate:     input.DueDate,
		UpdatedBy:   input.UpdatedBy,
	})
	if err != nil {
		return domain.Invoice{}, crmmodule.MapNotFound(err, "INVOICE_NOT_FOUND", "invoice not found or already deleted")
	}
	return invoice, nil
}

func (s *invoiceService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "INVOICE_NOT_FOUND", "invoice not found or already deleted")
}

func (s *invoiceService) Send(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Invoice, error) {
	invoice, err := s.repo.Send(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Invoice{}, crmmodule.MapNotFound(err, "INVOICE_NOT_DRAFT", "invoice not found, already deleted, or not draft")
	}
	return invoice, nil
}

func (s *invoiceService) MarkPaid(ctx context.Context, scope coretenant.Scope, id string, amountPaid string, updatedBy string) (domain.Invoice, error) {
	invoice, err := s.repo.MarkPaid(ctx, scope, id, amountPaid, updatedBy)
	if err != nil {
		return domain.Invoice{}, crmmodule.MapNotFound(err, "INVOICE_NOT_PAYABLE", "invoice not found, already deleted, or not sent/overdue")
	}
	return invoice, nil
}

func (s *invoiceService) Cancel(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Invoice, error) {
	invoice, err := s.repo.Cancel(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Invoice{}, crmmodule.MapNotFound(err, "INVOICE_ALREADY_PAID", "invoice not found, already deleted, or already paid")
	}
	return invoice, nil
}
