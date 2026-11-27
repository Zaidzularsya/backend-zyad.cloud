package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"zyad.cloud/internal/modules/social/domain"
)

// fakeDirectory compares emails exactly like the real adapter (lower(btrim)).
type fakeDirectory struct {
	contacts []ClaimableContact
	// notCustomer: contact ids without billing account or PPPoE onboarding.
	notCustomer map[string]bool
	deleted     map[string]bool
}

func (f *fakeDirectory) claimable(c ClaimableContact) bool {
	return !f.notCustomer[c.ContactID] && !f.deleted[c.ContactID]
}

func (f *fakeDirectory) FindClaimableByEmail(_ context.Context, email string) ([]ClaimableContact, error) {
	var out []ClaimableContact
	for _, c := range f.contacts {
		if domain.NormalizeEmail(c.Email) == domain.NormalizeEmail(email) && f.claimable(c) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeDirectory) IsClaimable(_ context.Context, orgID, contactID string) (ClaimableContact, bool, error) {
	for _, c := range f.contacts {
		if c.OrganizationID == orgID && c.ContactID == contactID {
			return c, f.claimable(c), nil
		}
	}
	return ClaimableContact{}, false, nil
}

type fakeLinkStore struct {
	links       []domain.CustomerLink
	createErr   error
	created     []domain.CustomerLink
	activated   []string
	declined    []string
	revoked     []string
	nextID      int
	requestedBy []string
}

func (f *fakeLinkStore) ListByUser(_ context.Context, userID string) ([]domain.CustomerLink, error) {
	var out []domain.CustomerLink
	for _, l := range f.links {
		if l.UserID == userID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeLinkStore) FindByID(_ context.Context, id string) (domain.CustomerLink, bool, error) {
	for _, l := range f.links {
		if l.ID == id {
			return l, true, nil
		}
	}
	return domain.CustomerLink{}, false, nil
}

func (f *fakeLinkStore) HasLiveLinkForContact(_ context.Context, orgID, contactID string) (bool, error) {
	for _, l := range f.links {
		if l.OrganizationID == orgID && l.ContactID == contactID && l.IsLive() {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeLinkStore) CreateActive(_ context.Context, link domain.CustomerLink) (domain.CustomerLink, error) {
	if f.createErr != nil {
		return domain.CustomerLink{}, f.createErr
	}
	link.ID = "new-link"
	f.created = append(f.created, link)
	f.links = append(f.links, link)
	return link, nil
}

func (f *fakeLinkStore) CreatePending(_ context.Context, link domain.CustomerLink, requestedBy string) (domain.CustomerLink, error) {
	for _, l := range f.links {
		if l.OrganizationID == link.OrganizationID && l.IsLive() && (l.ContactID == link.ContactID || l.UserID == link.UserID) {
			return domain.CustomerLink{}, domain.ErrCustomerLinkExists
		}
	}
	f.nextID++
	link.ID = fmt.Sprintf("pending-%d", f.nextID)
	link.Status = domain.LinkStatusPendingUser
	link.VerifiedVia = domain.VerifiedViaStaff
	f.requestedBy = append(f.requestedBy, requestedBy)
	f.links = append(f.links, link)
	return link, nil
}

func (f *fakeLinkStore) ListLiveByContact(_ context.Context, orgID, contactID string) ([]domain.CustomerLink, error) {
	var out []domain.CustomerLink
	for _, l := range f.links {
		if l.OrganizationID == orgID && l.ContactID == contactID && l.IsLive() {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeLinkStore) ActivatePending(_ context.Context, linkID, _ string) (domain.CustomerLink, error) {
	for i, l := range f.links {
		if l.ID == linkID && l.Status == domain.LinkStatusPendingUser {
			f.links[i].Status = domain.LinkStatusActive
			f.links[i].VerifiedVia = domain.VerifiedViaStaff
			f.activated = append(f.activated, linkID)
			return f.links[i], nil
		}
	}
	return domain.CustomerLink{}, domain.ErrClaimNotAvailable
}

func (f *fakeLinkStore) DeclinePending(_ context.Context, linkID, _ string) error {
	f.declined = append(f.declined, linkID)
	return nil
}

func (f *fakeLinkStore) InsertDeclined(_ context.Context, userID, orgID, contactID string) error {
	f.declined = append(f.declined, orgID+"/"+contactID)
	f.links = append(f.links, domain.CustomerLink{UserID: userID, OrganizationID: orgID, ContactID: contactID, Status: domain.LinkStatusDeclined})
	return nil
}

func (f *fakeLinkStore) Revoke(_ context.Context, link domain.CustomerLink, _ string, _ bool) error {
	f.revoked = append(f.revoked, link.ID)
	return nil
}

type fakeSettingsReader struct{ off map[string]bool }

func (f fakeSettingsReader) Get(_ context.Context, orgID string) (domain.TenantSettings, error) {
	s := domain.DefaultTenantSettings(orgID)
	s.CustomerClaimAutoOffer = !f.off[orgID]
	return s, nil
}

type fakeCustomerAttribution struct{ users []string }

func (f *fakeCustomerAttribution) RecordBecameCustomer(_ context.Context, userID string) error {
	f.users = append(f.users, userID)
	return nil
}

var budi = AuthUser{ID: "user-budi", Email: "budi@x.test", EmailVerified: true}

func newClaimFixture() (*fakeDirectory, *fakeLinkStore, *fakeCustomerAttribution) {
	return &fakeDirectory{
			contacts: []ClaimableContact{
				{OrganizationID: "org-a", OrganizationName: "ISP A", ContactID: "c-a", ContactName: "Budi", Email: " Budi@X.test "},
			},
			notCustomer: map[string]bool{},
			deleted:     map[string]bool{},
		},
		&fakeLinkStore{},
		&fakeCustomerAttribution{}
}

func TestOffersMatchesEmailCaseInsensitive(t *testing.T) {
	dir, links, attr := newClaimFixture()
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	offers, err := svc.Offers(context.Background(), budi)
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].ContactID != "c-a" || offers[0].Kind != ClaimKindEmailMatch || offers[0].OrganizationName != "ISP A" {
		t.Fatalf("offers = %+v", offers)
	}
}

func TestOffersExcludesLeadWithoutBillingOrPPPoE(t *testing.T) {
	dir, links, attr := newClaimFixture()
	dir.notCustomer["c-a"] = true
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	offers, err := svc.Offers(context.Background(), budi)
	if err != nil || len(offers) != 0 {
		t.Fatalf("offers = %+v, %v; want none", offers, err)
	}
}

func TestOffersRespectsTenantAutoOfferOff(t *testing.T) {
	dir, links, attr := newClaimFixture()
	// A staff request in the same tenant is still offered: opt-out only covers email matches.
	dir.contacts = append(dir.contacts, ClaimableContact{OrganizationID: "org-b", OrganizationName: "ISP B", ContactID: "c-b", ContactName: "Budi B", Email: "other@x.test"})
	links.links = []domain.CustomerLink{{ID: "link-b", UserID: budi.ID, OrganizationID: "org-b", ContactID: "c-b", Status: domain.LinkStatusPendingUser, VerifiedVia: domain.VerifiedViaStaff}}
	svc := NewClaimService(dir, links, fakeSettingsReader{off: map[string]bool{"org-a": true, "org-b": true}}, attr)

	offers, err := svc.Offers(context.Background(), budi)
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].Kind != ClaimKindStaffRequest || offers[0].LinkID != "link-b" {
		t.Fatalf("offers = %+v; want only the staff request", offers)
	}
	if _, err := svc.Accept(context.Background(), budi, "org-a", "c-a"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("Accept with auto-offer off err = %v, want ErrClaimNotAvailable", err)
	}
}

func TestOffersExcludesDeclined(t *testing.T) {
	dir, links, attr := newClaimFixture()
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)
	if err := svc.Decline(context.Background(), budi, "org-a", "c-a"); err != nil {
		t.Fatalf("Decline: %v", err)
	}
	offers, err := svc.Offers(context.Background(), budi)
	if err != nil || len(offers) != 0 {
		t.Fatalf("offers after decline = %+v, %v", offers, err)
	}
	if _, err := svc.Accept(context.Background(), budi, "org-a", "c-a"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("Accept after decline err = %v", err)
	}
}

func TestOffersExcludesContactWithLiveLink(t *testing.T) {
	dir, links, attr := newClaimFixture()
	links.links = []domain.CustomerLink{{ID: "l-other", UserID: "user-other", OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive}}
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	offers, err := svc.Offers(context.Background(), budi)
	if err != nil || len(offers) != 0 {
		t.Fatalf("offers = %+v, %v; want none", offers, err)
	}
}

func TestOffersRequiresVerifiedEmail(t *testing.T) {
	dir, links, attr := newClaimFixture()
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)
	unverified := budi
	unverified.EmailVerified = false

	if _, err := svc.Offers(context.Background(), unverified); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("Offers err = %v", err)
	}
	if _, err := svc.Accept(context.Background(), unverified, "org-a", "c-a"); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("Accept err = %v", err)
	}
	if err := svc.Decline(context.Background(), unverified, "org-a", "c-a"); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("Decline err = %v", err)
	}
}

