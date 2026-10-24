package service

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type activationReader struct {
	repo repository.ActivationRepository
}

func NewActivationReader(repo repository.ActivationRepository) ActivationReader {
	return &activationReader{repo: repo}
}

func (s *activationReader) OpenActivationsByPages(ctx context.Context, scope tenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}
	return s.repo.ListOpenByPages(ctx, scope, pageIDs)
}

// Input bounds of the activation form.
const (
	maxActivationDueHours  = 30 * 24
	maxActivationDueDays   = 30
	maxActivationNameRunes = 200
	// buyer_contact is also the receivable account phone (max 50).
	maxActivationContactRunes = 50
	maxActivationEmailBytes   = 255
)

// maxActivationDuration caps the publish period bought at once (two years).
var maxActivationDuration = map[domain.ActivationDurationUnit]int{
	domain.ActivationDurationDay:   730,
	domain.ActivationDurationWeek:  104,
	domain.ActivationDurationMonth: 24,
}

// activationPricePattern: plain decimal, at most 16 integer digits (fits the
// receivable invoice money columns) and 2 decimals.
var activationPricePattern = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)

var (
	errActivationPageNotFound = coreerrors.New("PAGE_NOT_FOUND", "landing page not found or already deleted", http.StatusNotFound)
	errActivationOpen         = coreerrors.New("ACTIVATION_OPEN",
		"Masih ada invoice yang menunggu pembayaran untuk halaman ini.", http.StatusConflict)
	errActivationNotEligible = coreerrors.New("ACTIVATION_PAGE_NOT_ELIGIBLE",
		"Hanya halaman GrapesJS yang tidak diarsipkan yang bisa ditagihkan.", http.StatusUnprocessableEntity)
	errActivationFeatureDisabled = coreerrors.New("FEATURE_NOT_ENABLED", "billing feature is not enabled", http.StatusForbidden)
)

