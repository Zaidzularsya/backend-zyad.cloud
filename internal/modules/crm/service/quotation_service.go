package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

var (
	ErrInvalidQuotationAmount = errors.New("invalid quotation amount")
	ErrQuotationLocked        = repository.ErrQuotationLocked
	ErrQuotationNotRevisable  = repository.ErrQuotationNotRevisable
	ErrProductInactive        = errors.New("product is inactive or deleted")
	ErrQuotationDealNotFound  = errors.New("deal not found")
)

// formatDocumentNumber renders a human-readable document number like
// QUO-2026-0001. Shared by quotationService and invoiceService (same
// package).
func formatDocumentNumber(prefix string, year int, seq int) string {
	return fmt.Sprintf("%s-%d-%04d", prefix, year, seq)
}

type quotationService struct {
	repo       repository.QuotationRepository
	numberRepo repository.DocumentCounterRepository
	catalog    CatalogProducts
	deals      repository.DealRepository
	docs       *QuotationDocumentDeps
	channels   *QuotationChannelDeps
	now        func() time.Time
}

func WithQuotationCatalog(p CatalogProducts) QuotationServiceOption {
	return func(s *quotationService) { s.catalog = p }
}

func WithQuotationDeals(d repository.DealRepository) QuotationServiceOption {
	return func(s *quotationService) { s.deals = d }
}

func NewQuotationService(repo repository.QuotationRepository, numberRepo repository.DocumentCounterRepository, opts ...QuotationServiceOption) QuotationService {
	s := &quotationService{repo: repo, numberRepo: numberRepo, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var revisionSuffix = regexp.MustCompile(`-R\d+$`)

func rootQuotationNumber(number string) string { return revisionSuffix.ReplaceAllString(number, "") }

func SuggestDealStatusAfterApprove(q domain.Quotation) string {
	if q.DealID != nil {
		return string(domain.DealStatusWon)
	}
	return ""
}

// applyCatalog mengisi baris ber-product_id dengan snapshot katalog sebagai
// nilai awal; nilai yang dikirim klien (deskripsi, harga, pajak, satuan)
// mengalahkan katalog.
func (s *quotationService) applyCatalog(ctx context.Context, scope coretenant.Scope, lines []QuotationLineInput) ([]QuotationLineInput, error) {
	var ids []string
	for _, l := range lines {
		if l.ProductID != "" {
			ids = append(ids, l.ProductID)
		}
	}
	if len(ids) == 0 {
		return lines, nil
	}
	if s.catalog == nil {
		return nil, ErrProductInactive
	}
	products, err := s.catalog.FindByIDs(ctx, scope, ids)
	if err != nil {
		return nil, err
	}
	out := make([]QuotationLineInput, len(lines))
	for i, l := range lines {
		out[i] = l
		if l.ProductID == "" {
			continue
		}
		p, ok := products[l.ProductID]
		if !ok || !p.IsActive {
			return nil, ErrProductInactive
		}
		if strings.TrimSpace(l.Description) == "" {
			out[i].Description = p.Name
		}
		if strings.TrimSpace(l.UnitPrice) == "" {
			out[i].UnitPrice = p.BasePrice
		}
		if strings.TrimSpace(l.TaxPercent) == "" {
			out[i].TaxPercent = p.TaxPercent
		}
		if strings.TrimSpace(l.Unit) == "" {
			out[i].Unit = p.Unit
		}
		if l.Pricing == (pricing.Attributes{}) {
			out[i].Pricing = p.Pricing
		}
		out[i].SKU = p.SKU
	}
	return out, nil
}

func (s *quotationService) expire(ctx context.Context, scope coretenant.Scope) {
	today := businesstime.DayOf(s.now())
	if _, err := s.repo.ExpireDue(ctx, scope, today); err != nil {
		slog.WarnContext(ctx, "quotation expiry failed", "error", err)
	}
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
	if input.DealID != "" && s.deals != nil {
		deal, err := s.deals.FindByID(ctx, scope, input.DealID)
		if err != nil {
			return domain.Quotation{}, ErrQuotationDealNotFound
		}
		if input.ContactID == "" && deal.ContactID != nil {
			input.ContactID = *deal.ContactID
		}
		if input.CompanyID == "" && deal.CompanyID != nil {
			input.CompanyID = *deal.CompanyID
		}
		if input.Currency == "" {
			input.Currency = deal.Currency
		}
	}
	lines, err := s.applyCatalog(ctx, scope, input.Items)
	if err != nil {
		return domain.Quotation{}, err
	}
	totals, items, err := priceQuotationLines(lines, input.TaxTotal)
	if err != nil {
		return domain.Quotation{}, err
	}
	number := strings.TrimSpace(input.QuotationNumber)
	if number == "" {
		seq, err := s.numberRepo.NextNumber(ctx, scope, "quotation")
		if err != nil {
			return domain.Quotation{}, err
		}
		number = formatDocumentNumber("QUO", s.now().UTC().Year(), seq)
	}
	return s.repo.Create(ctx, scope, repository.CreateQuotationParams{
		DealID: input.DealID, ContactID: input.ContactID, CompanyID: input.CompanyID, QuotationNumber: number,
		ValidUntil: input.ValidUntil, Subtotal: totals.Subtotal, DiscountTotal: totals.DiscountTotal,
		TaxTotal: totals.TaxTotal, GrandTotal: totals.GrandTotal, Currency: input.Currency, Notes: input.Notes,
		OneTimeTotal: totals.OneTimeTotal, FirstInvoiceTotal: totals.FirstInvoiceTotal, RecurringTotals: totals.RecurringTotals,
		Items: items, CreatedBy: input.CreatedBy,
	})
}

func (s *quotationService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Quotation, error) {
	s.expire(ctx, scope)
	quotation, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
	}
	return quotation, nil
}

func (s *quotationService) List(ctx context.Context, scope coretenant.Scope, filter repository.QuotationListFilter) ([]domain.Quotation, int64, error) {
	s.expire(ctx, scope)
	return s.repo.List(ctx, scope, filter)
}

func (s *quotationService) Update(ctx context.Context, scope coretenant.Scope, id string, input UpdateQuotationInput) (domain.Quotation, error) {
	if input.Items != nil {
		lines, err := s.applyCatalog(ctx, scope, input.Items)
		if err != nil {
			return domain.Quotation{}, err
		}
		totals, items, err := priceQuotationLines(lines, "")
		if err != nil {
			return domain.Quotation{}, err
		}
		if _, err := s.repo.ReplaceItems(ctx, scope, id, repository.ReplaceQuotationItemsParams{
			Items: items, Subtotal: totals.Subtotal, DiscountTotal: totals.DiscountTotal,
			TaxTotal: totals.TaxTotal, GrandTotal: totals.GrandTotal, UpdatedBy: input.UpdatedBy,
			OneTimeTotal: totals.OneTimeTotal, FirstInvoiceTotal: totals.FirstInvoiceTotal, RecurringTotals: totals.RecurringTotals,
		}); err != nil {
			return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
		}
	}
	quotation, err := s.repo.Update(ctx, scope, id, repository.UpdateQuotationParams{
		DealID: input.DealID, ContactID: input.ContactID, CompanyID: input.CompanyID,
		ValidUntil: input.ValidUntil, Notes: input.Notes, UpdatedBy: input.UpdatedBy,
	})
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
	}
	return quotation, nil
}

