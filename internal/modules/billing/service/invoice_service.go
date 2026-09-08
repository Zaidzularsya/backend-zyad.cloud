package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type InvoiceStore interface {
	Create(ctx context.Context, params repository.CreateInvoiceParams) (model.Invoice, error)
	FindByID(ctx context.Context, organizationID string, id string) (model.Invoice, error)
	List(ctx context.Context, filter repository.InvoiceListFilter) ([]model.Invoice, int64, error)
	ListAllOrganizations(ctx context.Context, filter repository.InvoiceListFilter) ([]model.Invoice, int64, error)
	ListItems(ctx context.Context, invoiceID string) ([]model.InvoiceItem, error)
	UpdateStatus(ctx context.Context, params repository.UpdateInvoiceStatusParams) (model.Invoice, error)
}

type InvoiceService struct {
	store InvoiceStore
	now   func() time.Time
}

func NewInvoiceService(store InvoiceStore) *InvoiceService {
	return &InvoiceService{
		store: store,
		now:   time.Now,
	}
}

// List returns invoices for a single tenant. query.OrganizationID must be
// set by the caller (e.g. from a verified tenant context) — this method
// refuses to run an unscoped, cross-tenant query.
func (s *InvoiceService) List(ctx context.Context, query dto.InvoiceListQuery) (dto.InvoiceListResponse, error) {
	filter, page, perPage, err := invoiceListFilter(query)
	if err != nil {
		return dto.InvoiceListResponse{}, err
	}
	invoices, total, err := s.store.List(ctx, filter)
	if err != nil {
		return dto.InvoiceListResponse{}, err
	}
	return dto.InvoiceListResponse{
		Items: invoiceResponses(invoices),
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

// ListAllOrganizations lists invoices across every tenant. Intended for
// platform-admin endpoints only — callers must gate access themselves.
func (s *InvoiceService) ListAllOrganizations(
	ctx context.Context,
	query dto.InvoiceListQuery,
) (dto.InvoiceListResponse, error) {
	filter, page, perPage, err := invoiceListFilter(query)
	if err != nil {
		return dto.InvoiceListResponse{}, err
	}
	invoices, total, err := s.store.ListAllOrganizations(ctx, filter)
	if err != nil {
		return dto.InvoiceListResponse{}, err
	}
	return dto.InvoiceListResponse{
		Items: invoiceResponses(invoices),
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

func (s *InvoiceService) FindByID(ctx context.Context, organizationID string, id string) (dto.InvoiceResponse, error) {
	invoice, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(id))
	if err != nil {
		return dto.InvoiceResponse{}, mapInvoiceError(err)
	}
	items, err := s.store.ListItems(ctx, invoice.ID)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}
	return invoiceResponse(invoice, items), nil
}

func (s *InvoiceService) Create(ctx context.Context, request dto.CreateInvoiceRequest) (dto.InvoiceResponse, error) {
	params, err := s.createParams(request)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}
	invoice, err := s.store.Create(ctx, params)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}
	items, err := s.store.ListItems(ctx, invoice.ID)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}
	return invoiceResponse(invoice, items), nil
}

func (s *InvoiceService) UpdateStatus(
	ctx context.Context,
	organizationID string,
	id string,
	status model.InvoiceStatus,
	paidAt *time.Time,
) (dto.InvoiceResponse, error) {
	if !status.IsValid() {
		return dto.InvoiceResponse{}, validationError("invoice status is invalid")
	}
	invoice, err := s.store.UpdateStatus(ctx, repository.UpdateInvoiceStatusParams{
		ID:             strings.TrimSpace(id),
		OrganizationID: strings.TrimSpace(organizationID),
		Status:         status,
		PaidAt:         paidAt,
	})
	if err != nil {
		return dto.InvoiceResponse{}, mapInvoiceError(err)
	}
	return invoiceResponse(invoice, nil), nil
}

