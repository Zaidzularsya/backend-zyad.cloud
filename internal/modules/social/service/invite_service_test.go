package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social/domain"
)

// fakeInviteStore mirrors InviteRepository: Create supersedes open invites of
// the contact, Accept consumes atomically and links with verified_via=invite.
type fakeInviteStore struct {
	invites []domain.ClaimInvite
	links   *fakeLinkStore
}

func (f *fakeInviteStore) Create(_ context.Context, invite domain.ClaimInvite) (domain.ClaimInvite, error) {
	for i := range f.invites {
		if f.invites[i].OrganizationID == invite.OrganizationID && f.invites[i].ContactID == invite.ContactID && f.invites[i].UsedAt == nil {
			at := invite.CreatedAt
			f.invites[i].UsedAt = &at
		}
	}
	invite.ID = "inv-" + string(rune('a'+len(f.invites)))
	f.invites = append(f.invites, invite)
	return invite, nil
}

func (f *fakeInviteStore) FindByTokenHash(_ context.Context, hash string) (domain.ClaimInvite, bool, error) {
	for _, inv := range f.invites {
		if inv.TokenHash == hash {
			return inv, true, nil
		}
	}
	return domain.ClaimInvite{}, false, nil
}

func (f *fakeInviteStore) Accept(ctx context.Context, hash, userID string, now time.Time) (domain.CustomerLink, error) {
	for i, inv := range f.invites {
		if inv.TokenHash != hash || inv.UsedAt != nil || !now.Before(inv.ExpiresAt) {
			continue
		}
		link, err := f.links.CreateActive(ctx, domain.CustomerLink{
			UserID: userID, OrganizationID: inv.OrganizationID, ContactID: inv.ContactID,
			Status: domain.LinkStatusActive, VerifiedVia: domain.VerifiedViaInvite,
		})
		if err != nil {
			return domain.CustomerLink{}, err
		}
		f.invites[i].UsedAt = &now
		return link, nil
	}
	return domain.CustomerLink{}, domain.ErrInviteInvalid
}

type fakeInviteSender struct{ sent []ClaimInviteEmail }

func (f *fakeInviteSender) SendClaimInvite(_ context.Context, msg ClaimInviteEmail) error {
	f.sent = append(f.sent, msg)
	return nil
}

type fakeEmailVerifier struct{ verified, emails []string }

func (f *fakeEmailVerifier) MarkEmailVerified(_ context.Context, userID, email string, _ time.Time) error {
	f.verified = append(f.verified, userID)
	f.emails = append(f.emails, email)
	return nil
}

type inviteFixture struct {
	svc      *InviteService
	dir      *fakeDirectory
	links    *fakeLinkStore
	invites  *fakeInviteStore
	sender   *fakeInviteSender
	verifier *fakeEmailVerifier
	attr     *fakeCustomerAttribution
	now      time.Time
}

func newInviteFixture(t *testing.T) *inviteFixture {
	t.Helper()
	dir, links, attr := newClaimFixture()
	dir.contacts = append(dir.contacts, ClaimableContact{OrganizationID: "org-a", OrganizationName: "ISP A", ContactID: "c-noemail", ContactName: "Tanpa Email"})
	f := &inviteFixture{
		dir: dir, links: links, attr: attr,
		invites:  &fakeInviteStore{links: links},
		sender:   &fakeInviteSender{},
		verifier: &fakeEmailVerifier{},
		now:      time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC),
	}
	f.svc = NewInviteService(dir, links, f.invites, f.sender, f.verifier, attr, "https://app.example.test/")
	f.svc.now = func() time.Time { return f.now }
	return f
}

func scopeOf(t *testing.T, orgID string) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "isp-a",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceInternal, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// lastToken extracts the raw token from the claim URL of the last email.
func (f *inviteFixture) lastToken(t *testing.T) string {
	t.Helper()
	if len(f.sender.sent) == 0 {
		t.Fatal("no invite email sent")
	}
	u, err := url.Parse(f.sender.sent[len(f.sender.sent)-1].ClaimURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/claim" {
		t.Fatalf("claim url path = %q, want /claim", u.Path)
	}
	return u.Query().Get("token")
}

