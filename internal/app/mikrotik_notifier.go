package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"zyad.cloud/internal/core/businesstime"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikdomain "zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/docpdf"
)

// Narrow views of the receivable services; the mikrotik module itself never
// imports receivable, only this wiring layer does.
type mikrotikNotifSettings interface {
	Get(ctx context.Context, scope coretenant.Scope) (receivabledomain.Settings, error)
}

type mikrotikNotifAccounts interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (receivabledomain.Account, error)
}

type payLinker interface {
	Link(ctx context.Context, scope coretenant.Scope, invoiceID, senderUserID string) (string, time.Time, error)
}

type mikrotikNotifIssuer interface {
	OrganizationProfile(ctx context.Context, scope coretenant.Scope) (docpdf.Party, error)
}

// mikrotikCustomerNotifier implements service.CustomerNotifier over the
// receivable email/WhatsApp senders, using the receivable default sender.
type mikrotikCustomerNotifier struct {
	settings mikrotikNotifSettings
	accounts mikrotikNotifAccounts
	links    payLinker
	email    receivableservice.EmailSender
	whatsapp receivableservice.WhatsAppSender
	issuer   mikrotikNotifIssuer
	now      func() time.Time
}

var _ service.CustomerNotifier = (*mikrotikCustomerNotifier)(nil)

func (n *mikrotikCustomerNotifier) NotifyIsolated(ctx context.Context, scope coretenant.Scope, accountID string, invoices []service.TriggerInvoice) error {
	return n.notify(ctx, scope, accountID, "mikrotik-isolated", func(sender, tenant string, acc receivabledomain.Account) (string, string, string) {
		links := make(map[string]string, len(invoices))
		for _, inv := range invoices {
			link, _, err := n.links.Link(ctx, scope, inv.ID, sender)
			if err != nil {
				slog.WarnContext(ctx, "mikrotik: pay link unavailable, notifying without it", "invoice_id", inv.ID, "error", err)
				continue
			}
			links[inv.ID] = link
		}
		return service.IsolatedMessage(tenant, acc.Name, invoices, links)
	})
}

func (n *mikrotikCustomerNotifier) NotifyRestored(ctx context.Context, scope coretenant.Scope, accountID string) error {
	return n.notify(ctx, scope, accountID, "mikrotik-restored", func(_, tenant string, acc receivabledomain.Account) (string, string, string) {
		return service.RestoredMessage(tenant, acc.Name)
	})
}

// notify tries email then WhatsApp. Unavailable channels (no sender, no
// address, ChannelError) are skipped; real send errors are joined and returned
// only after both channels were tried.
func (n *mikrotikCustomerNotifier) notify(ctx context.Context, scope coretenant.Scope, accountID, kind string,
	build func(sender, tenant string, acc receivabledomain.Account) (text, htmlBody, subject string)) error {
	settings, err := n.settings.Get(ctx, scope)
	if err != nil {
		return err
	}
	sender := strings.TrimSpace(settings.DefaultSenderUserID)
	if sender == "" {
		return nil
	}
	acc, err := n.accounts.Get(ctx, scope, accountID)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(acc.Email)
	contactID := acc.ContactID()
	if to == "" && contactID == "" {
		return nil
	}
	tenant := ""
	if n.issuer != nil {
		if party, err := n.issuer.OrganizationProfile(ctx, scope); err == nil {
			tenant = party.Name
		} else {
			slog.DebugContext(ctx, "mikrotik: organization profile unavailable, notifying without tenant name", "error", err)
		}
	}
	text, body, subject := build(sender, tenant, acc)

	var errs []error
	if to != "" && n.email != nil {
		// Stable per (account, kind, WIB day): a replayed task must not double-send.
		requestID := kind + ":" + accountID + ":" + n.now().In(businesstime.Location()).Format("20060102")
		_, err := n.email.SendInvoiceEmail(ctx, scope, receivableservice.EmailInput{
			SenderUserID: sender, To: to, Subject: subject, BodyHTML: body, ContactID: contactID, ClientRequestID: requestID,
		})
		if err != nil && !errors.Is(err, receivableservice.ErrChannelUnavailable) {
			errs = append(errs, err)
		}
	}
	if contactID != "" && n.whatsapp != nil {
		_, _, err := n.whatsapp.SendInvoiceWhatsApp(ctx, scope, receivableservice.WhatsAppInput{
			SenderUserID: sender, ContactID: contactID, Text: text,
		})
		if err != nil && !errors.Is(err, receivableservice.ErrChannelUnavailable) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type mikrotikAlertMembers interface {
	WithPermission(ctx context.Context, scope coretenant.Scope, permission string) ([]receivableservice.Member, error)
}

// mikrotikTenantAlerter implements service.TenantAlerter by publishing an
// outbox event per member holding mikrotik_task.read. The payload is limited
// to fixed, credential-free fields (never router host/username/password).
type mikrotikTenantAlerter struct {
	members     mikrotikAlertMembers
	publisher   notificationPublisher
	appName     string
	frontendURL string
	locale      string
}

var _ service.TenantAlerter = mikrotikTenantAlerter{}

func (a mikrotikTenantAlerter) NotifyTaskFailed(ctx context.Context, scope coretenant.Scope, e service.TaskFailureEvent) error {
	members, err := a.members.WithPermission(ctx, scope, "mikrotik_task.read")
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range members {
		_, err := a.publisher.Publish(ctx, notificationpublisher.Event{
			Type: "mikrotik.isolation_task_failed", OrganizationID: scope.OrganizationID(), UserID: m.UserID,
			Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: m.UserID, Name: m.Name, Email: m.Email},
			Payload: map[string]any{
				"app_name": a.appName, "user_name": m.Name, "router_name": singleLine(e.RouterName),
				"secret_name": singleLine(e.SecretName), "action": actionLabel(e.Action), "reason": singleLine(e.Reason),
				"tasks_url": strings.TrimRight(a.frontendURL, "/") + "/app/mikrotik/tasks",
			},
			Locale: a.locale,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// actionLabel maps the internal task action to the label the notification
// template expects ("Tugas {{action}} PPPoE ...").
func actionLabel(action string) string {
	switch action {
	case mikrotikdomain.ActionIsolate:
		return "isolir"
	case mikrotikdomain.ActionRestore:
		return "pulihkan"
	}
	return action
}
