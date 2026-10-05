package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
	"zyad.cloud/internal/shared/publiclink"
)

type invoiceService struct {
	accounts    repository.AccountRepository
	invoices    repository.InvoiceRepository
	counters    repository.CounterRepository
	settings    repository.SettingsRepository
	issuers     IssuerSource
	files       FileStore
	renderer    PDFRenderer
	links       Links
	sender      AutoSender
	listeners   *Registry
	frontendURL string
	now         func() time.Time
}

func NewInvoiceService(d InvoiceDeps) InvoiceService {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &invoiceService{
		accounts: d.Accounts, invoices: d.Invoices, counters: d.Counters, settings: d.Settings, issuers: d.Issuers,
		files: d.Files, renderer: d.Renderer, links: d.Links, sender: d.Sender, listeners: d.Listeners,
		frontendURL: d.FrontendURL, now: now,
	}
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func invalidInvoice(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInvoice, fmt.Sprintf(format, args...))
}

func validPeriod(start, end *time.Time) bool {
	return start == nil || end == nil || !end.Before(*start)
}

// buildParams memvalidasi input dan menghitung harga. Validasi format id (UUID) ada di handler.
func (s *invoiceService) buildParams(ctx context.Context, scope coretenant.Scope, in InvoiceInput, userID string) (repository.CreateInvoiceParams, error) {
	var zero repository.CreateInvoiceParams
	accountID := strings.TrimSpace(in.AccountID)
	if accountID == "" {
		return zero, receivable.ErrAccountNotFound
	}
	if _, err := s.accounts.FindByID(ctx, scope, accountID); errors.Is(err, pgx.ErrNoRows) {
		return zero, receivable.ErrAccountNotFound
	} else if err != nil {
		return zero, err
	}

	sourceType := in.SourceType
	switch sourceType {
	case "", domain.SourceManual:
		sourceType = domain.SourceManual
	case domain.SourceSalesOrder, domain.SourceContract:
	default:
		return zero, invalidInvoice("unknown source type %q", sourceType)
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = "IDR"
	}
	if !currencyPattern.MatchString(currency) {
		return zero, invalidInvoice("currency must be a 3-letter code")
	}
	if len(in.IdempotencyKey) > 100 {
		return zero, invalidInvoice("idempotency key must be at most 100 characters")
	}
	if !validPeriod(in.PeriodStart, in.PeriodEnd) {
		return zero, invalidInvoice("period end must not be before period start")
	}

	channels := in.Channels
	if len(channels) == 0 {
		settings, err := s.settings.Get(ctx, scope)
		if err != nil {
			return zero, err
		}
		channels = settings.DefaultChannels
	}
	channels, err := normalizeChannels(channels)
	if err != nil {
		return zero, err
	}

	if len(in.Lines) == 0 {
		return zero, invalidInvoice("at least one line is required")
	}
	lines := make([]pricing.LineInput, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = l.LineInput
		if !validPeriod(l.PeriodStart, l.PeriodEnd) {
			return zero, invalidInvoice("line %d: period end must not be before period start", i+1)
		}
		if utf8.RuneCountInString(strings.TrimSpace(l.Unit)) > 30 || utf8.RuneCountInString(strings.TrimSpace(l.SKU)) > 64 {
			return zero, invalidInvoice("line %d: unit max 30 and sku max 64 characters", i+1)
		}
	}
	totals, priced, err := pricing.PriceLines(lines)
	if err != nil {
		return zero, invalidInvoice("%v", err)
	}
	items := make([]repository.InvoiceItemParams, len(priced))
	for i, p := range priced {
		l := in.Lines[i]
		items[i] = repository.InvoiceItemParams{
			Line: p, Unit: strings.TrimSpace(l.Unit), ProductID: l.ProductID, SKU: strings.TrimSpace(l.SKU),
			ContractItemID: l.ContractItemID, SourceLineID: l.SourceLineID, PeriodStart: l.PeriodStart, PeriodEnd: l.PeriodEnd,
		}
	}
	return repository.CreateInvoiceParams{
		AccountID: accountID, SourceType: string(sourceType), SourceID: in.SourceID, ContractID: in.ContractID,
		IdempotencyKey: in.IdempotencyKey, Currency: currency, Notes: strings.TrimSpace(in.Notes), PICUserID: in.PICUserID,
		CreatedBy: userID, Channels: channels, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, Totals: totals, Items: items,
	}, nil
}

func normalizeChannels(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c != "email" && c != "whatsapp" {
			return nil, invalidInvoice("channel must be email or whatsapp")
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, invalidInvoice("at least one channel is required")
	}
	return out, nil
}

func (s *invoiceService) CreateDraft(ctx context.Context, scope coretenant.Scope, in InvoiceInput, userID string) (domain.Invoice, error) {
	p, err := s.buildParams(ctx, scope, in, userID)
	if err != nil {
		return domain.Invoice{}, err
	}
	return s.invoices.Create(ctx, scope, p)
}

func (s *invoiceService) UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, in InvoiceInput, userID string) (domain.Invoice, error) {
	p, err := s.buildParams(ctx, scope, in, userID)
	if err != nil {
		return domain.Invoice{}, err
	}
	inv, err := s.invoices.ReplaceDraft(ctx, scope, id, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, receivable.ErrInvoiceNotFound
	}
	return inv, err
}

func (s *invoiceService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error) {
	inv, err := s.invoices.FindByID(ctx, scope, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, receivable.ErrInvoiceNotFound
	}
	return inv, err
}

func (s *invoiceService) List(ctx context.Context, scope coretenant.Scope, f repository.InvoiceListFilter) ([]domain.Invoice, int64, error) {
	return s.invoices.List(ctx, scope, f)
}

func (s *invoiceService) Void(ctx context.Context, scope coretenant.Scope, id, reason, userID string) (domain.Invoice, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 1 || n > 500 {
		return domain.Invoice{}, ErrVoidReasonRequired
	}
	inv, err := s.invoices.Void(ctx, scope, id, reason, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, receivable.ErrInvoiceNotFound
	}
	if err != nil {
		return domain.Invoice{}, err
	}
	if s.links != nil {
		if err := s.links.Revoke(ctx, scope, publiclink.DocumentInvoice, inv.ID); err != nil {
			slog.WarnContext(ctx, "receivable: revoke invoice link failed", "invoice_id", inv.ID, "error", err)
		}
	}
	if err := s.RefreshSnapshot(ctx, scope, inv.ID); err != nil {
		slog.WarnContext(ctx, "receivable: refresh void snapshot failed", "invoice_id", inv.ID, "error", err)
	}
	return s.Get(ctx, scope, inv.ID)
}