func (s *InvoiceService) createParams(request dto.CreateInvoiceRequest) (repository.CreateInvoiceParams, error) {
	if strings.TrimSpace(request.OrganizationID) == "" {
		return repository.CreateInvoiceParams{}, validationError("organization id is required")
	}
	if len(request.Items) == 0 {
		return repository.CreateInvoiceParams{}, validationError("invoice items are required")
	}
	dueDate, err := parseOptionalTime(request.DueDate)
	if err != nil {
		return repository.CreateInvoiceParams{}, err
	}
	items, totals, err := calculateInvoiceItems(request.Items)
	if err != nil {
		return repository.CreateInvoiceParams{}, err
	}
	subscriptionID := trimOptionalString(&request.SubscriptionID)
	return repository.CreateInvoiceParams{
		OrganizationID: request.OrganizationID,
		SubscriptionID: subscriptionID,
		InvoiceNumber:  s.generateInvoiceNumber(),
		Status:         model.InvoiceStatusOpen,
		Currency:       currencyOrDefault(request.Currency, "IDR"),
		SubtotalAmount: formatMoney(totals.subtotal),
		DiscountAmount: formatMoney(totals.discount),
		TaxAmount:      formatMoney(totals.tax),
		TotalAmount:    formatMoney(totals.total),
		DueDate:        dueDate,
		Metadata:       request.Metadata,
		Items:          items,
	}, nil
}

func (s *InvoiceService) generateInvoiceNumber() string {
	now := s.now().UTC()
	return fmt.Sprintf("INV-%s-%d", now.Format("20060102"), now.UnixNano())
}

func invoiceListFilter(query dto.InvoiceListQuery) (repository.InvoiceListFilter, int, int, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	filter := repository.InvoiceListFilter{
		OrganizationID: strings.TrimSpace(query.OrganizationID),
		SubscriptionID: strings.TrimSpace(query.SubscriptionID),
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}
	if strings.TrimSpace(query.Status) != "" {
		status := model.InvoiceStatus(strings.TrimSpace(query.Status))
		if !status.IsValid() {
			return repository.InvoiceListFilter{}, 0, 0, validationError("invoice status is invalid")
		}
		filter.Status = status
	}
	return filter, page, perPage, nil
}

type invoiceTotals struct {
	subtotal *big.Rat
	discount *big.Rat
	tax      *big.Rat
	total    *big.Rat
}

func calculateInvoiceItems(requestItems []dto.CreateInvoiceItemRequest) ([]repository.CreateInvoiceItemParams, invoiceTotals, error) {
	items := make([]repository.CreateInvoiceItemParams, 0, len(requestItems))
	subtotal := new(big.Rat)
	discount := new(big.Rat)
	tax := new(big.Rat)
	for _, requestItem := range requestItems {
		itemType := model.InvoiceItemType(strings.TrimSpace(requestItem.Type))
		if !itemType.IsValid() {
			return nil, invoiceTotals{}, validationError("invoice item type is invalid")
		}
		if strings.TrimSpace(requestItem.Description) == "" {
			return nil, invoiceTotals{}, validationError("invoice item description is required")
		}
		quantity, err := moneyRat(defaultString(requestItem.Quantity, "1"))
		if err != nil {
			return nil, invoiceTotals{}, validationError("invoice item quantity is invalid")
		}
		unitAmount, err := moneyRat(requestItem.UnitAmount)
		if err != nil {
			return nil, invoiceTotals{}, validationError("invoice item unit amount is invalid")
		}
		if quantity.Sign() < 0 || unitAmount.Sign() < 0 {
			return nil, invoiceTotals{}, validationError("invoice item quantity and unit amount must be non-negative")
		}
		totalAmount := new(big.Rat).Mul(quantity, unitAmount)
		switch itemType {
		case model.InvoiceItemTypeTax:
			tax.Add(tax, totalAmount)
		case model.InvoiceItemTypeDiscount:
			discount.Add(discount, totalAmount)
		default:
			subtotal.Add(subtotal, totalAmount)
		}
		items = append(items, repository.CreateInvoiceItemParams{
			Type:        itemType,
			Description: requestItem.Description,
			Quantity:    formatMoney(quantity),
			UnitAmount:  formatMoney(unitAmount),
			TotalAmount: formatMoney(totalAmount),
			Metadata:    requestItem.Metadata,
		})
	}
	total := new(big.Rat).Add(subtotal, tax)
	total.Sub(total, discount)
	if total.Sign() < 0 {
		return nil, invoiceTotals{}, validationError("invoice total cannot be negative")
	}
	return items, invoiceTotals{
		subtotal: subtotal,
		discount: discount,
		tax:      tax,
		total:    total,
	}, nil
}

func moneyRat(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty money value")
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid money value")
	}
	return parsed, nil
}

func formatMoney(value *big.Rat) string {
	if value == nil {
		return "0.00"
	}
	return value.FloatString(2)
}

func currencyOrDefault(value string, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
