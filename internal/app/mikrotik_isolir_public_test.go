package app

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	mailboxdomain "zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/organization"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	wadomain "zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/shared/docpdf"
)

type recFeatures struct {
	keys []string
	err  error
}

func (f *recFeatures) RequireFeature(_ context.Context, _ string, key string) error {
	f.keys = append(f.keys, key)
	return f.err
}

type failLinker struct{ err error }

func (f failLinker) Link(context.Context, coretenant.Scope, string, string) (string, time.Time, error) {
	return "", time.Time{}, f.err
}

type profileIssuer struct{}

func (profileIssuer) OrganizationProfile(context.Context, coretenant.Scope) (docpdf.Party, error) {
	return docpdf.Party{Name: "ISP Jaya", Phone: "0812", Email: "a@isp.id"}, nil
}

func TestPayLink_NoDefaultSenderReturnsEmpty(t *testing.T) {
	pl := &fakePayLinker{}
	a := mikrotikPublicAdapter{settings: fakeNotifSettings{sender: "  "}, links: pl, features: &recFeatures{}}
	link, err := a.PayLink(context.Background(), mkScope("o1"), "inv1")
	if err != nil || link != "" || len(pl.calls) != 0 {
		t.Fatalf("link=%q err=%v calls=%v", link, err, pl.calls)
	}
}