func TestSendRequiresEligibleContactWithEmail(t *testing.T) {
	f := newInviteFixture(t)
	scope := scopeOf(t, "org-a")

	f.dir.notCustomer["c-a"] = true
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("not a customer: err = %v, want ErrClaimNotAvailable", err)
	}
	f.dir.notCustomer["c-a"] = false

	if err := f.svc.Send(context.Background(), scopeOf(t, "org-b"), "staff-1", "c-a"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("other organization: err = %v, want ErrClaimNotAvailable", err)
	}
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-noemail"); !errors.Is(err, domain.ErrContactEmailMissing) {
		t.Fatalf("no email: err = %v, want ErrContactEmailMissing", err)
	}
	if len(f.sender.sent) != 0 || len(f.invites.invites) != 0 {
		t.Fatalf("rejected sends must not store or send: %d invites, %d emails", len(f.invites.invites), len(f.sender.sent))
	}

	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	inv := f.invites.invites[0]
	if inv.Email != "budi@x.test" || inv.OrganizationID != "org-a" || inv.CreatedBy != "staff-1" {
		t.Fatalf("stored invite = %+v", inv)
	}
	if want := f.now.Add(7 * 24 * time.Hour); !inv.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", inv.ExpiresAt, want)
	}
	token := f.lastToken(t)
	if token == "" || inv.TokenHash != coreauth.HashToken(token) || strings.Contains(inv.TokenHash, token) {
		t.Fatalf("token must be stored only as its hash")
	}
	msg := f.sender.sent[0]
	if !strings.HasPrefix(msg.ClaimURL, "https://app.example.test/claim?token=") || msg.Email != "budi@x.test" ||
		msg.OrganizationName != "ISP A" || msg.ContactName != "Budi" {
		t.Fatalf("email = %+v", msg)
	}

	f.links.links = append(f.links.links, domain.CustomerLink{ID: "l1", UserID: "someone", OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive})
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); !errors.Is(err, domain.ErrCustomerLinkExists) {
		t.Fatalf("linked contact: err = %v, want ErrCustomerLinkExists", err)
	}
}

func TestSendSupersedesPreviousInvite(t *testing.T) {
	f := newInviteFixture(t)
	scope := scopeOf(t, "org-a")
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	first := f.lastToken(t)
	f.now = f.now.Add(time.Hour)
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	second := f.lastToken(t)
	if first == second {
		t.Fatal("resend must issue a new token")
	}

	user := AuthUser{ID: "user-budi", Email: "budi@x.test", EmailVerified: true}
	if _, err := f.svc.Accept(context.Background(), user, first); !errors.Is(err, domain.ErrInviteInvalid) {
		t.Fatalf("old invite: err = %v, want ErrInviteInvalid", err)
	}
	if _, err := f.svc.Accept(context.Background(), user, second); err != nil {
		t.Fatalf("new invite: %v", err)
	}
}

