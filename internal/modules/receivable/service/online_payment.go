package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/platform/doku"
)

// DokuNumberPrefix membedakan nomor DOKU invoice receivable dari invoice billing langganan.
const DokuNumberPrefix = "RCV-"

const (
	onlinePaymentFeature   = "receivable.online_payment"
	defaultCheckoutMinutes = 60
	statusSyncMinInterval  = 10 * time.Second
	checkoutStatusPending  = "pending"
	checkoutStatusPaid     = "paid"
)

// ErrOnlinePaymentUnavailable: org tidak punya entitlement receivable.online_payment.
var ErrOnlinePaymentUnavailable = errors.New("online payment unavailable")

// FeatureGate memeriksa entitlement org; error berarti tidak diizinkan.
type FeatureGate = Entitlements

type OnlinePaymentConfig struct {
	FrontendURL, NotificationURL string
	SessionMinutes               int
}

// DokuNotification adalah notifikasi/status DOKU yang sudah dinormalisasi.
type DokuNotification struct {
	InvoiceNumber, Status, Amount, ProviderReference, PaymentMethod string
	PaidAt                                                          *time.Time
	Payload                                                         map[string]any
}

type OnlinePayment struct {
	cfg       OnlinePaymentConfig
	client    doku.Client
	checkouts repository.CheckoutRepository
	invoices  InvoiceService
	payments  PaymentService
	links     LinkResolver
	scopes    PublicScopeResolver
	gate      FeatureGate
	now       func() time.Time

	mu       sync.Mutex
	lastSync map[string]time.Time
}

func NewOnlinePayment(cfg OnlinePaymentConfig, client doku.Client, checkouts repository.CheckoutRepository, invoices InvoiceService,
	payments PaymentService, links LinkResolver, scopes PublicScopeResolver, gate FeatureGate, now func() time.Time) *OnlinePayment {
	if now == nil {
		now = time.Now
	}
	if cfg.SessionMinutes <= 0 {
		cfg.SessionMinutes = defaultCheckoutMinutes
	}
	cfg.FrontendURL = strings.TrimRight(cfg.FrontendURL, "/")
	return &OnlinePayment{cfg: cfg, client: client, checkouts: checkouts, invoices: invoices, payments: payments,
		links: links, scopes: scopes, gate: gate, now: now, lastSync: map[string]time.Time{}}
}

func payable(inv domain.Invoice) bool {
	return inv.Status == domain.InvoiceIssued || inv.Status == domain.InvoiceOverdue
}

func remainingOf(inv domain.Invoice) (*big.Rat, bool) {
	grand, ok1 := new(big.Rat).SetString(inv.GrandTotal)
	paid, ok2 := new(big.Rat).SetString(inv.AmountPaid)
	if !ok1 || !ok2 {
		return nil, false
	}
	return grand.Sub(grand, paid), true
}

