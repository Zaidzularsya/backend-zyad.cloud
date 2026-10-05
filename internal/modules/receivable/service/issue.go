package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/invoicepdf"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/publiclink"
)

const (
	maxInvoicePDFBytes = 10 << 20
	linkGraceDays      = 90
)

// LinkExpiry: link publik berlaku sampai jatuh tempo + 90 hari pukul 23:59:59 WIB.
func LinkExpiry(inv domain.Invoice, now time.Time) time.Time {
	base := businesstime.DayOf(now)
	switch {
	case inv.DueDate != nil:
		base = *inv.DueDate
	case inv.IssueDate != nil:
		base = *inv.IssueDate
	}
	d := base.AddDate(0, 0, linkGraceDays)
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, businesstime.Location()).UTC()
}

// dueDate = issue + termin; untuk invoice berperiode tidak boleh sebelum awal periode
// (prabayar). Pada periode yang sudah lewat (pascabayar) batas ini otomatis tidak berlaku.
func dueDate(issue time.Time, terms int, periodStart *time.Time) time.Time {
	due := issue.AddDate(0, 0, terms)
	if periodStart != nil && periodStart.After(due) {
		return *periodStart
	}
	return due
}

func (s *invoiceService) Issue(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (domain.Invoice, error) {
	inv, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Invoice{}, err
	}
	switch inv.Status {
	case domain.InvoiceDraft:
	case domain.InvoiceIssued, domain.InvoiceOverdue, domain.InvoicePaid:
		return inv, nil // idempoten: tidak menomori ulang, tidak mengirim ulang
	default:
		return domain.Invoice{}, repository.ErrInvoiceNotDraft
	}

	settings, err := s.settings.Get(ctx, scope)
	if err != nil {
		return domain.Invoice{}, err
	}
	today := businesstime.DayOf(s.now())
	seq, err := s.counters.Next(ctx, scope, "invoice", today.Year())
	if err != nil {
		return domain.Invoice{}, err
	}
	number := invoiceNumber(today.Year(), seq)
	_, err = s.invoices.Issue(ctx, scope, id, number, today, dueDate(today, settings.PaymentTermsDays, inv.PeriodStart), actorUserID)
	if errors.Is(err, repository.ErrInvoiceNotDraft) {
		// Request lain menerbitkan lebih dulu; nomor yang terbuang diterima. Pemenang yang mengirim.
		return s.Get(ctx, scope, id)
	}
	if err != nil {
		return domain.Invoice{}, err
	}

	// Setelah terbit: kegagalan di bawah ini tidak membatalkan penerbitan.
	if _, _, err := s.storeSnapshot(ctx, scope, id, actorUserID); err != nil {
		slog.WarnContext(ctx, "receivable: invoice snapshot failed", "invoice_id", id, "error", err)
	}
	final, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Invoice{}, err
	}
	if s.links != nil {
		if _, err := s.links.Ensure(ctx, scope, publiclink.DocumentInvoice, id, LinkExpiry(final, s.now()), actorUserID); err != nil {
			slog.WarnContext(ctx, "receivable: invoice link failed", "invoice_id", id, "error", err)
		}
	}
	if s.sender != nil {
		s.sender.AutoSend(ctx, scope, final, actorUserID)
	}
	return final, nil
}

func invoiceNumber(year, seq int) string { return fmt.Sprintf("INV-%d-%04d", year, seq) }