func (s *quotationService) Revise(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.Quotation, error) {
	old, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Quotation{}, err
	}
	lines := make([]QuotationLineInput, 0, len(old.Items))
	for _, it := range old.Items {
		l := QuotationLineInput{Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice,
			TaxPercent: it.TaxPercent, Unit: it.Unit, SKU: it.SKU, Pricing: it.Pricing}
		if it.DiscountPercent != nil {
			l.DiscountPercent = *it.DiscountPercent
		}
		if it.ProductID != nil {
			l.ProductID = *it.ProductID
		}
		lines = append(lines, l)
	}
	totals, items, err := priceQuotationLines(lines, "")
	if err != nil {
		return domain.Quotation{}, err
	}
	next := old.RevisionNo + 1
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	revised, err := s.repo.Revise(ctx, scope, id, repository.CreateQuotationParams{
		DealID: deref(old.DealID), ContactID: deref(old.ContactID), CompanyID: deref(old.CompanyID),
		QuotationNumber: fmt.Sprintf("%s-R%d", rootQuotationNumber(old.QuotationNumber), next), RevisionNo: next,
		ValidUntil: old.ValidUntil, Subtotal: totals.Subtotal, DiscountTotal: totals.DiscountTotal,
		TaxTotal: totals.TaxTotal, GrandTotal: totals.GrandTotal, Currency: old.Currency, Notes: old.Notes,
		OneTimeTotal: totals.OneTimeTotal, FirstInvoiceTotal: totals.FirstInvoiceTotal, RecurringTotals: totals.RecurringTotals,
		Items: items, CreatedBy: userID,
	})
	if err != nil {
		return domain.Quotation{}, crmmodule.MapNotFound(err, "QUOTATION_NOT_FOUND", "quotation not found or already deleted")
	}
	return revised, nil
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
