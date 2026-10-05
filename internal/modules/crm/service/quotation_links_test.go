package service

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/publiclink"
)

type fakeLinks struct {
	revoked map[string]int
}

func (f *fakeLinks) Ensure(_ context.Context, _ coretenant.Scope, _, docID string, exp time.Time, _ string) (publiclink.Link, error) {
	return publiclink.Link{ID: "l-" + docID, DocumentID: docID, Token: "tok-" + docID, ExpiresAt: exp}, nil
}

func (f *fakeLinks) Revoke(_ context.Context, _ coretenant.Scope, _, docID string) error {
	if f.revoked == nil {
		f.revoked = map[string]int{}
	}
	f.revoked[docID]++
	return nil
}

func TestSendViaEmbedsPublicLinkAndRevisionRevokesIt(t *testing.T) {
	svc, repo, _, email, _ := sendFixture("budi@example.com")
	links := &fakeLinks{}
	WithQuotationLinks(QuotationLinkDeps{Links: links, FrontendURL: "https://app.test"})(svc)
	if _, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeText, ClientRequestID: "r1", UserID: "u1",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(email.calls[0].BodyHTML, "https://app.test/q/tok-q1") {
		t.Fatalf("body=%s", email.calls[0].BodyHTML)
	}
	q := repo.stored["q1"]
	q.Items = []domain.QuotationItem{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", TaxPercent: "0.00", LineTotal: "100.00"}}
	repo.stored["q1"] = q
	if _, err := svc.Revise(context.Background(), coretenant.Scope{}, "q1", "u1"); err != nil {
		t.Fatal(err)
	}
	if links.revoked["q1"] != 1 {
		t.Fatal("revise must revoke the old link")
	}
}

func TestSendViaPDFOnlyEmailStillCarriesLink(t *testing.T) {
	svc, _, _, email, _ := sendFixture("budi@example.com")
	WithQuotationLinks(QuotationLinkDeps{Links: &fakeLinks{}, FrontendURL: "https://app.test/"})(svc)
	if _, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModePDF, ClientRequestID: "r1", UserID: "u1",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(email.calls[0].BodyHTML, `href="https://app.test/q/tok-q1"`) {
		t.Fatalf("body=%s", email.calls[0].BodyHTML)
	}
}

func TestLinkRequiresSentStatus(t *testing.T) {
	svc, repo, _, _, _ := sendFixture("budi@example.com")
	WithQuotationLinks(QuotationLinkDeps{Links: &fakeLinks{}, FrontendURL: "https://app.test"})(svc)
	if _, err := svc.Link(context.Background(), coretenant.Scope{}, "q1", "u1"); err != ErrQuotationNotSendable {
		t.Fatalf("draft err=%v", err)
	}
	q := repo.stored["q1"]
	q.Status = domain.QuotationStatusSent
	repo.stored["q1"] = q
	l, err := svc.Link(context.Background(), coretenant.Scope{}, "q1", "u1")
	if err != nil || l.URL != "https://app.test/q/tok-q1" {
		t.Fatalf("link=%+v err=%v", l, err)
	}
}

func TestLinkExpiry(t *testing.T) {
	now := time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)
	vu := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	if got := LinkExpiry(domain.Quotation{ValidUntil: &vu}, now); !got.Equal(time.Date(2026, 10, 31, 16, 59, 59, 0, time.UTC)) {
		t.Fatalf("valid_until expiry = %v", got)
	}
	if got := LinkExpiry(domain.Quotation{}, now); !got.Equal(now.Add(30 * 24 * time.Hour)) {
		t.Fatalf("default expiry = %v", got)
	}
}