func TestAcceptInviteSingleUse(t *testing.T) {
	f := newInviteFixture(t)
	if err := f.svc.Send(context.Background(), scopeOf(t, "org-a"), "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	token := f.lastToken(t)
	user := AuthUser{ID: "user-budi", Email: "budi@x.test", EmailVerified: true}

	link, err := f.svc.Accept(context.Background(), user, token)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if link.VerifiedVia != domain.VerifiedViaInvite || link.Status != domain.LinkStatusActive ||
		link.OrganizationID != "org-a" || link.ContactID != "c-a" || link.UserID != user.ID {
		t.Fatalf("link = %+v", link)
	}
	if len(f.attr.users) != 1 || f.attr.users[0] != user.ID {
		t.Fatalf("became_customer attribution = %v", f.attr.users)
	}

	other := AuthUser{ID: "user-other", Email: "other@x.test", EmailVerified: true}
	if _, err := f.svc.Accept(context.Background(), other, token); !errors.Is(err, domain.ErrInviteInvalid) {
		t.Fatalf("second accept: err = %v, want ErrInviteInvalid", err)
	}
	if _, err := f.svc.Accept(context.Background(), user, "not-a-token"); !errors.Is(err, domain.ErrInviteInvalid) {
		t.Fatalf("unknown token: err = %v, want ErrInviteInvalid", err)
	}
	if len(f.links.created) != 1 {
		t.Fatalf("links created = %d, want 1", len(f.links.created))
	}
}

func TestAcceptInviteExpiredAfter7Days(t *testing.T) {
	f := newInviteFixture(t)
	if err := f.svc.Send(context.Background(), scopeOf(t, "org-a"), "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	token := f.lastToken(t)
	user := AuthUser{ID: "user-budi", Email: "budi@x.test", EmailVerified: true}

	f.now = f.now.Add(7 * 24 * time.Hour)
	if _, err := f.svc.Accept(context.Background(), user, token); !errors.Is(err, domain.ErrInviteExpired) {
		t.Fatalf("after 7 days: err = %v, want ErrInviteExpired", err)
	}
	f.now = f.now.Add(-time.Minute)
	if _, err := f.svc.Accept(context.Background(), user, token); err != nil {
		t.Fatalf("just before 7 days: %v", err)
	}
}

func TestAcceptInviteVerifiesMatchingEmail(t *testing.T) {
	f := newInviteFixture(t)
	scope := scopeOf(t, "org-a")
	f.dir.contacts = append(f.dir.contacts, ClaimableContact{OrganizationID: "org-a", OrganizationName: "ISP A", ContactID: "c-b", ContactName: "Sari", Email: "sari@x.test"})

	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	// Unverified account whose email equals the invited address: accepted and verified.
	unverified := AuthUser{ID: "user-budi", Email: " BUDI@x.test", EmailVerified: false}
	if _, err := f.svc.Accept(context.Background(), unverified, f.lastToken(t)); err != nil {
		t.Fatalf("Accept unverified matching: %v", err)
	}
	if len(f.verifier.verified) != 1 || f.verifier.verified[0] != "user-budi" {
		t.Fatalf("verified = %v, want [user-budi]", f.verifier.verified)
	}
	// The invited address is passed on so the repository binds verification to it.
	if domain.NormalizeEmail(f.verifier.emails[0]) != "budi@x.test" {
		t.Fatalf("verified email = %q, want the invite address", f.verifier.emails[0])
	}

	// Unverified account with a different email: link allowed, email stays unverified.
	if err := f.svc.Send(context.Background(), scope, "staff-1", "c-b"); err != nil {
		t.Fatal(err)
	}
	mismatch := AuthUser{ID: "user-x", Email: "x@x.test", EmailVerified: false}
	if _, err := f.svc.Accept(context.Background(), mismatch, f.lastToken(t)); err != nil {
		t.Fatalf("Accept unverified mismatch: %v", err)
	}
	if len(f.verifier.verified) != 1 {
		t.Fatalf("mismatching email must not be verified: %v", f.verifier.verified)
	}
}

func TestAcceptInviteRechecksEligibility(t *testing.T) {
	f := newInviteFixture(t)
	if err := f.svc.Send(context.Background(), scopeOf(t, "org-a"), "staff-1", "c-a"); err != nil {
		t.Fatal(err)
	}
	f.dir.deleted["c-a"] = true
	user := AuthUser{ID: "user-budi", Email: "budi@x.test", EmailVerified: true}
	if _, err := f.svc.Accept(context.Background(), user, f.lastToken(t)); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("deleted contact: err = %v, want ErrClaimNotAvailable", err)
	}
	if len(f.links.created) != 0 {
		t.Fatal("no link may be created for an ineligible contact")
	}
}
