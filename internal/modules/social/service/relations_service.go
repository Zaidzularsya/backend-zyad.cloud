package service

import (
	"context"
	"fmt"
)

// Relation sources.
const (
	// RelationSourceCheckout: the user owns a workspace bought through a
	// tenant's (or the platform's) checkout, linked to a CRM company there.
	RelationSourceCheckout = "checkout"
	// RelationSourceClaim: the user claimed a CRM contact through customer_links.
	RelationSourceClaim = "claim"
)

const customerLinkStatusActive = "active"

// CustomerRelation is one organization the user is a customer of.
type CustomerRelation struct {
	OrganizationID   string
	OrganizationName string
	Source           string
	// CustomerLinkID is set only for Source == "claim".
	CustomerLinkID string
}

// ClaimedLink is a customer_links row of the user, any status.
type ClaimedLink struct {
	ID               string
	OrganizationID   string
	OrganizationName string
	ContactID        string
	Status           string
}

// CheckoutRelationSource lists relations from workspaces the user owns.
type CheckoutRelationSource interface {
	CheckoutRelations(ctx context.Context, userID string) ([]CustomerRelation, error)
}

// ClaimedLinkSource lists the user's customer links.
type ClaimedLinkSource interface {
	ClaimedLinks(ctx context.Context, userID string) ([]ClaimedLink, error)
}

// ContactStatusChecker reports whether a CRM contact still exists (not soft-deleted)
// in its organization. Implementations must read it inside that tenant's scope.
type ContactStatusChecker interface {
	ContactActive(ctx context.Context, organizationID, contactID string) (bool, error)
}

// RelationsService is the read model "which organizations am I a customer of".
type RelationsService struct {
	checkout CheckoutRelationSource
	links    ClaimedLinkSource
	contacts ContactStatusChecker
}

func NewRelationsService(checkout CheckoutRelationSource, links ClaimedLinkSource, contacts ContactStatusChecker) *RelationsService {
	return &RelationsService{checkout: checkout, links: links, contacts: contacts}
}

// ForUser returns checkout relations followed by active claims whose contact
// still exists. The same organization may appear once per source.
func (s *RelationsService) ForUser(ctx context.Context, userID string) ([]CustomerRelation, error) {
	checkout, err := s.checkout.CheckoutRelations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list checkout relations: %w", err)
	}
	relations := make([]CustomerRelation, 0, len(checkout))
	seen := make(map[string]bool, len(checkout))
	for _, r := range checkout {
		if seen[r.OrganizationID] {
			continue
		}
		seen[r.OrganizationID] = true
		r.Source = RelationSourceCheckout
		r.CustomerLinkID = ""
		relations = append(relations, r)
	}

	links, err := s.links.ClaimedLinks(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list customer links: %w", err)
	}
	for _, link := range links {
		if link.Status != customerLinkStatusActive {
			continue
		}
		active, err := s.contacts.ContactActive(ctx, link.OrganizationID, link.ContactID)
		if err != nil {
			return nil, fmt.Errorf("check contact of customer link %s: %w", link.ID, err)
		}
		if !active {
			continue
		}
		relations = append(relations, CustomerRelation{
			OrganizationID:   link.OrganizationID,
			OrganizationName: link.OrganizationName,
			Source:           RelationSourceClaim,
			CustomerLinkID:   link.ID,
		})
	}
	return relations, nil
}