func activationValidation(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

type activationService struct {
	activations repository.ActivationRepository
	pages       repository.PageRepository
	features    ActivationFeatureGate
	billing     ActivationBilling
	links       ActivationLinks
	now         func() time.Time
}

func NewActivationService(d ActivationServiceDeps) ActivationService {
	now := d.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &activationService{
		activations: d.Activations, pages: d.Pages, features: d.Features, billing: d.Billing, links: d.Links, now: now,
	}
}

// normalizedActivation is the validated form input.
type normalizedActivation struct {
	buyerName, buyerContact, buyerEmail, price string
	durationValue                              int
	durationUnit                               domain.ActivationDurationUnit
	dueAfter                                   time.Duration
}

func normalizeActivationInput(in ActivationInput) (normalizedActivation, error) {
	n := normalizedActivation{
		buyerName:     strings.TrimSpace(in.BuyerName),
		buyerContact:  strings.TrimSpace(in.BuyerContact),
		buyerEmail:    strings.TrimSpace(in.BuyerEmail),
		durationValue: in.DurationValue,
		durationUnit:  in.DurationUnit,
	}
	if c := utf8.RuneCountInString(n.buyerName); c < 1 || c > maxActivationNameRunes {
		return n, activationValidation("Nama penagih wajib diisi (1–200 karakter).")
	}
	if c := utf8.RuneCountInString(n.buyerContact); c < 1 || c > maxActivationContactRunes {
		return n, activationValidation("Kontak wajib diisi (1–50 karakter).")
	}
	if n.buyerEmail != "" {
		addr, err := mail.ParseAddress(n.buyerEmail)
		if err != nil || addr.Address != n.buyerEmail || len(n.buyerEmail) > maxActivationEmailBytes {
			return n, activationValidation("Alamat email tidak valid.")
		}
	}

	price := strings.TrimSpace(in.Price)
	if !activationPricePattern.MatchString(price) {
		return n, activationValidation("Harga harus angka desimal lebih dari 0 dengan maksimal 2 desimal.")
	}
	rat, ok := new(big.Rat).SetString(price)
	if !ok || rat.Sign() <= 0 {
		return n, activationValidation("Harga harus lebih dari 0.")
	}
	n.price = rat.FloatString(2)

	maxDuration, ok := maxActivationDuration[n.durationUnit]
	if !ok {
		return n, activationValidation("Satuan masa aktif harus day, week, atau month.")
	}
	if n.durationValue < 1 || n.durationValue > maxDuration {
		return n, activationValidation("Masa aktif di luar batas (maksimal 2 tahun).")
	}

	switch {
	case in.InvoiceDueHours != nil && in.InvoiceDueDays != nil:
		return n, activationValidation("Isi salah satu: jatuh tempo dalam jam atau hari.")
	case in.InvoiceDueHours != nil:
		h := *in.InvoiceDueHours
		if h < 1 || h > maxActivationDueHours {
			return n, activationValidation("Jatuh tempo invoice 1–720 jam.")
		}
		n.dueAfter = time.Duration(h) * time.Hour
	case in.InvoiceDueDays != nil:
		d := *in.InvoiceDueDays
		if d < 1 || d > maxActivationDueDays {
			return n, activationValidation("Jatuh tempo invoice 1–30 hari.")
		}
		n.dueAfter = time.Duration(d) * 24 * time.Hour
	default:
		n.dueAfter = time.Hour
	}
	return n, nil
}

func (s *activationService) requireFeatures(ctx context.Context, scope tenant.Scope) error {
	if s.features == nil {
		return errActivationFeatureDisabled
	}
	for _, key := range []string{domain.FeatureLandingEnabled, FeaturePageActivation} {
		if err := s.features.RequireFeature(ctx, scope.OrganizationID(), key); err != nil {
			return err
		}
	}
	return nil
}

func (s *activationService) loadEligiblePage(ctx context.Context, scope tenant.Scope, pageID string) (domain.LandingPage, error) {
	page, err := s.pages.FindByID(ctx, scope, strings.TrimSpace(pageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LandingPage{}, errActivationPageNotFound
	}
	if err != nil {
		return domain.LandingPage{}, err
	}
	// FindByID also returns platform templates; only the scope's own pages qualify.
	if page.OrganizationID != scope.OrganizationID() || page.IsTemplate || page.DeletedAt != nil {
		return domain.LandingPage{}, errActivationPageNotFound
	}
	if page.Builder != domain.PageBuilderGrapesJS || page.Status == domain.PageStatusArchived {
		return domain.LandingPage{}, errActivationNotEligible
	}
	return page, nil
}

func (s *activationService) previewURL(ctx context.Context, scope tenant.Scope, token string) (string, error) {
	if s.links == nil {
		return strings.Replace(ActivationPreviewURLTemplate, "{token}", token, 1), nil
	}
	return s.links.PreviewURL(ctx, scope, token)
}

// CreateInvoice reserves the page's single open activation slot first (voiding
// an overdue one in the same transaction), then issues the receivable invoice.
// When the invoice step fails, the activation (and any invoice already
// created) is voided so the reseller can simply retry; a retry uses a new
// activation and therefore a new invoice idempotency key.
func (s *activationService) CreateInvoice(ctx context.Context, scope tenant.Scope, pageID string, in ActivationInput, userID string) (ActivationResult, error) {
	if !scope.IsValid() {
		return ActivationResult{}, tenant.ErrInvalidScope
	}
	if err := s.requireFeatures(ctx, scope); err != nil {
		return ActivationResult{}, err
	}
	input, err := normalizeActivationInput(in)
	if err != nil {
		return ActivationResult{}, err
	}
	page, err := s.loadEligiblePage(ctx, scope, pageID)
	if err != nil {
		return ActivationResult{}, err
	}
	if s.billing == nil {
		return ActivationResult{}, errors.New("landing: activation billing is not configured")
	}

	now := s.now().UTC()
	// Fast refusal without generating a token; the repository re-checks under lock.
	if open, err := s.activations.FindOpenByPage(ctx, scope, page.ID); err == nil {
		if open.InvoiceDueAt.After(now) {
			return ActivationResult{}, errActivationOpen
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ActivationResult{}, err
	}

	rawToken, tokenHash, err := domain.NewActivationPreviewToken()
	if err != nil {
		return ActivationResult{}, err
	}
	previewURL, err := s.previewURL(ctx, scope, rawToken)
	if err != nil {
		return ActivationResult{}, err
	}

	var email *string
	if input.buyerEmail != "" {
		email = &input.buyerEmail
	}
	activation, voided, err := s.activations.CreateReplacingOverdue(ctx, scope, repository.CreateActivationParams{
		PageID: page.ID, BuyerName: input.buyerName, BuyerContact: input.buyerContact, BuyerEmail: email,
		Price: input.price, DurationValue: input.durationValue, DurationUnit: input.durationUnit,
		PreviewTokenHash: tokenHash, InvoiceDueAt: now.Add(input.dueAfter), CreatedBy: userID,
	}, now)
	switch {
	case errors.Is(err, repository.ErrActivationOpen):
		return ActivationResult{}, errActivationOpen
	case errors.Is(err, pgx.ErrNoRows):
		return ActivationResult{}, errActivationPageNotFound
	case err != nil:
		return ActivationResult{}, err
	}
	if voided != nil {
		slog.InfoContext(ctx, "landing: overdue activation voided by a new activation",
			"page_id", page.ID, "voided_activation_id", voided.ID, "activation_id", activation.ID)
	}

	invoice, err := s.billing.IssueInvoice(ctx, scope, ActivationInvoiceRequest{
		ActivationID: activation.ID, BuyerName: input.buyerName, BuyerContact: input.buyerContact,
		BuyerEmail: input.buyerEmail, Description: "Halaman undangan " + page.Slug, Price: input.price,
		DueAt: activation.InvoiceDueAt,
	}, userID)
	if err == nil && invoice.InvoiceID == "" {
		err = errors.New("landing: activation invoice was not created")
	}
	if err != nil {
		s.rollback(ctx, scope, activation.ID, invoice.InvoiceID, userID)
		return ActivationResult{}, err
	}

	linked, err := s.activations.SetInvoice(ctx, scope, activation.ID, invoice.InvoiceID)
	if err != nil {
		s.rollback(ctx, scope, activation.ID, invoice.InvoiceID, userID)
		return ActivationResult{}, err
	}
	return ActivationResult{Activation: linked, PreviewURL: previewURL, PaymentURL: invoice.PaymentURL}, nil
}

// rollback frees the open slot and cancels an invoice that is not linked to
// any activation (so it can never be paid by mistake). Best effort: failures
// are logged; the stuck activation becomes replaceable once it is overdue.
func (s *activationService) rollback(ctx context.Context, scope tenant.Scope, activationID, invoiceID, userID string) {
	if invoiceID != "" {
		if err := s.billing.VoidInvoice(ctx, scope, invoiceID, "Pembuatan invoice aktivasi halaman gagal", userID); err != nil {
			slog.ErrorContext(ctx, "landing: void orphan activation invoice failed", "invoice_id", invoiceID, "activation_id", activationID, "error", err)
		}
	}
	if _, err := s.activations.Void(ctx, scope, activationID); err != nil {
		slog.ErrorContext(ctx, "landing: void activation after invoice failure failed", "activation_id", activationID, "error", err)
	}
}
