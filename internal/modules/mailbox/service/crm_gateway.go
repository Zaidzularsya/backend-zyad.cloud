package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
)

// CRMEntities is what MessageService needs from CRM: check the lead/contact
// an email is sent from, and log the sent email on its timeline.
type CRMEntities interface {
	RequireEntity(ctx context.Context, scope coretenant.Scope, entityType domain.RelatedEntityType, id string) error
	RecordEmailActivity(ctx context.Context, scope coretenant.Scope, input EmailActivityInput) error
}

type EmailActivityInput struct {
	EntityType  domain.RelatedEntityType
	EntityID    string
	Subject     string
	Description string
	UserID      string
}

// CRMGateway adapts the CRM repositories; it is the only place this module
// touches CRM.
type CRMGateway struct {
	leads      crmrepo.LeadRepository
	contacts   crmrepo.ContactRepository
	activities crmrepo.ActivityRepository
}

func NewCRMGateway(leads crmrepo.LeadRepository, contacts crmrepo.ContactRepository, activities crmrepo.ActivityRepository) *CRMGateway {
	return &CRMGateway{leads: leads, contacts: contacts, activities: activities}
}

func (g *CRMGateway) RequireEntity(ctx context.Context, scope coretenant.Scope, entityType domain.RelatedEntityType, id string) error {
	var err error
	switch entityType {
	case domain.RelatedEntityLead:
		_, err = g.leads.FindByID(ctx, scope, id)
	case domain.RelatedEntityContact:
		_, err = g.contacts.FindByID(ctx, scope, id)
	default:
		return mailboxmodule.ErrEntityNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mailboxmodule.ErrEntityNotFound
	}
	return err
}

func (g *CRMGateway) RecordEmailActivity(ctx context.Context, scope coretenant.Scope, input EmailActivityInput) error {
	activity, err := g.activities.Create(ctx, scope, crmrepo.CreateActivityParams{
		RelatedEntityType: crmdomain.ActivityEntityType(input.EntityType),
		RelatedEntityID:   input.EntityID,
		Type:              crmdomain.ActivityTypeEmail,
		Subject:           input.Subject,
		Description:       input.Description,
		AssigneeUserID:    input.UserID,
		CreatedBy:         input.UserID,
	})
	if err != nil {
		return err
	}
	// The email was already sent, so the timeline entry is done, not a to-do.
	_, err = g.activities.Complete(ctx, scope, activity.ID, input.UserID)
	return err
}