// ceilRupiah membulatkan ke atas ke rupiah bulat.
func ceilRupiah(r *big.Rat) int64 {
	q, m := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if m.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

func (p *OnlinePayment) load(ctx context.Context, token string) (publicInvoice, error) {
	return loadPublicInvoice(ctx, p.links, p.scopes, p.invoices, p.now(), token)
}

// Checkout membuat (atau memakai ulang) sesi DOKU untuk sisa tagihan invoice pada link publik.
func (p *OnlinePayment) Checkout(ctx context.Context, token string) (string, time.Time, error) {
	d, err := p.load(ctx, token)
	if err != nil {
		return "", time.Time{}, err
	}
	if p.gate == nil || p.gate.RequireFeature(ctx, d.scope.OrganizationID(), onlinePaymentFeature) != nil {
		return "", time.Time{}, ErrOnlinePaymentUnavailable
	}
	if !payable(d.inv) {
		return "", time.Time{}, repository.ErrInvoiceNotPayable
	}
	remaining, ok := remainingOf(d.inv)
	if !ok || remaining.Sign() <= 0 {
		return "", time.Time{}, repository.ErrInvoiceNotPayable
	}
	now := p.now()
	existing, err := p.checkouts.FindReusable(ctx, d.scope, d.inv.ID, now)
	if err == nil {
		return existing.PaymentURL, existing.ExpiresAt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, err
	}

	amount := ceilRupiah(remaining)
	number := DokuNumberPrefix + d.inv.ID
	currency := d.inv.Currency
	if currency == "" {
		currency = "IDR"
	}
	payment, err := p.client.CreatePayment(ctx, doku.CreatePaymentRequest{
		InvoiceNumber:         number,
		Amount:                amount,
		Currency:              currency,
		PaymentDueDateMinutes: p.cfg.SessionMinutes,
		CallbackURL:           fmt.Sprintf("%s/i/%s?paid=1", p.cfg.FrontendURL, token),
		NotificationURL:       p.cfg.NotificationURL,
	})
	if err != nil {
		return "", time.Time{}, coreerrors.Wrap("PAYMENT_CHECKOUT_FAILED", "failed to create payment checkout session", http.StatusBadGateway, err)
	}
	expiresAt := payment.ExpiredDate.UTC()
	if payment.ExpiredDate.IsZero() {
		expiresAt = now.Add(time.Duration(p.cfg.SessionMinutes) * time.Minute).UTC()
	}
	created, err := p.checkouts.Create(ctx, d.scope, repository.Checkout{
		InvoiceID: d.inv.ID, DokuInvoiceNumber: number, Amount: fmt.Sprintf("%d", amount),
		PaymentURL: payment.PaymentURL, ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return created.PaymentURL, created.ExpiresAt, nil
}

func isDokuSuccess(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	return s == "SUCCESS" || s == "PAID"
}

// HandleNotification memproses notifikasi DOKU bernomor RCV-. processed=false untuk status
// non-sukses atau nomor tak dikenal. Galat tak terduga dikembalikan agar DOKU mengirim ulang.
func (p *OnlinePayment) HandleNotification(ctx context.Context, n DokuNotification) (bool, error) {
	checkout, err := p.checkouts.FindByDokuNumber(ctx, n.InvoiceNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.WarnContext(ctx, "receivable: notifikasi DOKU untuk nomor tak dikenal", "invoice_number", n.InvoiceNumber)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !isDokuSuccess(n.Status) {
		return false, nil
	}
	tenantCtx, err := p.scopes.ResolveWorkerOrganization(ctx, checkout.OrganizationID, PublicLinkIdentity)
	if err != nil {
		return false, err
	}
	scope, err := coretenant.NewScope(tenantCtx)
	if err != nil {
		return false, err
	}
	inv, err := p.invoices.Get(ctx, scope, checkout.InvoiceID)
	if err != nil {
		return false, err
	}

	amount := strings.TrimSpace(n.Amount)
	if !validAmount(amount) {
		amount = checkout.Amount
	}
	// Pembayaran dibatasi sisa tagihan (checkout dibulatkan ke atas ke rupiah).
	if payable(inv) {
		if remaining, ok := remainingOf(inv); ok && remaining.Sign() > 0 {
			if got, ok := new(big.Rat).SetString(amount); ok && got.Cmp(remaining) > 0 {
				amount = remaining.FloatString(2)
			}
		}
	}
	paidAt := p.now()
	if n.PaidAt != nil && !n.PaidAt.After(paidAt) {
		paidAt = *n.PaidAt
	}
	reference := strings.TrimSpace(n.ProviderReference)
	if reference == "" {
		reference = n.InvoiceNumber + "-" + paidAt.UTC().Format("20060102")
	}

	_, err = p.payments.RecordProvider(ctx, scope, checkout.InvoiceID, repository.PaymentParams{
		Amount: amount, Method: "doku", Reference: reference, PaidAt: paidAt, ProviderPayload: n.Payload,
	})
	switch {
	case errors.Is(err, repository.ErrInvoiceNotPayable), errors.Is(err, repository.ErrPaymentExceeds):
		slog.ErrorContext(ctx, "receivable: pembayaran DOKU untuk invoice non-payable, perlu refund manual",
			"invoice_id", checkout.InvoiceID, "doku_invoice_number", n.InvoiceNumber, "amount", amount, "reference", reference, "error", err)
		return true, nil
	case err != nil:
		return false, err
	}
	if err := p.checkouts.SetStatus(ctx, checkout.ID, checkoutStatusPaid); err != nil {
		slog.WarnContext(ctx, "receivable: set checkout status failed", "checkout_id", checkout.ID, "error", err)
	}
	return true, nil
}

// SyncStatus menanyakan status ke DOKU saat customer kembali sebelum webhook masuk.
func (p *OnlinePayment) SyncStatus(ctx context.Context, token string) (domain.InvoiceStatus, error) {
	d, err := p.load(ctx, token)
	if err != nil {
		return "", err
	}
	if !payable(d.inv) || !p.allowSync(d.inv.ID) {
		return d.inv.Status, nil
	}
	number := DokuNumberPrefix + d.inv.ID
	checkout, err := p.checkouts.FindByDokuNumber(ctx, number)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && checkout.Status != checkoutStatusPending) {
		return d.inv.Status, nil
	}
	if err != nil {
		return "", err
	}
	st, err := p.client.CheckStatus(ctx, number)
	if err != nil {
		return "", coreerrors.Wrap("PAYMENT_STATUS_FAILED", "failed to check payment status", http.StatusBadGateway, err)
	}
	if !st.IsFinalSuccess() {
		return d.inv.Status, nil
	}
	if _, err := p.HandleNotification(ctx, DokuNotification{
		InvoiceNumber: number, Status: st.Status, Amount: st.Amount, ProviderReference: st.OriginalRequestID,
		PaymentMethod: st.Channel, Payload: st.Raw,
	}); err != nil {
		return "", err
	}
	inv, err := p.invoices.Get(ctx, d.scope, d.inv.ID)
	if err != nil {
		return "", err
	}
	return inv.Status, nil
}

func (p *OnlinePayment) allowSync(invoiceID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if last, ok := p.lastSync[invoiceID]; ok && now.Sub(last) < statusSyncMinInterval {
		return false
	}
	p.lastSync[invoiceID] = now
	return true
}
