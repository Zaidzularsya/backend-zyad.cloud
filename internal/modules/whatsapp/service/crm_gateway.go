package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
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

// CRMEntity is the CRM record a conversation is started from.
type CRMEntity struct {
	Type        domain.RelatedEntityType
	ID          string
	Name        string
	Phone       string
	OwnerUserID string
}

type CRMActivityInput struct {
	EntityType  domain.RelatedEntityType
	EntityID    string
	Subject     string
	Description string
	UserID      string
}

// CRMEntities is what ConversationService needs from CRM.
type CRMEntities interface {
	// FindEntity returns the lead/contact, or pgx.ErrNoRows.
	FindEntity(ctx context.Context, scope coretenant.Scope, entityType domain.RelatedEntityType, id string) (CRMEntity, error)
	// RecordActivity writes a completed 'whatsapp' timeline activity.
	RecordActivity(ctx context.Context, scope coretenant.Scope, input CRMActivityInput) error
	IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error)
}

// CRMGateway adapts the CRM repositories for the whatsapp module (matching,
// conversation start, timeline activity). It is the only place the module
// touches CRM.
type CRMGateway struct {
	leads      crmrepo.LeadRepository
	contacts   crmrepo.ContactRepository
	activities crmrepo.ActivityRepository
	members    crmrepo.MemberRepository
}

var (
	_ CRMMatcher  = (*CRMGateway)(nil)
	_ CRMEntities = (*CRMGateway)(nil)
)

func NewCRMGateway(
	leads crmrepo.LeadRepository,
	contacts crmrepo.ContactRepository,
	activities crmrepo.ActivityRepository,
	members crmrepo.MemberRepository,
) *CRMGateway {
	return &CRMGateway{leads: leads, contacts: contacts, activities: activities, members: members}
}

func (m *CRMGateway) MatchPhone(ctx context.Context, scope coretenant.Scope, phoneNormalized string) (CRMMatch, bool, error) {
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

func (m *CRMGateway) CreateLead(ctx context.Context, scope coretenant.Scope, input CreateLeadInput) (CRMMatch, error) {
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

func (m *CRMGateway) FindEntity(ctx context.Context, scope coretenant.Scope, entityType domain.RelatedEntityType, id string) (CRMEntity, error) {
	switch entityType {
	case domain.RelatedEntityLead:
		lead, err := m.leads.FindByID(ctx, scope, id)
		if err != nil {
			return CRMEntity{}, err
		}
		return CRMEntity{Type: entityType, ID: lead.ID, Name: lead.ContactName, Phone: lead.Phone, OwnerUserID: lead.OwnerUserID}, nil
	case domain.RelatedEntityContact:
		contact, err := m.contacts.FindByID(ctx, scope, id)
		if err != nil {
			return CRMEntity{}, err
		}
		return CRMEntity{
			Type: entityType, ID: contact.ID, Name: strings.TrimSpace(contact.FirstName + " " + contact.LastName),
			Phone: contact.Phone, OwnerUserID: contact.OwnerUserID,
		}, nil
	default:
		return CRMEntity{}, pgx.ErrNoRows
	}
}

func (m *CRMGateway) RecordActivity(ctx context.Context, scope coretenant.Scope, input CRMActivityInput) error {
	activity, err := m.activities.Create(ctx, scope, crmrepo.CreateActivityParams{
		RelatedEntityType: crmdomain.ActivityEntityType(input.EntityType),
		RelatedEntityID:   input.EntityID,
		Type:              crmdomain.ActivityTypeWhatsApp,
		Subject:           input.Subject,
		Description:       input.Description,
		AssigneeUserID:    input.UserID,
		CreatedBy:         input.UserID,
	})
	if err != nil {
		return err
	}
	// A chat already happened, so the timeline entry is completed, not a to-do.
	_, err = m.activities.Complete(ctx, scope, activity.ID, input.UserID)
	return err
}

func (m *CRMGateway) IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error) {
	return m.members.IsActiveMember(ctx, scope, userID)
}

// ActivityRecorder is the narrow part of CRMEntities/InboundCRM
// claimAndRecordDailyActivity needs.
type ActivityRecorder interface {
	RecordActivity(ctx context.Context, scope coretenant.Scope, input CRMActivityInput) error
}

// claimAndRecordDailyActivity writes at most one 'whatsapp' CRM activity per
// conversation per calendar day (the actual boundary/dedupe is
// repository.ConversationRepository.ClaimActivityDay, keyed on
// wa_conversations.crm_activity_on), regardless of how many messages were
// exchanged that day or which direction they went. Both ConversationService
// (chat started or sent from the app) and InboundProcessor (message arrived
// via webhook, inbound or sent from the phone) call this, so a busy
// back-and-forth never spams the lead's timeline. now must already be in the
// timezone the day boundary is measured in (Asia/Jakarta).
func claimAndRecordDailyActivity(
	ctx context.Context,
	scope coretenant.Scope,
	conversations repository.ConversationRepository,
	crm ActivityRecorder,
	now time.Time,
	conversation domain.Conversation,
	session domain.Session,
	userID string,
	log *slog.Logger,
) {
	if conversation.RelatedEntityID == "" || crm == nil {
		return
	}
	claimed, err := conversations.ClaimActivityDay(ctx, scope, conversation.ID, now)
	if err != nil || !claimed {
		if err != nil {
			log.Warn("whatsapp: claim activity day failed", "conversation_id", conversation.ID, "error", err)
		}
		return
	}

	number := "+" + conversation.PhoneNormalized
	via := session.DisplayName
	if via == "" && session.Phone != "" {
		via = "+" + session.Phone
	}
	description := fmt.Sprintf("Percakapan WhatsApp dengan %s", number)
	if via != "" {
		description += " melalui " + via
	}
	if err := crm.RecordActivity(ctx, scope, CRMActivityInput{
		EntityType:  conversation.RelatedEntityType,
		EntityID:    conversation.RelatedEntityID,
		Subject:     "Chat WhatsApp " + number,
		Description: description,
		UserID:      userID,
	}); err != nil {
		log.Warn("whatsapp: record crm activity failed", "conversation_id", conversation.ID, "error", err)
	}
}
