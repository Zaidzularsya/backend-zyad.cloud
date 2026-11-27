package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"zyad.cloud/internal/modules/social/domain"
)

// Claim offer kinds.
const (
	// ClaimKindEmailMatch: a CRM contact whose email equals the user's verified email.
	ClaimKindEmailMatch = "email_match"
	// ClaimKindStaffRequest: a pending_user link created by tenant staff for this user.
	ClaimKindStaffRequest = "staff_request"
)

// AuthUser is the signed-in user as the claim flow needs it.
type AuthUser struct {
	ID            string
	Email         string
	EmailVerified bool
}

// ClaimOffer is one "are you this customer?" prompt for the user.
type ClaimOffer struct {
	OrganizationID   string
	OrganizationName string
	ContactID        string
	ContactName      string
	Kind             string
	// LinkID is set for Kind == ClaimKindStaffRequest.
	LinkID string
}

// ClaimableContact is a CRM contact that is not deleted and is a real customer of
// its organization (has a billing account or a confirmed/paired PPPoE onboarding).
type ClaimableContact struct {
	OrganizationID   string
	OrganizationName string
	ContactID        string
	ContactName      string
	Email            string
}

// ContactDirectory reads CRM contacts across organizations. Implementations must
// check each contact inside its own tenant scope (crm/receivable/mikrotik are RLS).
type ContactDirectory interface {
	// FindClaimableByEmail returns claimable contacts whose normalized email
	// equals the normalized email, across all active organizations.
	FindClaimableByEmail(ctx context.Context, email string) ([]ClaimableContact, error)
	// IsClaimable re-reads one contact; found=false when it is missing, deleted,
	// in an inactive organization, or not a customer.
	IsClaimable(ctx context.Context, organizationID, contactID string) (ClaimableContact, bool, error)
}

// CustomerLinkStore persists customer_links. Writes also write their audit_logs row.
type CustomerLinkStore interface {
	// ListByUser returns every link of the user, any status.
	ListByUser(ctx context.Context, userID string) ([]domain.CustomerLink, error)
	FindByID(ctx context.Context, linkID string) (domain.CustomerLink, bool, error)
	HasLiveLinkForContact(ctx context.Context, organizationID, contactID string) (bool, error)
	// CreateActive inserts an active link; domain.ErrCustomerLinkExists on a live-link conflict.
	CreateActive(ctx context.Context, link domain.CustomerLink) (domain.CustomerLink, error)
	// ActivatePending turns the user's pending_user link active (verified_via=staff);
	// domain.ErrClaimNotAvailable when it is no longer pending.
	ActivatePending(ctx context.Context, linkID, userID string) (domain.CustomerLink, error)
	// DeclinePending marks the user's pending_user link declined.
	DeclinePending(ctx context.Context, linkID, userID string) error
	// InsertDeclined records that the user declined an email-match offer.
	InsertDeclined(ctx context.Context, userID, organizationID, contactID string) error
	// Revoke ends a live link; domain.ErrCustomerLinkNotFound when it is no longer live.
	Revoke(ctx context.Context, link domain.CustomerLink, actorUserID string, byStaff bool) error
}

// TenantSettingsReader returns a tenant's social settings (defaults applied).
type TenantSettingsReader interface {
	Get(ctx context.Context, organizationID string) (domain.TenantSettings, error)
}

// CustomerAttribution credits the user's referrer when the user becomes a customer.
type CustomerAttribution interface {
	RecordBecameCustomer(ctx context.Context, userID string) error
}

// ClaimService lets a user find and claim the CRM contacts that are them
// (trigger A: verified email match; trigger C: staff request).
type ClaimService struct {
	contacts    ContactDirectory
	links       CustomerLinkStore
	settings    TenantSettingsReader
	attribution CustomerAttribution
	log         *slog.Logger
}

func NewClaimService(contacts ContactDirectory, links CustomerLinkStore, settings TenantSettingsReader, attribution CustomerAttribution) *ClaimService {
	return &ClaimService{
		contacts:    contacts,
		links:       links,
		settings:    settings,
		attribution: attribution,
		log:         slog.Default(),
	}
}

