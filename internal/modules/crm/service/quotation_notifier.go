package service

import (
	"context"
	"errors"
	"strings"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

const (
	EventQuotationApproved          = "crm.quotation_approved"
	EventQuotationRevisionRequested = "crm.quotation_revision_requested"
)

type NotificationPublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

// QuotationNotifier memberi tahu pemilik deal (atau pembuat quotation bila
// tanpa deal) saat customer merespons lewat link publik.
type QuotationNotifier struct {
	publisher   NotificationPublisher
	deals       repository.DealRepository
	members     repository.MemberRepository
	appName     string
	frontendURL string
	locale      string
}

func NewQuotationNotifier(publisher NotificationPublisher, deals repository.DealRepository, members repository.MemberRepository,
	appName, frontendURL, locale string) *QuotationNotifier {
	return &QuotationNotifier{publisher: publisher, deals: deals, members: members, appName: appName,
		frontendURL: strings.TrimRight(frontendURL, "/"), locale: locale}
}

func (n *QuotationNotifier) Notify(ctx context.Context, scope coretenant.Scope, q domain.Quotation, r domain.QuotationResponse) error {
	recipientID := q.CreatedBy
	url := n.frontendURL + "/app/crm/quotations/" + q.ID
	if q.DealID != nil {
		url = n.frontendURL + "/app/crm/deals/" + *q.DealID
		if deal, err := n.deals.FindByID(ctx, scope, *q.DealID); err == nil && deal.OwnerUserID != "" {
			recipientID = deal.OwnerUserID
		}
	}
	if recipientID == "" {
		return errors.New("quotation response: no recipient")
	}
	members, err := n.members.ListActive(ctx, scope)
	if err != nil {
		return err
	}
	var recipient *domain.OrganizationMember
	for i := range members {
		if members[i].UserID == recipientID {
			recipient = &members[i]
			break
		}
	}
	if recipient == nil || recipient.Email == "" {
		return errors.New("quotation response: recipient is not an active member with email")
	}

	// Nama responden masuk body email text/plain; buang CR/LF agar tidak
	// bisa menyisipkan baris palsu. Catatan dikirim apa adanya (bukan HTML).
	payload := map[string]any{
		"app_name": n.appName, "user_name": recipient.Name, "quotation_number": q.QuotationNumber,
		"responder_name": singleLine(r.ResponderName), "deal_url": url,
	}
	eventType := EventQuotationApproved
	if r.Action == domain.QuotationResponseRevisionRequested {
		eventType = EventQuotationRevisionRequested
		labels := make([]string, 0, len(r.Categories))
		for _, c := range r.Categories {
			labels = append(labels, c.Label())
		}
		payload["categories"] = strings.Join(labels, ", ")
		payload["note"] = r.Note
	}
	_, err = n.publisher.Publish(ctx, notificationpublisher.Event{
		Type: eventType, OrganizationID: scope.OrganizationID(), UserID: recipient.UserID,
		Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: recipient.UserID, Name: recipient.Name, Email: recipient.Email},
		Payload:   payload, Locale: n.locale,
	})
	return err
}

func singleLine(v string) string {
	return strings.Join(strings.Fields(v), " ")
}
