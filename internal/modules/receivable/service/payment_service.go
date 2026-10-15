package service

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

var moneyPattern = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)

type paymentService struct {
	payments  repository.PaymentRepository
	invoices  InvoiceService
	listeners *Registry
	now       func() time.Time
}

func NewPaymentService(d PaymentDeps) PaymentService {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &paymentService{payments: d.Payments, invoices: d.Invoices, listeners: d.Listeners, now: now}
}

func validAmount(v string) bool {
	if !moneyPattern.MatchString(v) {
		return false
	}
	r, ok := new(big.Rat).SetString(v)
	return ok && r.Sign() > 0
}

func (s *paymentService) record(ctx context.Context, scope coretenant.Scope, invoiceID string, p repository.PaymentParams) (domain.Invoice, domain.Payment, error) {
	if !validAmount(p.Amount) || utf8.RuneCountInString(p.Reference) > 150 || utf8.RuneCountInString(p.Note) > 500 {
		return domain.Invoice{}, domain.Payment{}, ErrInvalidPayment
	}
	now := s.now()
	if p.PaidAt.IsZero() {
		p.PaidAt = now
	}
	if p.PaidAt.After(now) {
		return domain.Invoice{}, domain.Payment{}, ErrInvalidPayment
	}
	p.PaidAt = p.PaidAt.UTC()
	inv, pay, created, err := s.payments.Record(ctx, scope, invoiceID, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, domain.Payment{}, receivable.ErrInvoiceNotFound
	}
	if err != nil {
		return domain.Invoice{}, domain.Payment{}, err
	}
	if created && inv.Status == domain.InvoicePaid {
		s.afterPaid(ctx, scope, inv)
	}
	return inv, pay, nil
}

// afterPaid berjalan SETELAH commit: kegagalan hanya dicatat, pembayaran tetap sah.
func (s *paymentService) afterPaid(ctx context.Context, scope coretenant.Scope, inv domain.Invoice) {
	s.listeners.InvoicePaid(ctx, scope, InvoiceRef{ID: inv.ID, Number: inv.InvoiceNumber, SourceType: inv.SourceType, SourceID: inv.SourceID, ContractID: inv.ContractID})
	if s.invoices == nil {
		return
	}
	if err := s.invoices.RefreshSnapshot(ctx, scope, inv.ID); err != nil {
		slog.WarnContext(ctx, "receivable: refresh paid snapshot failed", "invoice_id", inv.ID, "error", err)
	}
}

func (s *paymentService) RecordManual(ctx context.Context, scope coretenant.Scope, invoiceID string, in ManualPaymentInput, userID string) (domain.Invoice, domain.Payment, error) {
	return s.record(ctx, scope, invoiceID, repository.PaymentParams{
		Amount: strings.TrimSpace(in.Amount), Method: "manual", Reference: strings.TrimSpace(in.Reference),
		Note: strings.TrimSpace(in.Note), RecordedBy: userID, PaidAt: in.PaidAt,
	})
}

func (s *paymentService) RecordProvider(ctx context.Context, scope coretenant.Scope, invoiceID string, p repository.PaymentParams) (domain.Invoice, error) {
	if p.Method != "doku" || strings.TrimSpace(p.Reference) == "" {
		return domain.Invoice{}, ErrInvalidPayment
	}
	inv, _, err := s.record(ctx, scope, invoiceID, p)
	return inv, err
}

func (s *paymentService) List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Payment, error) {
	return s.payments.List(ctx, scope, invoiceID)
}

func (s *paymentService) ListAll(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Payment, int64, error) {
	return s.payments.ListAll(ctx, scope, limit, offset)
}
