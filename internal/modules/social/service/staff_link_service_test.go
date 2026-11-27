package service

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/social/domain"
)

type fakeUserDirectory struct{ ids map[string]string }

func (f fakeUserDirectory) FindUserIDByEmail(_ context.Context, email string) (string, bool, error) {
	id, ok := f.ids[domain.NormalizeEmail(email)]
	return id, ok, nil
}

type staffFixture struct {
	svc    *StaffLinkService
	claims *ClaimService
	dir    *fakeDirectory
	links  *fakeLinkStore
}

func newStaffFixture() *staffFixture {
	dir, links, attr := newClaimFixture()
	// Different address than the contact email: staff linking does not rely on email equality.
	users := fakeUserDirectory{ids: map[string]string{"budi.app@x.test": budi.ID}}
	claims := NewClaimService(dir, links, fakeSettingsReader{}, attr)
	return &staffFixture{svc: NewStaffLinkService(dir, links, users, claims), claims: claims, dir: dir, links: links}
}

func TestStaffRequestCreatesPendingUntilUserAccepts(t *testing.T) {
	f := newStaffFixture()
	ctx := context.Background()

	link, err := f.svc.Request(ctx, scopeOf(t, "org-a"), "staff-1", "c-a", "  Budi.App@X.test ")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if link.Status != domain.LinkStatusPendingUser || link.VerifiedVia != domain.VerifiedViaStaff ||
		link.UserID != budi.ID || link.OrganizationID != "org-a" || link.ContactID != "c-a" {
		t.Fatalf("link = %+v", link)
	}
	if len(f.links.requestedBy) != 1 || f.links.requestedBy[0] != "staff-1" {
		t.Fatalf("requested_by = %v", f.links.requestedBy)
	}

	offers, err := f.claims.Offers(ctx, AuthUser{ID: budi.ID, Email: "other@x.test", EmailVerified: true})
	if err != nil || len(offers) != 1 || offers[0].Kind != ClaimKindStaffRequest || offers[0].LinkID != link.ID {
		t.Fatalf("offers = %+v, %v", offers, err)
	}
	accepted, err := f.claims.Accept(ctx, AuthUser{ID: budi.ID, Email: "other@x.test", EmailVerified: true}, "org-a", "c-a")
	if err != nil || accepted.Status != domain.LinkStatusActive || accepted.VerifiedVia != domain.VerifiedViaStaff {
		t.Fatalf("accept = %+v, %v", accepted, err)
	}
}

func TestStaffRequestUnknownEmail404(t *testing.T) {
	f := newStaffFixture()
	_, err := f.svc.Request(context.Background(), scopeOf(t, "org-a"), "staff-1", "c-a", "nobody@x.test")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
	if len(f.links.links) != 0 {
		t.Fatalf("no link may be stored: %+v", f.links.links)
	}
}

func TestStaffRequestRejectsIneligibleOrTakenContact(t *testing.T) {
	f := newStaffFixture()
	ctx := context.Background()

	if _, err := f.svc.Request(ctx, scopeOf(t, "org-b"), "staff-9", "c-a", "budi.app@x.test"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("other tenant contact: err = %v, want ErrClaimNotAvailable", err)
	}
	f.dir.notCustomer["c-a"] = true
	if _, err := f.svc.Request(ctx, scopeOf(t, "org-a"), "staff-1", "c-a", "budi.app@x.test"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("lead: err = %v, want ErrClaimNotAvailable", err)
	}
	f.dir.notCustomer["c-a"] = false
	if _, err := f.svc.Request(ctx, scopeOf(t, "org-a"), "staff-1", "c-a", "budi.app@x.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Request(ctx, scopeOf(t, "org-a"), "staff-1", "c-a", "budi.app@x.test"); !errors.Is(err, domain.ErrCustomerLinkExists) {
		t.Fatalf("second request: err = %v, want ErrCustomerLinkExists", err)
	}
}

func TestStaffListForContactIsScopedToOrganization(t *testing.T) {
	f := newStaffFixture()
	f.links.links = []domain.CustomerLink{
		{ID: "l-a", UserID: "u1", OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive},
		{ID: "l-b", UserID: "u1", OrganizationID: "org-b", ContactID: "c-a", Status: domain.LinkStatusActive},
		{ID: "l-old", UserID: "u2", OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusRevoked},
	}
	got, err := f.svc.ListForContact(context.Background(), scopeOf(t, "org-a"), "c-a")
	if err != nil || len(got) != 1 || got[0].ID != "l-a" {
		t.Fatalf("list = %+v, %v", got, err)
	}
}

func TestStaffCanRevokeOwnTenantLink(t *testing.T) {
	f := newStaffFixture()
	f.links.links = []domain.CustomerLink{{ID: "l-a", UserID: "u1", OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive}}
	if err := f.svc.Revoke(context.Background(), scopeOf(t, "org-a"), "staff-1", "l-a"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(f.links.revoked) != 1 || f.links.revoked[0] != "l-a" {
		t.Fatalf("revoked = %v", f.links.revoked)
	}
}

func TestStaffCannotRevokeOtherTenantLink(t *testing.T) {
	f := newStaffFixture()
	f.links.links = []domain.CustomerLink{{ID: "l-b", UserID: "u1", OrganizationID: "org-b", ContactID: "c-b", Status: domain.LinkStatusActive}}
	err := f.svc.Revoke(context.Background(), scopeOf(t, "org-a"), "staff-1", "l-b")
	if !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("err = %v, want ErrCustomerLinkNotFound (same as unknown id)", err)
	}
	if len(f.links.revoked) != 0 {
		t.Fatalf("other tenant link must stay: revoked = %v", f.links.revoked)
	}
	if err := f.svc.Revoke(context.Background(), scopeOf(t, "org-a"), "staff-1", "missing"); !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("unknown id: err = %v", err)
	}
}

func TestUserCanRevokeOwnLink(t *testing.T) {
	f := newStaffFixture()
	f.links.links = []domain.CustomerLink{
		{ID: "mine", UserID: budi.ID, OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive},
		{ID: "theirs", UserID: "someone", OrganizationID: "org-a", ContactID: "c-x", Status: domain.LinkStatusActive},
	}
	if err := f.claims.Revoke(context.Background(), budi.ID, "mine", false); err != nil {
		t.Fatalf("revoke own: %v", err)
	}
	if err := f.claims.Revoke(context.Background(), budi.ID, "theirs", false); !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("revoke other's: err = %v, want ErrCustomerLinkNotFound", err)
	}
}
