package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/publiclink"
)

type publicQuoteStub struct {
	QuotationService
	q domain.Quotation
}

func (s *publicQuoteStub) Get(context.Context, coretenant.Scope, string) (domain.Quotation, error) {
	return s.q, nil
}

func (s *publicQuoteStub) PDF(context.Context, coretenant.Scope, string) (QuotationPDF, error) {
	return QuotationPDF{Filename: s.q.QuotationNumber + ".pdf", Content: []byte("%PDF")}, nil
}

type publicResponses struct {
	repository.QuotationResponseRepository
	stub  *publicQuoteStub
	items []domain.QuotationResponse
}

func (r *publicResponses) Respond(_ context.Context, _ coretenant.Scope, p repository.RecordResponseParams) (domain.Quotation, domain.QuotationResponse, error) {
	if r.stub.q.Status != domain.QuotationStatusSent {
		return domain.Quotation{}, domain.QuotationResponse{}, repository.ErrQuotationNotRespondable
	}
	if p.Action == domain.QuotationResponseApproved {
		r.stub.q.Status = domain.QuotationStatusApproved
	} else {
		r.stub.q.Status = domain.QuotationStatusRevisionRequested
	}
	resp := domain.QuotationResponse{ID: "r1", QuotationID: p.QuotationID, Action: p.Action, Categories: p.Categories,
		Note: p.Note, ResponderName: p.ResponderName, CreatedAt: time.Now()}
	r.items = append([]domain.QuotationResponse{resp}, r.items...)
	return r.stub.q, resp, nil
}

func (r *publicResponses) ListByQuotation(context.Context, coretenant.Scope, string) ([]domain.QuotationResponse, error) {
	return r.items, nil
}

type publicLinkStore struct {
	link    publiclink.Link
	touched int
}

func (l *publicLinkStore) Resolve(_ context.Context, token string) (publiclink.Link, error) {
	if token != "tok1" {
		return publiclink.Link{}, publiclink.ErrLinkInvalid
	}
	return l.link, nil
}
func (l *publicLinkStore) Touch(context.Context, publiclink.Link) { l.touched++ }

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

type fakeNotifier struct{ calls int }

func (n *fakeNotifier) Notify(context.Context, coretenant.Scope, domain.Quotation, domain.QuotationResponse) error {
	n.calls++
	return nil
}

type fakeApprovedHook struct{ calls int }

func (h *fakeApprovedHook) QuotationApproved(context.Context, coretenant.Scope, domain.Quotation, string) error {
	h.calls++
	return nil
}

type publicHarness struct {
	svc        *PublicQuotationService
	stub       *publicQuoteStub
	link       publiclink.Link
	links      *publicLinkStore
	scopes     *publicScopes
	hook       *fakeApprovedHook
	notifier   *fakeNotifier
	activities *fakeActivities
	now        time.Time
}

func newPublicHarness(t *testing.T) *publicHarness {
	t.Helper()
	h := &publicHarness{now: time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)}
	deal := "d1"
	h.stub = &publicQuoteStub{q: domain.Quotation{ID: "q1", QuotationNumber: "QUO-2026-0001", Status: domain.QuotationStatusSent, DealID: &deal}}
	h.link = publiclink.Link{ID: "l1", OrganizationID: "11111111-1111-1111-1111-111111111111", DocumentType: publiclink.DocumentQuotation,
		DocumentID: "q1", ExpiresAt: h.now.Add(10 * 24 * time.Hour)}
	h.links = &publicLinkStore{link: h.link}
	h.hook, h.notifier, h.activities = &fakeApprovedHook{}, &fakeNotifier{}, &fakeActivities{}
	h.svc = NewPublicQuotationService(h.links, publicScopes{}, h.stub, &publicResponses{stub: h.stub}, fakeIssuer{}, h.activities,
		h.notifier, h.hook, func() time.Time { return h.now })
	return h
}

func (h *publicHarness) supersede() {
	h.stub.q.Status = domain.QuotationStatusSuperseded
	revoked := h.now
	h.link.RevokedAt = &revoked
	h.links.link = h.link
}

