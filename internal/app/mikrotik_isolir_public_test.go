package app

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
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
	a := mikrotikPublicAdapter{settings: fakeNotifSettings{sender: "  "}, links: pl}
	link, err := a.PayLink(context.Background(), mkScope("o1"), "inv1")
	if err != nil || link != "" || len(pl.calls) != 0 {
		t.Fatalf("link=%q err=%v calls=%v", link, err, pl.calls)
	}
}

func TestPayLink_UsesDefaultSenderAndPropagatesError(t *testing.T) {
	pl := &fakePayLinker{}
	a := mikrotikPublicAdapter{settings: fakeNotifSettings{sender: "u1"}, links: pl}
	link, err := a.PayLink(context.Background(), mkScope("o1"), "inv1")
	if err != nil || link != "https://pay.example/inv1" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	boom := errors.New("boom")
	a.links = failLinker{err: boom}
	if _, err := a.PayLink(context.Background(), mkScope("o1"), "inv1"); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
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