// Offers lists email matches the user may claim plus staff requests awaiting
// the user. Email matches are left out when the tenant turned auto-offer off,
// the contact already has a live link, the user already has a live link in
// that organization, or the user declined that contact before.
func (s *ClaimService) Offers(ctx context.Context, user AuthUser) ([]ClaimOffer, error) {
	if !user.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}
	links, err := s.links.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list customer links: %w", err)
	}
	offers := make([]ClaimOffer, 0)

	if email := domain.NormalizeEmail(user.Email); email != "" {
		candidates, err := s.contacts.FindClaimableByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("find claimable contacts: %w", err)
		}
		autoOffer := map[string]bool{}
		for _, c := range candidates {
			if domain.NormalizeEmail(c.Email) != email ||
				hasDeclined(links, c.OrganizationID, c.ContactID) ||
				hasLiveInOrganization(links, c.OrganizationID) {
				continue
			}
			enabled, ok := autoOffer[c.OrganizationID]
			if !ok {
				settings, err := s.settings.Get(ctx, c.OrganizationID)
				if err != nil {
					return nil, err
				}
				enabled = settings.CustomerClaimAutoOffer
				autoOffer[c.OrganizationID] = enabled
			}
			if !enabled {
				continue
			}
			taken, err := s.links.HasLiveLinkForContact(ctx, c.OrganizationID, c.ContactID)
			if err != nil {
				return nil, fmt.Errorf("check live link: %w", err)
			}
			if taken {
				continue
			}
			offers = append(offers, ClaimOffer{
				OrganizationID: c.OrganizationID, OrganizationName: c.OrganizationName,
				ContactID: c.ContactID, ContactName: c.ContactName, Kind: ClaimKindEmailMatch,
			})
		}
	}

	for _, link := range links {
		if link.Status != domain.LinkStatusPendingUser {
			continue
		}
		c, ok, err := s.contacts.IsClaimable(ctx, link.OrganizationID, link.ContactID)
		if err != nil {
			return nil, fmt.Errorf("check contact of link %s: %w", link.ID, err)
		}
		if !ok {
			continue
		}
		offers = append(offers, ClaimOffer{
			OrganizationID: c.OrganizationID, OrganizationName: c.OrganizationName,
			ContactID: c.ContactID, ContactName: c.ContactName, Kind: ClaimKindStaffRequest, LinkID: link.ID,
		})
	}
	return offers, nil
}

// Accept claims a contact. Every check of Offers is repeated because the offer
// may be stale. A staff request becomes active with verified_via=staff; an
// email match is inserted with verified_via=email.
func (s *ClaimService) Accept(ctx context.Context, user AuthUser, organizationID, contactID string) (domain.CustomerLink, error) {
	if !user.EmailVerified {
		return domain.CustomerLink{}, domain.ErrEmailNotVerified
	}
	organizationID, contactID = strings.TrimSpace(organizationID), strings.TrimSpace(contactID)
	links, err := s.links.ListByUser(ctx, user.ID)
	if err != nil {
		return domain.CustomerLink{}, fmt.Errorf("list customer links: %w", err)
	}

	var link domain.CustomerLink
	if pending, ok := findLink(links, organizationID, contactID, domain.LinkStatusPendingUser); ok {
		if _, claimable, err := s.contacts.IsClaimable(ctx, organizationID, contactID); err != nil {
			return domain.CustomerLink{}, err
		} else if !claimable {
			return domain.CustomerLink{}, domain.ErrClaimNotAvailable
		}
		link, err = s.links.ActivatePending(ctx, pending.ID, user.ID)
	} else {
		if err := s.checkEmailMatch(ctx, user, links, organizationID, contactID); err != nil {
			return domain.CustomerLink{}, err
		}
		link, err = s.links.CreateActive(ctx, domain.CustomerLink{
			UserID:         user.ID,
			OrganizationID: organizationID,
			ContactID:      contactID,
			Status:         domain.LinkStatusActive,
			VerifiedVia:    domain.VerifiedViaEmail,
		})
	}
	if err != nil {
		return domain.CustomerLink{}, err
	}

	recordBecameCustomer(ctx, s.attribution, s.log, user.ID)
	return link, nil
}

