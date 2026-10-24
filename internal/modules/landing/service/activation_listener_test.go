package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var listenerNow = time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)

// settlerFake mirrors repository.SettleByInvoice using the same domain rules,
// keyed by organization like the SQL (organization_id = scope).
type settlerFake struct {
	acts  map[string]*domain.LandingPageActivation // by invoice id
	pages map[string]*domain.LandingPage
	err   error
}

func (f *settlerFake) SettleByInvoice(_ context.Context, scope coretenant.Scope, invoiceID string, paidAt time.Time) (repository.ActivationSettlement, error) {
	if f.err != nil {
		return repository.ActivationSettlement{}, f.err
	}
	a, ok := f.acts[invoiceID]
	if !ok || a.OrganizationID != scope.OrganizationID() {
		return repository.ActivationSettlement{}, nil
	}
	out := repository.ActivationSettlement{Found: true, Activation: *a}
	if a.Status == domain.ActivationStatusPaid {
		return out, nil
	}
	p := f.pages[a.PageID]
	if p != nil && p.DeletedAt == nil {
		out.PageFound, out.PageSlug, out.PageTitle, out.IsHomepage = true, p.Slug, p.Title, p.IsHomepage
	}
	a.Status, a.PaidAt = domain.ActivationStatusPaid, &paidAt
	out.Applied, out.Activation = true, *a
	if out.PageFound {
		if s := domain.SettleActivationPayment(*p, *a, paidAt); s.Publish {
			p.Status, p.PublishedAt, p.ActiveUntil = domain.PageStatusPublished, &s.PublishedAt, &s.ActiveUntil
			out.Published, out.ActiveUntil = true, &s.ActiveUntil
		}
	}
	return out, nil
}

type noticeFake struct {
	notices []ActivationPaymentNotice
	err     error
}

func (n *noticeFake) PaymentReceived(_ context.Context, _ coretenant.Scope, in ActivationPaymentNotice) error {
	n.notices = append(n.notices, in)
	return n.err
}

type publicLinkFake struct{ url string }

func (l publicLinkFake) PublicURL(_ context.Context, _ coretenant.Scope, slug string, home bool) (string, error) {
	if l.url == "" {
		return "", nil
	}
	if home {
		return l.url + "/", nil
	}
	return l.url + "/" + slug, nil
}

type listenerHarness struct {
	settler  *settlerFake
	notifier *noticeFake
	l        *ActivationPaidListener
}

func newListenerHarness() *listenerHarness {
	email := "budi@example.test"
	h := &listenerHarness{
		settler: &settlerFake{
			acts: map[string]*domain.LandingPageActivation{
				"inv-1": {ID: "act-1", OrganizationID: landingServiceOrganizationID, PageID: "page-a", Status: domain.ActivationStatusOpen,
					DurationValue: 7, DurationUnit: domain.ActivationDurationDay, BuyerName: "Budi", BuyerContact: "0812", BuyerEmail: &email,
					InvoiceDueAt: listenerNow.Add(-48 * time.Hour)},
			},
			pages: map[string]*domain.LandingPage{
				"page-a": {ID: "page-a", OrganizationID: landingServiceOrganizationID, Status: domain.PageStatusDraft, Slug: "toko-budi", Title: "Toko Budi"},
			},
		},
		notifier: &noticeFake{},
	}
	h.l = NewActivationPaidListener(h.settler, h.notifier, publicLinkFake{url: "https://klien.test"}, func() time.Time { return listenerNow })
	return h
}

func (h *listenerHarness) pay(t *testing.T, invoiceID string) {
	t.Helper()
	h.l.InvoicePaid(context.Background(), mustLandingScope(t), invoiceID)
}