func TestAcceptEmailMatchCreatesActiveLinkAndCreditsReferrer(t *testing.T) {
	dir, links, attr := newClaimFixture()
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	link, err := svc.Accept(context.Background(), budi, "org-a", "c-a")
	if err != nil {
		t.Fatal(err)
	}
	if link.Status != domain.LinkStatusActive || link.VerifiedVia != domain.VerifiedViaEmail || link.UserID != budi.ID {
		t.Fatalf("link = %+v", link)
	}
	if len(attr.users) != 1 || attr.users[0] != budi.ID {
		t.Fatalf("became_customer calls = %v", attr.users)
	}
}

func TestAcceptStaffRequestActivatesPendingLink(t *testing.T) {
	dir, links, attr := newClaimFixture()
	// Staff requests do not need an email match.
	dir.contacts[0].Email = "someone-else@x.test"
	links.links = []domain.CustomerLink{{ID: "link-1", UserID: budi.ID, OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusPendingUser, VerifiedVia: domain.VerifiedViaStaff}}
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	link, err := svc.Accept(context.Background(), budi, "org-a", "c-a")
	if err != nil {
		t.Fatal(err)
	}
	if link.ID != "link-1" || link.Status != domain.LinkStatusActive || link.VerifiedVia != domain.VerifiedViaStaff {
		t.Fatalf("link = %+v", link)
	}
	if len(links.created) != 0 {
		t.Fatalf("created = %+v; want none", links.created)
	}
}