func TestPayLink_UsesDefaultSenderAndPropagatesError(t *testing.T) {
	pl := &fakePayLinker{}
	f := &recFeatures{}
	a := mikrotikPublicAdapter{settings: fakeNotifSettings{sender: "u1"}, links: pl, features: f}
	link, err := a.PayLink(context.Background(), mkScope("o1"), "inv1")
	if err != nil || link != "https://pay.example/inv1" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	if len(f.keys) != 1 || f.keys[0] != "receivable.online_payment" {
		t.Fatalf("feature keys=%v", f.keys)
	}
	boom := errors.New("boom")
	a.links = failLinker{err: boom}
	if _, err := a.PayLink(context.Background(), mkScope("o1"), "inv1"); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

// Without receivable.online_payment the public invoice page cannot take the
// payment, so the isolir page must not offer a Bayar link.
func TestPayLink_NoOnlinePaymentEntitlementReturnsEmpty(t *testing.T) {
	pl := &fakePayLinker{}
	a := mikrotikPublicAdapter{settings: fakeNotifSettings{sender: "u1"}, links: pl,
		features: &recFeatures{err: organization.FeatureNotEnabledError()}}
	link, err := a.PayLink(context.Background(), mkScope("o1"), "inv1")
	if err != nil || link != "" || len(pl.calls) != 0 {
		t.Fatalf("link=%q err=%v calls=%v", link, err, pl.calls)
	}
	a.features = &recFeatures{err: errors.New("store down")}
	if _, err := a.PayLink(context.Background(), mkScope("o1"), "inv1"); err == nil {
		t.Fatal("infrastructure error must be returned")
	}
}

func TestFeatureGate_UsesMikrotikEnabled(t *testing.T) {
	f := &recFeatures{}
	a := mikrotikPublicAdapter{features: f}
	ok, err := a.Enabled(context.Background(), mkScope("o1"))
	if err != nil || !ok || len(f.keys) != 1 || f.keys[0] != "mikrotik.enabled" {
		t.Fatalf("ok=%v err=%v keys=%v", ok, err, f.keys)
	}
	f.err = organization.FeatureNotEnabledError()
	if ok, err := a.Enabled(context.Background(), mkScope("o1")); ok || err != nil {
		t.Fatalf("denied: ok=%v err=%v", ok, err)
	}
	f.err = errors.New("store down")
	if _, err := a.Enabled(context.Background(), mkScope("o1")); err == nil {
		t.Fatal("infrastructure error must be returned")
	}
}

func TestProfile_MapsOrganizationProfile(t *testing.T) {
	a := mikrotikPublicAdapter{issuer: profileIssuer{}}
	got, err := a.Profile(context.Background(), mkScope("o1"))
	if err != nil || got.Name != "ISP Jaya" || got.Phone != "0812" || got.Email != "a@isp.id" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

type fakeNameLookup struct{ ids []string }

func (f fakeNameLookup) AccountIDsByName(context.Context, coretenant.Scope, string) ([]string, error) {
	return f.ids, nil
}

func TestAccountsByUsername_ResolvesIDsToRefs(t *testing.T) {
	accs := &fakeAccounts{byOrg: map[string]map[string]receivabledomain.Account{
		"o1": {"a1": {ID: "a1", Name: "Budi", Phone: "0812"}},
	}}
	a := mikrotikPublicAdapter{names: fakeNameLookup{ids: []string{"a1"}}, accounts: accs}
	got, err := a.AccountsByUsername(context.Background(), mkScope("o1"), "budi")
	if err != nil || len(got) != 1 || got[0].ID != "a1" || got[0].Name != "Budi" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

type emptyIssuer struct{}

func (emptyIssuer) OrganizationProfile(context.Context, coretenant.Scope) (docpdf.Party, error) {
	return docpdf.Party{Name: "ISP Jaya"}, nil
}

type fakeContactSessions struct {
	list []wadomain.Session
	err  error
}

func (f fakeContactSessions) List(context.Context, coretenant.Scope) ([]wadomain.Session, error) {
	return f.list, f.err
}

type fakeContactMailboxes struct {
	byUser map[string][]mailboxdomain.Mailbox
	err    error
}

func (f fakeContactMailboxes) ListByUser(_ context.Context, _ coretenant.Scope, userID string) ([]mailboxdomain.Mailbox, error) {
	return f.byUser[userID], f.err
}

// Without a branding contact, the CS contact falls back to the connected channels that also send
// the invoices: the default working WhatsApp session and the default sender's active mailbox
// (prod finding T13-E1: no UI could fill the branding contact).
func TestProfile_FallsBackToConnectedChannels(t *testing.T) {
	a := mikrotikPublicAdapter{
		issuer:   emptyIssuer{},
		settings: fakeNotifSettings{sender: "u1"},
		sessions: fakeContactSessions{list: []wadomain.Session{
			{Phone: "6281111", Status: wadomain.SessionStatusWorking},
			{Phone: "6282222", Status: wadomain.SessionStatusStopped, IsDefault: true},
			{Phone: "6283333", Status: wadomain.SessionStatusWorking, IsDefault: true},
		}},
		mailboxes: fakeContactMailboxes{byUser: map[string][]mailboxdomain.Mailbox{"u1": {
			{EmailAddress: "old@isp.id", Status: mailboxdomain.MailboxStatus("error")},
			{EmailAddress: "billing@isp.id", Status: mailboxdomain.MailboxStatusActive},
		}}},
	}
	got, err := a.Profile(context.Background(), mkScope("o1"))
	if err != nil || got.Name != "ISP Jaya" || got.Phone != "6283333" || got.Email != "billing@isp.id" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestProfile_BrandingContactWinsOverChannels(t *testing.T) {
	a := mikrotikPublicAdapter{
		issuer:    profileIssuer{},
		settings:  fakeNotifSettings{sender: "u1"},
		sessions:  fakeContactSessions{list: []wadomain.Session{{Phone: "6283333", Status: wadomain.SessionStatusWorking, IsDefault: true}}},
		mailboxes: fakeContactMailboxes{byUser: map[string][]mailboxdomain.Mailbox{"u1": {{EmailAddress: "billing@isp.id", Status: mailboxdomain.MailboxStatusActive}}}},
	}
	got, _ := a.Profile(context.Background(), mkScope("o1"))
	if got.Phone != "0812" || got.Email != "a@isp.id" {
		t.Fatalf("got %+v", got)
	}
}

// Channel lookups are a best-effort fallback: no connected channel or a lookup error leaves the
// contact empty (the activation gate then refuses) instead of failing the request.
func TestProfile_NoConnectedChannelLeavesContactEmpty(t *testing.T) {
	a := mikrotikPublicAdapter{
		issuer:    emptyIssuer{},
		settings:  fakeNotifSettings{sender: ""},
		sessions:  fakeContactSessions{err: errors.New("boom")},
		mailboxes: fakeContactMailboxes{},
	}
	got, err := a.Profile(context.Background(), mkScope("o1"))
	if err != nil || got.Phone != "" || got.Email != "" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
