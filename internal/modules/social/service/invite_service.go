package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social/domain"
)

// inviteTokenBytes is the entropy of a claim invite token (same as password reset tokens).
const inviteTokenBytes = 32

// InviteStore persists customer_claim_invites.
type InviteStore interface {
	// Create stores the invite and marks open invites of the same contact used.
	Create(ctx context.Context, invite domain.ClaimInvite) (domain.ClaimInvite, error)
	FindByTokenHash(ctx context.Context, tokenHash string) (domain.ClaimInvite, bool, error)
	// Accept atomically consumes the still-usable invite and creates the active
	// link (verified_via=invite) with its audit row. domain.ErrInviteInvalid when
	// the invite is no longer usable; domain.ErrCustomerLinkExists on a live-link
	// conflict (the invite then stays usable).
	Accept(ctx context.Context, tokenHash, userID string, now time.Time) (domain.CustomerLink, error)
}

// ClaimInviteEmail is the invite message to a CRM contact.
type ClaimInviteEmail struct {
	OrganizationID   string
	OrganizationName string
	ContactName      string
	Email            string
	ClaimURL         string
	ExpiresAt        time.Time
}

// ClaimInviteSender delivers the invite email (notification outbox).
type ClaimInviteSender interface {
	SendClaimInvite(ctx context.Context, msg ClaimInviteEmail) error
}

// EmailVerifier marks a user's email verified (user module).
type EmailVerifier interface {
	MarkEmailVerified(ctx context.Context, userID string, at time.Time) error
}

// InviteService lets tenant staff invite a CRM contact to claim it (trigger B).
// The emailed token is the proof of email ownership.
type InviteService struct {
	contacts     ContactDirectory
	links        CustomerLinkStore
	invites      InviteStore
	sender       ClaimInviteSender
	verifier     EmailVerifier
	attribution  CustomerAttribution
	claimURLBase string
	now          func() time.Time
	log          *slog.Logger
}

func NewInviteService(
	contacts ContactDirectory,
	links CustomerLinkStore,
	invites InviteStore,
	sender ClaimInviteSender,
	verifier EmailVerifier,
	attribution CustomerAttribution,
	claimURLBase string,
) *InviteService {
	return &InviteService{
		contacts:     contacts,
		links:        links,
		invites:      invites,
		sender:       sender,
		verifier:     verifier,
		attribution:  attribution,
		claimURLBase: strings.TrimRight(strings.TrimSpace(claimURLBase), "/"),
		now:          func() time.Time { return time.Now().UTC() },
		log:          slog.Default(),
	}
}

// Send emails a claim invite to a customer contact of the scoped organization.
// A resend supersedes the previous invite. The organization always comes from
// the verified tenant scope.
func (s *InviteService) Send(ctx context.Context, scope coretenant.Scope, actorUserID, contactID string) error {
	organizationID := scope.OrganizationID()
	contactID = strings.TrimSpace(contactID)
	contact, ok, err := s.contacts.IsClaimable(ctx, organizationID, contactID)
	if err != nil {
		return fmt.Errorf("check contact: %w", err)
	}
	if !ok {
		return domain.ErrClaimNotAvailable
	}
	email := domain.NormalizeEmail(contact.Email)
	if email == "" {
		return domain.ErrContactEmailMissing
	}
	taken, err := s.links.HasLiveLinkForContact(ctx, organizationID, contactID)
	if err != nil {
		return fmt.Errorf("check live link: %w", err)
	}
	if taken {
		return domain.ErrCustomerLinkExists
	}

	token, err := coreauth.NewRandomToken(inviteTokenBytes)
	if err != nil {
		return fmt.Errorf("create invite token: %w", err)
	}
	now := s.now().UTC()
	invite, err := s.invites.Create(ctx, domain.ClaimInvite{
		OrganizationID: organizationID,
		ContactID:      contactID,
		Email:          email,
		TokenHash:      coreauth.HashToken(token),
		ExpiresAt:      now.Add(domain.InviteTTL),
		CreatedBy:      actorUserID,
		CreatedAt:      now,
	})
	if err != nil {
		return fmt.Errorf("store claim invite: %w", err)
	}
	// A failed enqueue is returned so staff can resend; the next send supersedes this invite.
	return s.sender.SendClaimInvite(ctx, ClaimInviteEmail{
		OrganizationID:   organizationID,
		OrganizationName: contact.OrganizationName,
		ContactName:      contact.ContactName,
		Email:            email,
		ClaimURL:         s.claimURLBase + "/claim?token=" + url.QueryEscape(token),
		ExpiresAt:        invite.ExpiresAt,
	})
}

// Accept links the user to the invited contact (verified_via=invite). An
// unverified account may accept; when its email equals the invited address the
// token proves ownership, so the email is marked verified.
func (s *InviteService) Accept(ctx context.Context, user AuthUser, token string) (domain.CustomerLink, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.CustomerLink{}, domain.ErrInviteInvalid
	}
	tokenHash := coreauth.HashToken(token)
	invite, found, err := s.invites.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return domain.CustomerLink{}, fmt.Errorf("load claim invite: %w", err)
	}
	if !found {
		return domain.CustomerLink{}, domain.ErrInviteInvalid
	}
	now := s.now().UTC()
	if err := invite.Check(now); err != nil {
		return domain.CustomerLink{}, err
	}
	if _, ok, err := s.contacts.IsClaimable(ctx, invite.OrganizationID, invite.ContactID); err != nil {
		return domain.CustomerLink{}, fmt.Errorf("check contact: %w", err)
	} else if !ok {
		return domain.CustomerLink{}, domain.ErrClaimNotAvailable
	}

	link, err := s.invites.Accept(ctx, tokenHash, user.ID, now)
	if err != nil {
		return domain.CustomerLink{}, err
	}

	// The link is committed; marking the email verified is best-effort for the
	// same reason as attribution: the claim itself succeeded.
	if !user.EmailVerified && s.verifier != nil && domain.NormalizeEmail(user.Email) == domain.NormalizeEmail(invite.Email) {
		if err := s.verifier.MarkEmailVerified(ctx, user.ID, now); err != nil {
			s.log.WarnContext(ctx, "mark email verified from claim invite failed", "user_id", user.ID, "error", err)
		}
	}
	recordBecameCustomer(ctx, s.attribution, s.log, user.ID)
	return link, nil
}
