package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/publiclink"
)

// linkDefaultTTL adalah masa berlaku link bila quotation tidak punya valid_until.
const linkDefaultTTL = 30 * 24 * time.Hour

// QuotationLinks adalah subset publiclink.Service yang dipakai quotation.
type QuotationLinks interface {
	Ensure(ctx context.Context, scope coretenant.Scope, docType, docID string, expiresAt time.Time, createdBy string) (publiclink.Link, error)
	Revoke(ctx context.Context, scope coretenant.Scope, docType, docID string) error
}

type QuotationLinkDeps struct {
	Links       QuotationLinks
	FrontendURL string
	Responses   repository.QuotationResponseRepository
}

func WithQuotationLinks(d QuotationLinkDeps) QuotationServiceOption {
	return func(s *quotationService) { s.links = &d }
}

// QuotationLink adalah URL publik yang dibagikan ke customer.
type QuotationLink struct {
	URL       string
	ExpiresAt time.Time
}

// LinkExpiry: valid_until pukul 23:59:59 WIB, atau now + 30 hari bila kosong.
func LinkExpiry(q domain.Quotation, now time.Time) time.Time {
	if q.ValidUntil == nil {
		return now.Add(linkDefaultTTL)
	}
	v := q.ValidUntil
	return time.Date(v.Year(), v.Month(), v.Day(), 23, 59, 59, 0, businesstime.Location()).UTC()
}

func (s *quotationService) linkURL(token string) string {
	return strings.TrimRight(s.links.FrontendURL, "/") + "/q/" + token
}

// publicLinkURL mengembalikan "" bila link tidak tersedia; pengiriman tetap
// jalan tanpa link (customer masih menerima ringkasan/PDF).
func (s *quotationService) publicLinkURL(ctx context.Context, scope coretenant.Scope, q domain.Quotation, userID string) string {
	if s.links == nil || s.links.Links == nil {
		return ""
	}
	link, err := s.links.Links.Ensure(ctx, scope, publiclink.DocumentQuotation, q.ID, LinkExpiry(q, s.now()), userID)
	if err != nil {
		slog.Warn("quotation public link unavailable", "quotation_id", q.ID, "error", err)
		return ""
	}
	return s.linkURL(link.Token)
}

func (s *quotationService) Link(ctx context.Context, scope coretenant.Scope, id, userID string) (QuotationLink, error) {
	if s.links == nil || s.links.Links == nil {
		return QuotationLink{}, ErrQuotationDocumentsUnavailable
	}
	q, err := s.Get(ctx, scope, id)
	if err != nil {
		return QuotationLink{}, err
	}
	if q.Status != domain.QuotationStatusSent {
		return QuotationLink{}, ErrQuotationNotSendable
	}
	link, err := s.links.Links.Ensure(ctx, scope, publiclink.DocumentQuotation, q.ID, LinkExpiry(q, s.now()), userID)
	if err != nil {
		return QuotationLink{}, err
	}
	return QuotationLink{URL: s.linkURL(link.Token), ExpiresAt: link.ExpiresAt}, nil
}

func (s *quotationService) Responses(ctx context.Context, scope coretenant.Scope, id string) ([]domain.QuotationResponse, error) {
	if _, err := s.Get(ctx, scope, id); err != nil {
		return nil, err
	}
	if s.links == nil || s.links.Responses == nil {
		return []domain.QuotationResponse{}, nil
	}
	return s.links.Responses.ListByQuotation(ctx, scope, id)
}

// revokeLink mencabut link publik versi lama setelah revisi; kegagalan hanya
// dicatat karena revisi sudah tersimpan.
func (s *quotationService) revokeLink(ctx context.Context, scope coretenant.Scope, id string) {
	if s.links == nil || s.links.Links == nil {
		return
	}
	if err := s.links.Links.Revoke(ctx, scope, publiclink.DocumentQuotation, id); err != nil {
		slog.Warn("quotation public link revoke failed", "quotation_id", id, "error", err)
	}
}