func TestAcceptRechecksEligibility(t *testing.T) {
	dir, links, attr := newClaimFixture()
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)
	offers, err := svc.Offers(context.Background(), budi)
	if err != nil || len(offers) != 1 {
		t.Fatalf("offers = %+v, %v", offers, err)
	}

	dir.deleted["c-a"] = true // contact deleted between Offers and Accept

	if _, err := svc.Accept(context.Background(), budi, "org-a", "c-a"); !errors.Is(err, domain.ErrClaimNotAvailable) {
		t.Fatalf("Accept err = %v, want ErrClaimNotAvailable", err)
	}
	if len(links.created) != 0 || len(attr.users) != 0 {
		t.Fatalf("created = %+v, attribution = %v; want nothing", links.created, attr.users)
	}
}

func TestAcceptConflictReturns409(t *testing.T) {
	dir, links, attr := newClaimFixture()
	// A concurrent claim wins between the pre-checks and the insert.
	links.createErr = domain.ErrCustomerLinkExists
	svc := NewClaimService(dir, links, fakeSettingsReader{}, attr)

	if _, err := svc.Accept(context.Background(), budi, "org-a", "c-a"); !errors.Is(err, domain.ErrCustomerLinkExists) {
		t.Fatalf("Accept err = %v, want ErrCustomerLinkExists", err)
	}
	if len(attr.users) != 0 {
		t.Fatalf("attribution on conflict = %v", attr.users)
	}
}

func TestRevokeOnlyOwnLiveLink(t *testing.T) {
	_, links, _ := newClaimFixture()
	links.links = []domain.CustomerLink{
		{ID: "mine", UserID: budi.ID, OrganizationID: "org-a", ContactID: "c-a", Status: domain.LinkStatusActive},
		{ID: "theirs", UserID: "user-other", OrganizationID: "org-b", ContactID: "c-b", Status: domain.LinkStatusActive},
		{ID: "old", UserID: budi.ID, OrganizationID: "org-c", ContactID: "c-c", Status: domain.LinkStatusRevoked},
	}
	svc := NewClaimService(&fakeDirectory{}, links, fakeSettingsReader{}, nil)
	ctx := context.Background()

	if err := svc.Revoke(ctx, budi.ID, "theirs", false); !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("revoke other's link err = %v", err)
	}
	if err := svc.Revoke(ctx, budi.ID, "old", false); !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("revoke revoked link err = %v", err)
	}
	if err := svc.Revoke(ctx, budi.ID, "mine", false); err != nil {
		t.Fatalf("revoke own link: %v", err)
	}
	if err := svc.Revoke(ctx, "staff-1", "theirs", true); err != nil {
		t.Fatalf("staff revoke: %v", err)
	}
	if len(links.revoked) != 2 || links.revoked[0] != "mine" || links.revoked[1] != "theirs" {
		t.Fatalf("revoked = %v", links.revoked)
	}
}
