package service

import (
	"context"
	"errors"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/publiclink"
)

// PublicLinkIdentity: identitas layanan untuk membangun scope tenant dari link publik
// (organization_id berasal dari baris link, bukan input). Nilainya sama dengan identitas
// halaman publik quotation, jadi satu WorkerResolver melayani keduanya.
const PublicLinkIdentity = "public-document-link"

var ErrLinkInvalid = publiclink.ErrLinkInvalid

type PublicScopeResolver interface {
	ResolveWorkerOrganization(ctx context.Context, organizationID, serviceIdentity string) (coretenant.Context, error)
}

type LinkResolver interface {
	Resolve(ctx context.Context, token string) (publiclink.Link, error)
	Touch(ctx context.Context, link publiclink.Link)
}

// PublicInvoices adalah subset InvoiceService yang dipakai halaman publik.
type PublicInvoices interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error)
	PDF(ctx context.Context, scope coretenant.Scope, id string) (InvoicePDF, error)
}

type PublicInvoiceState string

const (
	PublicInvoiceStateOpen PublicInvoiceState = "open" // issued | overdue
	PublicInvoiceStatePaid PublicInvoiceState = "paid"
	PublicInvoiceStateVoid PublicInvoiceState = "void"
)

type PublicInvoiceView struct {
	TenantName string
	Invoice    domain.Invoice
	State      PublicInvoiceState
	// CanPay: state open dan org punya receivable.online_payment (checkout baru ada di S6).
	CanPay bool
}

type PublicInvoiceService struct {
	links        LinkResolver
	scopes       PublicScopeResolver
	invoices     PublicInvoices
	issuers      IssuerSource
	entitlements Entitlements
	now          func() time.Time
}

func NewPublicInvoiceService(links LinkResolver, scopes PublicScopeResolver, invoices PublicInvoices, issuers IssuerSource,
	entitlements Entitlements, now func() time.Time) *PublicInvoiceService {
	if now == nil {
		now = time.Now
	}
	return &PublicInvoiceService{links: links, scopes: scopes, invoices: invoices, issuers: issuers, entitlements: entitlements, now: now}
}

type publicInvoice struct {
	scope coretenant.Scope
	link  publiclink.Link
	inv   domain.Invoice
	state PublicInvoiceState
}

// load memvalidasi token dan membangun scope tenant dari baris link. Token tidak dikenal,
// kedaluwarsa, bukan link invoice, atau invoice tidak ditemukan/draft → ErrLinkInvalid yang
// sama (tidak membocorkan keberadaan dokumen). Link dicabut karena void tetap dilayani agar
// pembaca melihat "Dibatalkan".
func (s *PublicInvoiceService) load(ctx context.Context, token string) (publicInvoice, error) {
	link, err := s.links.Resolve(ctx, token)
	if err != nil {
		return publicInvoice{}, err
	}
	if link.DocumentType != publiclink.DocumentInvoice || s.now().After(link.ExpiresAt) {
		return publicInvoice{}, ErrLinkInvalid
	}
	tenantCtx, err := s.scopes.ResolveWorkerOrganization(ctx, link.OrganizationID, PublicLinkIdentity)
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) && (appErr.Code == "WORKER_ORGANIZATION_NOT_FOUND" || appErr.Code == "WORKER_ORGANIZATION_INACTIVE") {
			return publicInvoice{}, ErrLinkInvalid
		}
		return publicInvoice{}, err
	}
	scope, err := coretenant.NewScope(tenantCtx)
	if err != nil {
		return publicInvoice{}, err
	}
	inv, err := s.invoices.Get(ctx, scope, link.DocumentID)
	if errors.Is(err, receivable.ErrInvoiceNotFound) {
		return publicInvoice{}, ErrLinkInvalid
	}
	if err != nil {
		return publicInvoice{}, err
	}
	if inv.Status == domain.InvoiceDraft {
		return publicInvoice{}, ErrLinkInvalid
	}
	if link.RevokedAt != nil && inv.Status != domain.InvoiceVoid {
		return publicInvoice{}, ErrLinkInvalid
	}
	return publicInvoice{scope: scope, link: link, inv: inv, state: stateOf(inv)}, nil
}

func stateOf(inv domain.Invoice) PublicInvoiceState {
	switch inv.Status {
	case domain.InvoiceVoid:
		return PublicInvoiceStateVoid
	case domain.InvoicePaid:
		return PublicInvoiceStatePaid
	}
	return PublicInvoiceStateOpen
}

func (s *PublicInvoiceService) View(ctx context.Context, token string) (PublicInvoiceView, error) {
	d, err := s.load(ctx, token)
	if err != nil {
		return PublicInvoiceView{}, err
	}
	s.links.Touch(ctx, d.link)
	v := PublicInvoiceView{Invoice: d.inv, State: d.state}
	if s.issuers != nil {
		if issuer, err := s.issuers.OrganizationProfile(ctx, d.scope); err == nil {
			v.TenantName = issuer.Name
		}
	}
	v.CanPay = d.state == PublicInvoiceStateOpen && s.entitlements != nil &&
		s.entitlements.RequireFeature(ctx, d.scope.OrganizationID(), "receivable.online_payment") == nil
	return v, nil
}

func (s *PublicInvoiceService) PDF(ctx context.Context, token string) (InvoicePDF, error) {
	d, err := s.load(ctx, token)
	if err != nil {
		return InvoicePDF{}, err
	}
	return s.invoices.PDF(ctx, d.scope, d.inv.ID)
}
