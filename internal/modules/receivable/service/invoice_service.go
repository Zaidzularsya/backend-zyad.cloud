package service

import (
	"context"
	"errors"
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
	members     MemberDirectory
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
		files: d.Files, renderer: d.Renderer, links: d.Links, sender: d.Sender, members: d.Members, listeners: d.Listeners,
		frontendURL: d.FrontendURL, now: now,
	}
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

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
		return zero, invalidInvoice("Jenis sumber tidak dikenal.")
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = "IDR"
	}
	if !currencyPattern.MatchString(currency) {
		return zero, invalidInvoice("Mata uang harus kode 3 huruf, mis. IDR.")
	}
	if len(in.IdempotencyKey) > 100 {
		return zero, invalidInvoice("Kunci idempotensi maksimal 100 karakter.")
	}
	if !validPeriod(in.PeriodStart, in.PeriodEnd) {
		return zero, invalidInvoice("Akhir periode tidak boleh sebelum awal periode.")
	}

	if pic := strings.TrimSpace(in.PICUserID); pic != "" && s.members != nil {
		if _, err := s.members.Find(ctx, scope, pic); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return zero, invalidInvoice("PIC harus anggota aktif organisasi ini.")
			}
			return zero, err
		}
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
		return zero, invalidInvoice("Tambahkan minimal satu baris.")
	}
	lines := make([]pricing.LineInput, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = l.LineInput
		if !validPeriod(l.PeriodStart, l.PeriodEnd) {
			return zero, invalidInvoice("Baris %d: akhir periode tidak boleh sebelum awal periode.", i+1)
		}
		if utf8.RuneCountInString(strings.TrimSpace(l.Unit)) > 30 || utf8.RuneCountInString(strings.TrimSpace(l.SKU)) > 64 {
			return zero, invalidInvoice("Baris %d: satuan maksimal 30 dan SKU maksimal 64 karakter.", i+1)
		}
	}
	totals, priced, err := pricing.PriceLines(lines)
	if err != nil {
		return zero, invalidInvoice("Baris tidak valid: deskripsi 1–500 karakter, qty/harga ≥ 0, diskon & pajak 0–100, atribut harga valid.")
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
			return nil, invalidInvoice("Kanal harus email atau whatsapp.")
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, invalidInvoice("Pilih minimal satu kanal.")
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
