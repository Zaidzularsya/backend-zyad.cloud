package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

// LeadSourceWhatsApp is the crm_leads.source of leads auto-created from an
// inbound WhatsApp message.
const LeadSourceWhatsApp = "whatsapp"

// CRMMatch is the CRM record a WhatsApp number belongs to.
type CRMMatch struct {
	EntityType  domain.RelatedEntityType
	EntityID    string
	Name        string
	OwnerUserID string
}

type CreateLeadInput struct {
	ContactName string
	Phone       string
	OwnerUserID string
}

// CRMMatcher links WhatsApp numbers to CRM leads/contacts.
type CRMMatcher interface {
	// MatchPhone finds a non-converted lead, then a contact, by normalized
	// phone. found is false when neither matches.
	MatchPhone(ctx context.Context, scope coretenant.Scope, phoneNormalized string) (match CRMMatch, found bool, err error)
	CreateLead(ctx context.Context, scope coretenant.Scope, input CreateLeadInput) (CRMMatch, error)
}

type crmRepositoryMatcher struct {
	leads    crmrepo.LeadRepository
	contacts crmrepo.ContactRepository
}

func NewCRMRepositoryMatcher(leads crmrepo.LeadRepository, contacts crmrepo.ContactRepository) CRMMatcher {
	return &crmRepositoryMatcher{leads: leads, contacts: contacts}
}

func (m *crmRepositoryMatcher) MatchPhone(ctx context.Context, scope coretenant.Scope, phoneNormalized string) (CRMMatch, bool, error) {
	if phoneNormalized == "" {
		return CRMMatch{}, false, nil
	}

	lead, err := m.leads.FindActiveByPhone(ctx, scope, phoneNormalized)
	if err == nil {
		return leadMatch(lead), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CRMMatch{}, false, err
	}

	contact, err := m.contacts.FindActiveByPhone(ctx, scope, phoneNormalized)
	if err == nil {
		return CRMMatch{
			EntityType:  domain.RelatedEntityContact,
			EntityID:    contact.ID,
			Name:        strings.TrimSpace(contact.FirstName + " " + contact.LastName),
			OwnerUserID: contact.OwnerUserID,
		}, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return CRMMatch{}, false, nil
	}
	return CRMMatch{}, false, err
}

func (m *crmRepositoryMatcher) CreateLead(ctx context.Context, scope coretenant.Scope, input CreateLeadInput) (CRMMatch, error) {
	lead, err := m.leads.Create(ctx, scope, crmrepo.CreateLeadParams{
		ContactName: input.ContactName,
		Phone:       input.Phone,
		Source:      LeadSourceWhatsApp,
		OwnerUserID: input.OwnerUserID,
		CreatedBy:   input.OwnerUserID,
	})
	if err != nil {
		return CRMMatch{}, err
	}
	return leadMatch(lead), nil
}

func leadMatch(lead crmdomain.Lead) CRMMatch {
	return CRMMatch{
		EntityType:  domain.RelatedEntityLead,
		EntityID:    lead.ID,
		Name:        lead.ContactName,
		OwnerUserID: lead.OwnerUserID,
	}
}