func TestPublicApproveFlow(t *testing.T) {
	ctx := context.Background()
	h := newPublicHarness(t)
	view, err := h.svc.View(ctx, "tok1")
	if err != nil || view.State != PublicQuotationStateActive || view.TenantName != "PT Zyad" || h.links.touched != 1 {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	view, err = h.svc.Approve(ctx, "tok1", PublicResponseInput{ResponderName: "Budi Santoso", IP: "203.0.113.5"})
	if err != nil || view.State != PublicQuotationStateDecided || view.Status != domain.QuotationStatusApproved {
		t.Fatalf("approve=%+v err=%v", view, err)
	}
	if h.hook.calls != 1 || h.notifier.calls != 1 || len(h.activities.created) != 1 || h.activities.created[0].Type != domain.ActivityTypeQuotationResponse {
		t.Fatalf("hook=%d notify=%d acts=%+v", h.hook.calls, h.notifier.calls, h.activities.created)
	}
	act := h.activities.created[0]
	if act.RelatedEntityID != "d1" || act.CreatedBy != "" || act.Metadata["action"] != "approved" || !strings.Contains(act.Subject, "QUO-2026-0001") {
		t.Fatalf("activity=%+v", act)
	}
	if _, err := h.svc.RequestRevision(ctx, "tok1", PublicResponseInput{ResponderName: "Budi", Categories: []domain.RevisionCategory{domain.RevisionPrice}}); !errors.Is(err, ErrQuotationNotRespondable) {
		t.Fatalf("second response err=%v", err)
	}
	if len(h.activities.created) != 1 {
		t.Fatal("a rejected second response must not log another activity")
	}
}

func TestPublicRevisionStoresNoteVerbatimAndLogsActivity(t *testing.T) {
	h := newPublicHarness(t)
	view, err := h.svc.RequestRevision(context.Background(), "tok1", PublicResponseInput{
		ResponderName: "Budi", Categories: []domain.RevisionCategory{domain.RevisionPrice, domain.RevisionOther}, Note: "<script>x</script>",
	})
	if err != nil || view.Status != domain.QuotationStatusRevisionRequested || view.LastResponse == nil || view.LastResponse.Note != "<script>x</script>" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	act := h.activities.created[0]
	if act.Metadata["note"] != "<script>x</script>" || h.hook.calls != 0 || h.notifier.calls != 1 {
		t.Fatalf("activity=%+v hook=%d", act.Metadata, h.hook.calls)
	}
}

func TestPublicRevisionValidationAndStates(t *testing.T) {
	ctx := context.Background()
	h := newPublicHarness(t)
	for _, in := range []PublicResponseInput{
		{ResponderName: "B", Categories: []domain.RevisionCategory{domain.RevisionPrice}},
		{ResponderName: "Budi"},
		{ResponderName: "Budi", Categories: []domain.RevisionCategory{"color"}},
		{ResponderName: "Budi", Categories: []domain.RevisionCategory{domain.RevisionOther}},
		{ResponderName: "Budi", Categories: []domain.RevisionCategory{domain.RevisionPrice}, Note: strings.Repeat("x", 2001)},
	} {
		if _, err := h.svc.RequestRevision(ctx, "tok1", in); !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("%+v err=%v", in, err)
		}
	}
	h.now = h.link.ExpiresAt.Add(time.Hour)
	if v, _ := h.svc.View(ctx, "tok1"); v.State != PublicQuotationStateExpired {
		t.Fatalf("expired state=%v", v.State)
	}
	if _, err := h.svc.Approve(ctx, "tok1", PublicResponseInput{ResponderName: "Budi"}); !errors.Is(err, ErrQuotationNotRespondable) {
		t.Fatalf("approve after expiry err=%v", err)
	}
	h.now = h.link.ExpiresAt.Add(31 * 24 * time.Hour)
	if _, err := h.svc.View(ctx, "tok1"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("long-expired link must be invalid")
	}
	h.supersede()
	h.now = h.link.ExpiresAt.Add(-time.Hour)
	if v, err := h.svc.View(ctx, "tok1"); err != nil || v.State != PublicQuotationStateSuperseded {
		t.Fatalf("superseded view=%+v err=%v", v, err)
	}
	if _, err := h.svc.PDF(ctx, "tok1"); err != nil {
		t.Fatalf("old version PDF must stay viewable: %v", err)
	}
	if _, err := h.svc.View(ctx, "unknown"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("unknown token")
	}
}

func TestPublicRevokedNonSupersededAndInactiveOrgAreInvalid(t *testing.T) {
	ctx := context.Background()
	h := newPublicHarness(t)
	revoked := h.now
	h.links.link.RevokedAt = &revoked
	if _, err := h.svc.View(ctx, "tok1"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("revoked (not superseded) err=%v", err)
	}
	h.links.link.RevokedAt = nil
	h.svc.scopes = publicScopes{err: coreerrors.New("WORKER_ORGANIZATION_INACTIVE", "inactive", 409)}
	if _, err := h.svc.View(ctx, "tok1"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("inactive org err=%v", err)
	}
	h.svc.scopes = publicScopes{err: coreerrors.New("WORKER_TENANT_RESOLUTION_FAILED", "db down", 500)}
	if _, err := h.svc.View(ctx, "tok1"); err == nil || errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("infrastructure failure must not look like an invalid link: %v", err)
	}
}

func TestInternalApproveCallsHook(t *testing.T) {
	svc, _, _, _, _ := docFixture()
	hook := &fakeApprovedHook{}
	WithQuotationApprovedHook(hook)(svc)
	if _, err := svc.Approve(context.Background(), coretenant.Scope{}, "q1", "u1"); err != nil {
		t.Fatal(err)
	}
	if hook.calls != 1 {
		t.Fatalf("hook calls=%d", hook.calls)
	}
}
