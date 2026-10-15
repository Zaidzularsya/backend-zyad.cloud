package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/publiclink"
)

type publicInvoiceStub struct{ inv domain.Invoice }

func (s *publicInvoiceStub) Get(_ context.Context, _ coretenant.Scope, id string) (domain.Invoice, error) {
	if s.inv.ID != id {
		return domain.Invoice{}, receivable.ErrInvoiceNotFound
	}
	return s.inv, nil
}
func (s *publicInvoiceStub) PDF(context.Context, coretenant.Scope, string) (InvoicePDF, error) {
	return InvoicePDF{Filename: s.inv.InvoiceNumber + ".pdf", Content: []byte("%PDF")}, nil
}

type publicLinks struct {
	link    publiclink.Link
	touched int
}

func (l *publicLinks) Resolve(_ context.Context, token string) (publiclink.Link, error) {
	if token != "tok1" {
		return publiclink.Link{}, publiclink.ErrLinkInvalid
	}
	return l.link, nil
}
func (l *publicLinks) Touch(context.Context, publiclink.Link) { l.touched++ }

type publicScopes struct{ err error }

func (p publicScopes) ResolveWorkerOrganization(context.Context, string, string) (coretenant.Context, error) {
	if p.err != nil {
		return coretenant.Context{}, p.err
	}
	return coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "a",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceWorker, DataPlacement: coretenant.DataPlacementShared,
	})
}

type publicHarness struct {
	svc   *PublicInvoiceService
	inv   *publicInvoiceStub
	links *publicLinks
}

func newPublicHarness(status domain.InvoiceStatus, online bool) *publicHarness {
	h := &publicHarness{
		inv: &publicInvoiceStub{inv: domain.Invoice{ID: "inv-1", InvoiceNumber: "INV-2026-0001", Status: status, GrandTotal: "333000.00", AmountPaid: "0.00", Currency: "IDR"}},
		links: &publicLinks{link: publiclink.Link{ID: "l1", OrganizationID: "11111111-1111-1111-1111-111111111111",
			DocumentType: publiclink.DocumentInvoice, DocumentID: "inv-1", ExpiresAt: nowWIB.Add(30 * 24 * time.Hour)}},
	}
	h.svc = NewPublicInvoiceService(h.links, publicScopes{}, h.inv, fakeIssuer{}, fakeEntitlements{online: online}, func() time.Time { return nowWIB })
	return h
}

func TestPublicViewOpenInvoice(t *testing.T) {
	h := newPublicHarness(domain.InvoiceIssued, false)
	v, err := h.svc.View(ctx, "tok1")
	if err != nil || v.State != PublicInvoiceStateOpen || v.CanPay || v.TenantName != "PT Zyad Technovation" || v.Invoice.InvoiceNumber != "INV-2026-0001" {
		t.Fatalf("view = %+v err=%v", v, err)
	}
	if h.links.touched != 1 {
		t.Fatalf("touched = %d", h.links.touched)
	}
	h.inv.inv.Status = domain.InvoiceOverdue
	if v, _ := h.svc.View(ctx, "tok1"); v.State != PublicInvoiceStateOpen {
		t.Fatalf("overdue is still open, got %s", v.State)
	}
}

func TestPublicCanPayOnlyWhenOpenAndEntitled(t *testing.T) {
	open := newPublicHarness(domain.InvoiceIssued, true)
	if v, _ := open.svc.View(ctx, "tok1"); !v.CanPay {
		t.Fatal("open + online_payment must allow paying")
	}
	for _, st := range []domain.InvoiceStatus{domain.InvoicePaid, domain.InvoiceVoid} {
		h := newPublicHarness(st, true)
		if v, _ := h.svc.View(ctx, "tok1"); v.CanPay {
			t.Fatalf("%s must never be payable", st)
		}
	}
}

func TestPublicPaidAndVoidStates(t *testing.T) {
	paid := newPublicHarness(domain.InvoicePaid, false)
	if v, err := paid.svc.View(ctx, "tok1"); err != nil || v.State != PublicInvoiceStatePaid {
		t.Fatalf("paid view = %+v err=%v", v, err)
	}
	void := newPublicHarness(domain.InvoiceVoid, true)
	revoked := nowWIB
	void.links.link.RevokedAt = &revoked // Void mencabut link, tetapi pembaca harus melihat "Dibatalkan", bukan galat
	if v, err := void.svc.View(ctx, "tok1"); err != nil || v.State != PublicInvoiceStateVoid {
		t.Fatalf("void view = %+v err=%v", v, err)
	}
}

func TestPublicInvalidLinksAreIndistinguishable(t *testing.T) {
	cases := map[string]func(h *publicHarness) string{
		"unknown token":    func(h *publicHarness) string { return "nope" },
		"expired":          func(h *publicHarness) string { h.links.link.ExpiresAt = nowWIB.Add(-time.Second); return "tok1" },
		"wrong document":   func(h *publicHarness) string { h.links.link.DocumentType = publiclink.DocumentQuotation; return "tok1" },
		"draft invoice":    func(h *publicHarness) string { h.inv.inv.Status = domain.InvoiceDraft; return "tok1" },
		"invoice gone":     func(h *publicHarness) string { h.links.link.DocumentID = "other"; return "tok1" },
		"revoked not void": func(h *publicHarness) string { r := nowWIB; h.links.link.RevokedAt = &r; return "tok1" },
	}
	for name, mutate := range cases {
		h := newPublicHarness(domain.InvoiceIssued, false)
		token := mutate(h)
		if _, err := h.svc.View(ctx, token); !errors.Is(err, ErrLinkInvalid) {
			t.Errorf("%s: View err = %v", name, err)
		}
		if _, err := h.svc.PDF(ctx, token); !errors.Is(err, ErrLinkInvalid) {
			t.Errorf("%s: PDF err = %v", name, err)
		}
	}
	h := newPublicHarness(domain.InvoiceIssued, false)
	h.svc.scopes = publicScopes{err: coreerrors.New("WORKER_ORGANIZATION_INACTIVE", "inactive", 403)}
	if _, err := h.svc.View(ctx, "tok1"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("inactive org err = %v", err)
	}
	h.svc.scopes = publicScopes{err: errors.New("db down")}
	if _, err := h.svc.View(ctx, "tok1"); err == nil || errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("infrastructure error must not look like an invalid link: %v", err)
	}
}

func TestPublicPDF(t *testing.T) {
	h := newPublicHarness(domain.InvoiceIssued, false)
	pdf, err := h.svc.PDF(ctx, "tok1")
	if err != nil || pdf.Filename != "INV-2026-0001.pdf" || string(pdf.Content) != "%PDF" {
		t.Fatalf("pdf = %+v err=%v", pdf, err)
	}
}