func (s *invoiceService) CreateAndIssue(ctx context.Context, scope coretenant.Scope, in InvoiceInput, actorUserID string) (domain.Invoice, error) {
	idempotent := in.SourceID != "" && in.IdempotencyKey != "" && in.SourceType != "" && in.SourceType != domain.SourceManual
	if idempotent {
		existing, err := s.invoices.FindBySource(ctx, scope, string(in.SourceType), in.SourceID, in.IdempotencyKey)
		if err == nil {
			return s.issueExisting(ctx, scope, existing, actorUserID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Invoice{}, err
		}
	}
	draft, err := s.CreateDraft(ctx, scope, in, actorUserID)
	if errors.Is(err, repository.ErrDuplicateSource) && idempotent {
		// Balapan: pemanggil lain membuat invoice sumber yang sama lebih dulu.
		existing, findErr := s.invoices.FindBySource(ctx, scope, string(in.SourceType), in.SourceID, in.IdempotencyKey)
		if findErr != nil {
			return domain.Invoice{}, findErr
		}
		return s.issueExisting(ctx, scope, existing, actorUserID)
	}
	if err != nil {
		return domain.Invoice{}, err
	}
	return s.Issue(ctx, scope, draft.ID, actorUserID)
}

// issueExisting: invoice yang sudah terbit dikembalikan apa adanya; draft sisa proses yang terputus dilanjutkan.
func (s *invoiceService) issueExisting(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, actorUserID string) (domain.Invoice, error) {
	if inv.Status == domain.InvoiceDraft {
		return s.Issue(ctx, scope, inv.ID, actorUserID)
	}
	return inv, nil
}

// ---- PDF snapshot ----

func (s *invoiceService) render(ctx context.Context, scope coretenant.Scope, inv domain.Invoice) ([]byte, string, error) {
	if s.issuers == nil || s.renderer == nil {
		return nil, "", ErrDocumentsUnavailable
	}
	issuer, err := s.issuers.OrganizationProfile(ctx, scope)
	if err != nil {
		return nil, "", err
	}
	doc := invoicepdf.Build(inv, issuer, s.now())
	content, err := s.renderer.Render(doc)
	return content, doc.Filename, err
}

// storeSnapshot merender invoice saat ini, menyimpannya sebagai objek privat, dan menautkannya.
func (s *invoiceService) storeSnapshot(ctx context.Context, scope coretenant.Scope, id, userID string) (string, []byte, error) {
	if s.files == nil {
		return "", nil, ErrDocumentsUnavailable
	}
	inv, err := s.Get(ctx, scope, id)
	if err != nil {
		return "", nil, err
	}
	content, filename, err := s.render(ctx, scope, inv)
	if err != nil {
		return "", nil, err
	}
	obj, err := s.files.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename: filename, MimeType: "application/pdf", SizeBytes: int64(len(content)),
		Class: assetdomain.ObjectClassPrivate, Label: "invoice", CreatedBy: userID,
	}, bytes.NewReader(content))
	if err != nil {
		return "", nil, err
	}
	if err := s.invoices.SetPDF(ctx, scope, id, obj.ID); err != nil {
		_ = s.files.DeleteObject(ctx, scope, obj.ID)
		return "", nil, err
	}
	return obj.ID, content, nil
}

func (s *invoiceService) RefreshSnapshot(ctx context.Context, scope coretenant.Scope, id string) error {
	inv, err := s.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if inv.InvoiceNumber == "" { // draft tidak punya snapshot
		return nil
	}
	if _, _, err := s.storeSnapshot(ctx, scope, id, ""); err != nil {
		return err
	}
	if inv.PDFAssetID != nil {
		if err := s.files.DeleteObject(ctx, scope, *inv.PDFAssetID); err != nil {
			slog.WarnContext(ctx, "receivable: delete old invoice snapshot failed", "invoice_id", id, "error", err)
		}
	}
	return nil
}

func (s *invoiceService) PDF(ctx context.Context, scope coretenant.Scope, id string) (InvoicePDF, error) {
	inv, err := s.Get(ctx, scope, id)
	if err != nil {
		return InvoicePDF{}, err
	}
	if inv.Status == domain.InvoiceDraft {
		content, filename, err := s.render(ctx, scope, inv)
		return InvoicePDF{Filename: filename, Content: content}, err
	}
	filename := inv.InvoiceNumber + ".pdf"
	if inv.PDFAssetID == nil {
		_, content, err := s.storeSnapshot(ctx, scope, id, "")
		return InvoicePDF{Filename: filename, Content: content}, err
	}
	if s.files == nil {
		return InvoicePDF{}, ErrDocumentsUnavailable
	}
	rc, _, err := s.files.OpenObject(ctx, scope, *inv.PDFAssetID)
	if err != nil {
		return InvoicePDF{}, err
	}
	defer rc.Close()
	content, err := io.ReadAll(io.LimitReader(rc, maxInvoicePDFBytes))
	return InvoicePDF{Filename: filename, Content: content}, err
}

func (s *invoiceService) Link(ctx context.Context, scope coretenant.Scope, id, userID string) (string, time.Time, error) {
	inv, err := s.Get(ctx, scope, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if inv.Status != domain.InvoiceIssued && inv.Status != domain.InvoiceOverdue && inv.Status != domain.InvoicePaid {
		return "", time.Time{}, ErrInvoiceNotSendable
	}
	if s.links == nil {
		return "", time.Time{}, ErrDocumentsUnavailable
	}
	link, err := s.links.Ensure(ctx, scope, publiclink.DocumentInvoice, inv.ID, LinkExpiry(inv, s.now()), userID)
	if err != nil {
		return "", time.Time{}, err
	}
	return strings.TrimRight(s.frontendURL, "/") + "/i/" + link.Token, link.ExpiresAt, nil
}
