package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/quotationpdf"
	"zyad.cloud/internal/modules/crm/repository"
)

var ErrQuotationDocumentsUnavailable = errors.New("quotation documents are not configured")

const maxQuotationPDFBytes = 10 << 20

func WithQuotationDocuments(d QuotationDocumentDeps) QuotationServiceOption {
	return func(s *quotationService) { s.docs = &d }
}

func (s *quotationService) buildDocument(ctx context.Context, scope coretenant.Scope, q domain.Quotation) (quotationpdf.Document, error) {
	issuer, err := s.docs.Issuers.Find(ctx, scope)
	if err != nil {
		return quotationpdf.Document{}, err
	}
	var customer quotationpdf.Customer
	if q.ContactID != nil && s.docs.Contacts != nil {
		if c, err := s.docs.Contacts.FindByID(ctx, scope, *q.ContactID); err == nil {
			customer.Name = joinName(c.FirstName, c.LastName)
			customer.Email, customer.Phone = c.Email, c.Phone
		}
	}
	if q.CompanyID != nil && s.docs.Companies != nil {
		if co, err := s.docs.Companies.FindByID(ctx, scope, *q.CompanyID); err == nil {
			customer.Company = co.Name
		}
	}
	issued := s.now()
	if q.SentAt != nil {
		issued = *q.SentAt
	}
	return quotationpdf.BuildDocument(q, issuer, customer, issued), nil
}

func joinName(first, last string) string {
	if last == "" {
		return first
	}
	return first + " " + last
}

func (s *quotationService) PDF(ctx context.Context, scope coretenant.Scope, id string) (QuotationPDF, error) {
	if s.docs == nil {
		return QuotationPDF{}, ErrQuotationDocumentsUnavailable
	}
	q, err := s.Get(ctx, scope, id)
	if err != nil {
		return QuotationPDF{}, err
	}
	if q.Status == domain.QuotationStatusDraft {
		doc, err := s.buildDocument(ctx, scope, q)
		if err != nil {
			return QuotationPDF{}, err
		}
		content, err := s.docs.Renderer.Render(doc)
		if err != nil {
			return QuotationPDF{}, err
		}
		return QuotationPDF{Filename: doc.Filename, Content: content}, nil
	}
	return s.ensureSnapshot(ctx, scope, q, "")
}

// ensureSnapshot mengembalikan PDF final tersimpan, membuatnya bila belum
// ada. Balapan dua pembuat: yang kalah menghapus upload-nya dan membaca
// milik pemenang.
func (s *quotationService) ensureSnapshot(ctx context.Context, scope coretenant.Scope, q domain.Quotation, userID string) (QuotationPDF, error) {
	filename := q.QuotationNumber + ".pdf"
	if q.PDFAssetID != nil {
		return s.readSnapshot(ctx, scope, *q.PDFAssetID, filename)
	}
	final := q
	if final.Status == domain.QuotationStatusDraft {
		final.Status = domain.QuotationStatusSent // dokumen final tanpa watermark
		now := s.now()
		final.SentAt = &now
	}
	doc, err := s.buildDocument(ctx, scope, final)
	if err != nil {
		return QuotationPDF{}, err
	}
	content, err := s.docs.Renderer.Render(doc)
	if err != nil {
		return QuotationPDF{}, err
	}
	obj, err := s.docs.Files.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename: filename, MimeType: "application/pdf", SizeBytes: int64(len(content)),
		Class: assetdomain.ObjectClassPrivate, Label: "quotation", CreatedBy: userID,
	}, bytes.NewReader(content))
	if err != nil {
		return QuotationPDF{}, err
	}
	ok, err := s.repo.SetPDFSnapshot(ctx, scope, q.ID, obj.ID)
	if err != nil {
		_ = s.docs.Files.DeleteObject(ctx, scope, obj.ID)
		return QuotationPDF{}, err
	}
	if !ok {
		_ = s.docs.Files.DeleteObject(ctx, scope, obj.ID)
		fresh, err := s.repo.FindByID(ctx, scope, q.ID)
		if err != nil || fresh.PDFAssetID == nil {
			return QuotationPDF{}, ErrQuotationDocumentsUnavailable
		}
		return s.readSnapshot(ctx, scope, *fresh.PDFAssetID, filename)
	}
	return QuotationPDF{Filename: filename, Content: content}, nil
}

func (s *quotationService) readSnapshot(ctx context.Context, scope coretenant.Scope, assetID, filename string) (QuotationPDF, error) {
	rc, _, err := s.docs.Files.OpenObject(ctx, scope, assetID)
	if err != nil {
		return QuotationPDF{}, err
	}
	defer rc.Close()
	content, err := io.ReadAll(io.LimitReader(rc, maxQuotationPDFBytes))
	if err != nil {
		return QuotationPDF{}, err
	}
	return QuotationPDF{Filename: filename, Content: content}, nil
}

func (s *quotationService) MarkSentManually(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.Quotation, error) {
	q, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Quotation{}, err
	}
	if q.Status != domain.QuotationStatusDraft {
		return domain.Quotation{}, ErrQuotationLocked
	}
	if s.docs != nil {
		if _, err := s.ensureSnapshot(ctx, scope, q, userID); err != nil {
			return domain.Quotation{}, err
		}
	}
	sent, err := s.Send(ctx, scope, id, userID)
	if err != nil {
		return domain.Quotation{}, err
	}
	s.recordDealActivity(ctx, scope, sent, domain.ActivityTypeNote, "Penawaran "+sent.QuotationNumber+" ditandai terkirim (manual)", userID)
	return sent, nil
}

// recordDealActivity: best effort; kegagalan tidak membatalkan pengiriman.
func (s *quotationService) recordDealActivity(ctx context.Context, scope coretenant.Scope, q domain.Quotation, activityType domain.ActivityType, subject, userID string) {
	if s.docs == nil || s.docs.Activities == nil || q.DealID == nil {
		return
	}
	if _, err := s.docs.Activities.Create(ctx, scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: *q.DealID, Type: activityType,
		Subject: subject, Status: domain.ActivityStatusCompleted, CreatedBy: userID, AssigneeUserID: userID,
	}); err != nil {
		slog.WarnContext(ctx, "quotation deal activity failed", "quotation_id", q.ID, "error", err)
	}
}