func TestInvoicePaid_PublishesPage(t *testing.T) {
	cases := []struct {
		name  string
		value int
		unit  domain.ActivationDurationUnit
		want  time.Time
	}{
		{"7 day", 7, domain.ActivationDurationDay, listenerNow.AddDate(0, 0, 7)},
		{"1 month", 1, domain.ActivationDurationMonth, listenerNow.AddDate(0, 1, 0)},
		{"2 week", 2, domain.ActivationDurationWeek, listenerNow.AddDate(0, 0, 14)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newListenerHarness()
			h.settler.acts["inv-1"].DurationValue, h.settler.acts["inv-1"].DurationUnit = tc.value, tc.unit
			h.pay(t, "inv-1")

			page := h.settler.pages["page-a"]
			if page.Status != domain.PageStatusPublished {
				t.Fatalf("page status = %q, want published", page.Status)
			}
			if page.PublishedAt == nil || !page.PublishedAt.Equal(listenerNow) {
				t.Fatalf("published_at = %v, want %v", page.PublishedAt, listenerNow)
			}
			if page.ActiveUntil == nil || !page.ActiveUntil.Equal(tc.want) {
				t.Fatalf("active_until = %v, want %v", page.ActiveUntil, tc.want)
			}
			act := h.settler.acts["inv-1"]
			if act.Status != domain.ActivationStatusPaid || act.PaidAt == nil || !act.PaidAt.Equal(listenerNow) {
				t.Fatalf("activation = %+v, want paid at %v", act, listenerNow)
			}
			if len(h.notifier.notices) != 1 {
				t.Fatalf("notices = %d, want 1", len(h.notifier.notices))
			}
			n := h.notifier.notices[0]
			if n.BuyerEmail != "budi@example.test" || n.PageURL != "https://klien.test/toko-budi" || !n.ActiveUntil.Equal(tc.want) {
				t.Fatalf("notice = %+v", n)
			}
		})
	}
}

func TestInvoicePaid_Twice_ExtendsOnce(t *testing.T) {
	h := newListenerHarness()
	h.pay(t, "inv-1")
	first := *h.settler.pages["page-a"].ActiveUntil
	h.l.now = func() time.Time { return listenerNow.Add(3 * 24 * time.Hour) }
	h.pay(t, "inv-1")

	if !h.settler.pages["page-a"].ActiveUntil.Equal(first) {
		t.Fatalf("active_until changed on repeat: %v -> %v", first, h.settler.pages["page-a"].ActiveUntil)
	}
	if !h.settler.acts["inv-1"].PaidAt.Equal(listenerNow) {
		t.Fatalf("paid_at moved on repeat: %v", h.settler.acts["inv-1"].PaidAt)
	}
	if len(h.notifier.notices) != 1 {
		t.Fatalf("notices = %d, want exactly 1", len(h.notifier.notices))
	}
}

func TestInvoicePaid_AfterDueDate_StillActivates(t *testing.T) {
	h := newListenerHarness() // invoice_due_at is 48h before listenerNow
	h.pay(t, "inv-1")
	if h.settler.pages["page-a"].Status != domain.PageStatusPublished {
		t.Fatal("overdue open activation was not activated")
	}
	want := listenerNow.AddDate(0, 0, 7)
	if !h.settler.pages["page-a"].ActiveUntil.Equal(want) {
		t.Fatalf("active_until = %v, want from payment time %v", h.settler.pages["page-a"].ActiveUntil, want)
	}
}

func TestInvoicePaid_OnVoidActivation_Activates(t *testing.T) {
	h := newListenerHarness()
	h.settler.acts["inv-1"].Status = domain.ActivationStatusVoid
	h.pay(t, "inv-1")
	if h.settler.acts["inv-1"].Status != domain.ActivationStatusPaid || h.settler.pages["page-a"].Status != domain.PageStatusPublished {
		t.Fatalf("void activation not activated: %+v / %+v", h.settler.acts["inv-1"], h.settler.pages["page-a"])
	}
	if len(h.notifier.notices) != 1 {
		t.Fatalf("notices = %d, want 1", len(h.notifier.notices))
	}
}

func TestInvoicePaid_DoublePayment_NeverShortens(t *testing.T) {
	h := newListenerHarness()
	h.settler.acts["inv-1"].DurationValue, h.settler.acts["inv-1"].DurationUnit = 1, domain.ActivationDurationMonth
	h.pay(t, "inv-1")
	long := *h.settler.pages["page-a"].ActiveUntil

	h.settler.acts["inv-2"] = &domain.LandingPageActivation{ID: "act-2", OrganizationID: landingServiceOrganizationID, PageID: "page-a",
		Status: domain.ActivationStatusOpen, DurationValue: 7, DurationUnit: domain.ActivationDurationDay}
	h.l.now = func() time.Time { return listenerNow.Add(24 * time.Hour) }
	h.pay(t, "inv-2")

	if !h.settler.pages["page-a"].ActiveUntil.Equal(long) {
		t.Fatalf("active_until = %v, want unchanged %v", h.settler.pages["page-a"].ActiveUntil, long)
	}
	if h.settler.acts["inv-2"].Status != domain.ActivationStatusPaid {
		t.Fatal("second activation must still be marked paid")
	}
	if !h.settler.pages["page-a"].PublishedAt.Equal(listenerNow) {
		t.Fatalf("published_at moved for an already published page: %v", h.settler.pages["page-a"].PublishedAt)
	}
}

