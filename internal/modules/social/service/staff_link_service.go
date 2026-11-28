package service

import (
	"context"
	"fmt"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social/domain"
)

// UserDirectory finds registered users (user module, via an adapter).
type UserDirectory interface {
	// FindUserIDByEmail matches lower(btrim(email)) among non-deleted users.
	FindUserIDByEmail(ctx context.Context, email string) (userID string, found bool, err error)
}

// StaffLinkStore is the part of customer_links that staff linking needs.
type StaffLinkStore interface {
	FindByID(ctx context.Context, linkID string) (domain.CustomerLink, bool, error)
	HasLiveLinkForContact(ctx context.Context, organizationID, contactID string) (bool, error)
	// CreatePending inserts a pending_user link requested by staff (verified_via=staff);
	// domain.ErrCustomerLinkExists on a live-link conflict.
	CreatePending(ctx context.Context, link domain.CustomerLink, requestedBy string) (domain.CustomerLink, error)
	// ListLiveByContact returns pending_user/active links of the contact within the organization.
	ListLiveByContact(ctx context.Context, organizationID, contactID string) ([]domain.CustomerLink, error)
}

// LinkRevoker ends a live link (ClaimService).
type LinkRevoker interface {
	Revoke(ctx context.Context, actorUserID, linkID string, byStaff bool) error
}

// StaffLinkService lets tenant staff link a CRM contact to an existing user
// (trigger C). The link stays pending_user until that user accepts it.
type StaffLinkService struct {
	contacts ContactDirectory
	links    StaffLinkStore
	users    UserDirectory
	revoker  LinkRevoker
}

func NewStaffLinkService(contacts ContactDirectory, links StaffLinkStore, users UserDirectory, revoker LinkRevoker) *StaffLinkService {
	return &StaffLinkService{contacts: contacts, links: links, users: users, revoker: revoker}
}

// Request asks the user registered with userEmail to confirm they are the
// contact. An unknown email returns domain.ErrUserNotFound so staff switch to
// a claim invite. The email only locates the account; the link activates
// solely when that user accepts it.
func (s *StaffLinkService) Request(ctx context.Context, scope coretenant.Scope, actorUserID, contactID, userEmail string) (domain.CustomerLink, error) {
	organizationID := scope.OrganizationID()
	contactID = strings.TrimSpace(contactID)
	if _, ok, err := s.contacts.IsClaimable(ctx, organizationID, contactID); err != nil {
		return domain.CustomerLink{}, fmt.Errorf("check contact: %w", err)
	} else if !ok {
		return domain.CustomerLink{}, domain.ErrClaimNotAvailable
	}
	userID, found, err := s.users.FindUserIDByEmail(ctx, domain.NormalizeEmail(userEmail))
	if err != nil {
		return domain.CustomerLink{}, fmt.Errorf("find user: %w", err)
	}
	if !found {
		return domain.CustomerLink{}, domain.ErrUserNotFound
	}
	taken, err := s.links.HasLiveLinkForContact(ctx, organizationID, contactID)
	if err != nil {
		return domain.CustomerLink{}, fmt.Errorf("check live link: %w", err)
	}
	if taken {
		return domain.CustomerLink{}, domain.ErrCustomerLinkExists
	}
	return s.links.CreatePending(ctx, domain.CustomerLink{
		UserID:         userID,
		OrganizationID: organizationID,
		ContactID:      contactID,
	}, actorUserID)
}

// ListForContact returns the live links of a contact of the scoped organization.
func (s *StaffLinkService) ListForContact(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.CustomerLink, error) {
	return s.links.ListLiveByContact(ctx, scope.OrganizationID(), strings.TrimSpace(contactID))
}

// Revoke ends a live link of the scoped organization. A link of another
// organization looks exactly like an unknown one (no existence leak).
func (s *StaffLinkService) Revoke(ctx context.Context, scope coretenant.Scope, actorUserID, linkID string) error {
	link, found, err := s.links.FindByID(ctx, strings.TrimSpace(linkID))
	if err != nil {
		return fmt.Errorf("load customer link: %w", err)
	}
	if !found || link.OrganizationID != scope.OrganizationID() {
		return domain.ErrCustomerLinkNotFound
	}
	return s.revoker.Revoke(ctx, actorUserID, link.ID, true)
}
