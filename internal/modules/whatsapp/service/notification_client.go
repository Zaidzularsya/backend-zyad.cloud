package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

// ErrNoNotificationSession means neither a connected platform session with
// purpose "notification" nor WHATSAPP_API_SESSION is available.
var ErrNoNotificationSession = errors.New("whatsapp: no connected notification session")

type TextSender interface {
	SendText(ctx context.Context, message platformwhatsapp.Message) (platformwhatsapp.Result, error)
}

// NotificationClient implements platformwhatsapp.Client for the notification
// WhatsAppDispatcher. It sends from the platform organization's connected
// session with purpose "notification" (the default one first), falling back
// to WHATSAPP_API_SESSION. Tenant sessions are never used for platform
// notifications.
type NotificationClient struct {
	sender          TextSender
	sessions        repository.SessionRepository
	resolver        WorkerTenantResolver
	platformOrgID   string
	fallbackSession string
	log             *slog.Logger
}

var _ platformwhatsapp.Client = (*NotificationClient)(nil)

func NewNotificationClient(
	sender TextSender,
	sessions repository.SessionRepository,
	resolver WorkerTenantResolver,
	platformOrgID string,
	fallbackSession string,
	log *slog.Logger,
) *NotificationClient {
	if log == nil {
		log = slog.Default()
	}
	return &NotificationClient{
		sender:          sender,
		sessions:        sessions,
		resolver:        resolver,
		platformOrgID:   strings.TrimSpace(platformOrgID),
		fallbackSession: strings.TrimSpace(fallbackSession),
		log:             log,
	}
}

func (c *NotificationClient) SendText(ctx context.Context, message platformwhatsapp.Message) (platformwhatsapp.Result, error) {
	if strings.TrimSpace(message.Session) == "" {
		session, err := c.notificationSession(ctx)
		if err != nil {
			return platformwhatsapp.Result{}, err
		}
		message.Session = session
	}
	return c.sender.SendText(ctx, message)
}

func (c *NotificationClient) notificationSession(ctx context.Context) (string, error) {
	if c.platformOrgID != "" && c.sessions != nil && c.resolver != nil {
		name, err := c.platformSession(ctx)
		if err != nil {
			c.log.Warn("whatsapp: resolve platform notification session failed", "error", err)
		}
		if name != "" {
			return name, nil
		}
	}
	if c.fallbackSession != "" {
		return c.fallbackSession, nil
	}
	return "", ErrNoNotificationSession
}

func (c *NotificationClient) platformSession(ctx context.Context) (string, error) {
	tenantContext, err := c.resolver.ResolveWorkerOrganization(ctx, c.platformOrgID, WorkerIdentity)
	if err != nil {
		return "", err
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		return "", err
	}
	sessions, err := c.sessions.List(ctx, scope)
	if err != nil {
		return "", err
	}
	name := ""
	for _, session := range sessions {
		if session.Purpose != domain.SessionPurposeNotification || session.Status != domain.SessionStatusWorking {
			continue
		}
		if session.IsDefault {
			return session.Name, nil
		}
		if name == "" {
			name = session.Name
		}
	}
	return name, nil
}