func TestInvoicePaid_OtherOrgScope_NoEffect(t *testing.T) {
	h := newListenerHarness()
	h.settler.acts["inv-1"].OrganizationID = "99999999-9999-9999-9999-999999999999"
	h.pay(t, "inv-1")
	h.pay(t, "inv-unknown")
	if h.settler.acts["inv-1"].Status != domain.ActivationStatusOpen || h.settler.pages["page-a"].Status != domain.PageStatusDraft {
		t.Fatal("foreign-org activation was touched")
	}
	if len(h.notifier.notices) != 0 {
		t.Fatalf("notices = %d, want 0", len(h.notifier.notices))
	}
}

func TestInvoicePaid_NotificationFailureStillPublishes(t *testing.T) {
	h := newListenerHarness()
	h.notifier.err = errors.New("smtp down")
	h.pay(t, "inv-1")
	if h.settler.pages["page-a"].Status != domain.PageStatusPublished || h.settler.acts["inv-1"].Status != domain.ActivationStatusPaid {
		t.Fatal("notification failure must not undo the publish")
	}
}

func TestInvoicePaid_SettlerErrorIsSwallowed(t *testing.T) {
	h := newListenerHarness()
	h.settler.err = errors.New("db down")
	h.pay(t, "inv-1") // must not panic
	if len(h.notifier.notices) != 0 {
		t.Fatal("no notice expected when settlement failed")
	}
}

func TestRenewal_FromExpired_RecomputesFromPayment(t *testing.T) {
	h := newListenerHarness()
	past := listenerNow.Add(-10 * 24 * time.Hour)
	pub := listenerNow.Add(-40 * 24 * time.Hour)
	p := h.settler.pages["page-a"]
	p.Status, p.ActiveUntil, p.PublishedAt = domain.PageStatusPublished, &past, &pub
	h.pay(t, "inv-1")
	want := listenerNow.AddDate(0, 0, 7)
	if p.Status != domain.PageStatusPublished || !p.ActiveUntil.Equal(want) {
		t.Fatalf("page = %s until %v, want published until %v", p.Status, p.ActiveUntil, want)
	}
}

func TestInvoicePaid_ArchivedPage_StaysArchived(t *testing.T) {
	h := newListenerHarness()
	h.settler.pages["page-a"].Status = domain.PageStatusArchived
	h.pay(t, "inv-1")
	if h.settler.pages["page-a"].Status != domain.PageStatusArchived || h.settler.pages["page-a"].ActiveUntil != nil {
		t.Fatalf("archived page changed: %+v", h.settler.pages["page-a"])
	}
	if h.settler.acts["inv-1"].Status != domain.ActivationStatusPaid {
		t.Fatal("activation must still be marked paid")
	}
	if len(h.notifier.notices) != 0 {
		t.Fatal("no 'page live' notice for a page that was not published")
	}
}

func TestInvoicePaid_NoHostSendsWithoutLink(t *testing.T) {
	h := newListenerHarness()
	h.l.links = publicLinkFake{}
	h.pay(t, "inv-1")
	if len(h.notifier.notices) != 1 || h.notifier.notices[0].PageURL != "" {
		t.Fatalf("notices = %+v", h.notifier.notices)
	}
}

func TestInvoicePaid_NoEmailNoNotice(t *testing.T) {
	h := newListenerHarness()
	h.settler.acts["inv-1"].BuyerEmail = nil
	h.pay(t, "inv-1")
	if len(h.notifier.notices) != 0 || h.settler.pages["page-a"].Status != domain.PageStatusPublished {
		t.Fatalf("notices = %d, page = %s", len(h.notifier.notices), h.settler.pages["page-a"].Status)
	}
}

func TestInvoicePaid_NoticeCarriesNoInternalData(t *testing.T) {
	h := newListenerHarness()
	h.pay(t, "inv-1")
	n := h.notifier.notices[0]
	for _, s := range []string{n.BuyerName, n.PageTitle, n.PageURL} {
		if strings.Contains(s, "act-1") || strings.Contains(s, "page-a") || strings.Contains(s, "inv-1") {
			t.Fatalf("internal id leaked into %q", s)
		}
	}
}
