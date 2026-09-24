package service

import (
	"context"
	"errors"
	"strings"
	"time"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
)

// EventSessionDisconnected is the notification event (and template code)
// sent to organization owners when a WORKING session drops.
const EventSessionDisconnected = "whatsapp.session_disconnected"

type NotificationPublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

type OwnerLister interface {
	ListOrganizationOwners(ctx context.Context, organizationID string) ([]repository.Owner, error)
}

type DisconnectNotifierConfig struct {
	AppName     string
	FrontendURL string
	Locale      string
	MaxAttempts int
}

// OwnerDisconnectNotifier enqueues one email notification per organization
// owner through the notification outbox (delivered by the worker).
type OwnerDisconnectNotifier struct {
	owners    OwnerLister
	publisher NotificationPublisher
	config    DisconnectNotifierConfig
	location  *time.Location
}

func NewOwnerDisconnectNotifier(owners OwnerLister, publisher NotificationPublisher, config DisconnectNotifierConfig) *OwnerDisconnectNotifier {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		location = time.FixedZone("WIB", 7*60*60)
	}
	return &OwnerDisconnectNotifier{owners: owners, publisher: publisher, config: config, location: location}
}

func (n *OwnerDisconnectNotifier) NotifyDisconnected(ctx context.Context, session domain.Session, _ domain.SessionStatus) error {
	owners, err := n.owners.ListOrganizationOwners(ctx, session.OrganizationID)
	if err != nil {
		return err
	}

	occurredAt := time.Now().In(n.location)
	if session.LastStatusAt != nil {
		occurredAt = session.LastStatusAt.In(n.location)
	}
	payload := map[string]any{
		"app_name":        n.config.AppName,
		"session_name":    sessionLabel(session),
		"status":          statusLabel(session.Status),
		"occurred_at":     occurredAt.Format("2006-01-02 15:04") + " WIB",
		"connections_url": strings.TrimRight(n.config.FrontendURL, "/") + "/app/whatsapp",
	}

	var errs []error
	for _, owner := range owners {
		ownerPayload := make(map[string]any, len(payload)+1)
		for key, value := range payload {
			ownerPayload[key] = value
		}
		ownerPayload["user_name"] = owner.Name
		_, err := n.publisher.Publish(ctx, notificationpublisher.Event{
			Type:           EventSessionDisconnected,
			OrganizationID: session.OrganizationID,
			UserID:         owner.UserID,
			Recipient: notificationdomain.NotificationRecipient{
				Type:   "user",
				UserID: owner.UserID,
				Name:   owner.Name,
				Email:  owner.Email,
			},
			Payload:     ownerPayload,
			Locale:      n.config.Locale,
			MaxAttempts: n.config.MaxAttempts,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func sessionLabel(session domain.Session) string {
	parts := []string{}
	if session.DisplayName != "" {
		parts = append(parts, session.DisplayName)
	}
	if session.Phone != "" {
		parts = append(parts, "+"+session.Phone)
	}
	if len(parts) == 0 {
		return "WhatsApp"
	}
	return strings.Join(parts, " ")
}

func statusLabel(status domain.SessionStatus) string {
	switch status {
	case domain.SessionStatusScanQRCode:
		return "perangkat logout, perlu pairing ulang"
	case domain.SessionStatusStopped:
		return "berhenti"
	case domain.SessionStatusFailed:
		return "gagal"
	default:
		return string(status)
	}
}