// recordBecameCustomer credits the referrer after a link was committed. It is
// best-effort so a failure does not turn a successful claim into an error the
// client would retry.
func recordBecameCustomer(ctx context.Context, attribution CustomerAttribution, log *slog.Logger, userID string) {
	if attribution == nil {
		return
	}
	if err := attribution.RecordBecameCustomer(ctx, userID); err != nil {
		log.WarnContext(ctx, "record became_customer attribution failed", "user_id", userID, "error", err)
	}
}

// Decline hides an offer for good: a staff request becomes declined, an email
// match is remembered as declined so it is not offered again.
func (s *ClaimService) Decline(ctx context.Context, user AuthUser, organizationID, contactID string) error {
	if !user.EmailVerified {
		return domain.ErrEmailNotVerified
	}
	organizationID, contactID = strings.TrimSpace(organizationID), strings.TrimSpace(contactID)
	links, err := s.links.ListByUser(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("list customer links: %w", err)
	}
	if pending, ok := findLink(links, organizationID, contactID, domain.LinkStatusPendingUser); ok {
		return s.links.DeclinePending(ctx, pending.ID, user.ID)
	}
	if hasDeclined(links, organizationID, contactID) {
		return nil
	}
	if err := s.checkEmailMatch(ctx, user, links, organizationID, contactID); err != nil {
		if errors.Is(err, domain.ErrCustomerLinkExists) {
			return domain.ErrClaimNotAvailable
		}
		return err
	}
	return s.links.InsertDeclined(ctx, user.ID, organizationID, contactID)
}

// Revoke ends a live link. A user may revoke only their own link; byStaff
// skips that check, so staff callers must have scoped the link to their
// organization before calling.
func (s *ClaimService) Revoke(ctx context.Context, actorUserID, linkID string, byStaff bool) error {
	link, found, err := s.links.FindByID(ctx, strings.TrimSpace(linkID))
	if err != nil {
		return fmt.Errorf("load customer link: %w", err)
	}
	if !found || !link.IsLive() || (!byStaff && link.UserID != actorUserID) {
		return domain.ErrCustomerLinkNotFound
	}
	return s.links.Revoke(ctx, link, actorUserID, byStaff)
}

// checkEmailMatch verifies the user may claim the contact by verified email.
func (s *ClaimService) checkEmailMatch(ctx context.Context, user AuthUser, links []domain.CustomerLink, organizationID, contactID string) error {
	email := domain.NormalizeEmail(user.Email)
	if email == "" || hasDeclined(links, organizationID, contactID) {
		return domain.ErrClaimNotAvailable
	}
	contact, ok, err := s.contacts.IsClaimable(ctx, organizationID, contactID)
	if err != nil {
		return fmt.Errorf("check contact: %w", err)
	}
	if !ok || domain.NormalizeEmail(contact.Email) != email {
		return domain.ErrClaimNotAvailable
	}
	settings, err := s.settings.Get(ctx, organizationID)
	if err != nil {
		return err
	}
	if !settings.CustomerClaimAutoOffer {
		return domain.ErrClaimNotAvailable
	}
	taken, err := s.links.HasLiveLinkForContact(ctx, organizationID, contactID)
	if err != nil {
		return fmt.Errorf("check live link: %w", err)
	}
	if taken || hasLiveInOrganization(links, organizationID) {
		return domain.ErrCustomerLinkExists
	}
	return nil
}

func findLink(links []domain.CustomerLink, organizationID, contactID, status string) (domain.CustomerLink, bool) {
	for _, l := range links {
		if l.OrganizationID == organizationID && l.ContactID == contactID && l.Status == status {
			return l, true
		}
	}
	return domain.CustomerLink{}, false
}

func hasDeclined(links []domain.CustomerLink, organizationID, contactID string) bool {
	_, ok := findLink(links, organizationID, contactID, domain.LinkStatusDeclined)
	return ok
}

func hasLiveInOrganization(links []domain.CustomerLink, organizationID string) bool {
	for _, l := range links {
		if l.OrganizationID == organizationID && l.IsLive() {
			return true
		}
	}
	return false
}
