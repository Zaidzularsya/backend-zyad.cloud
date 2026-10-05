package service

import (
	"context"
	"testing"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type capturePublisher struct{ events []notificationpublisher.Event }

func (c *capturePublisher) Publish(_ context.Context, e notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	c.events = append(c.events, e)
	return notificationdomain.OutboxEvent{}, nil
}

type notifierDeals struct {
	repository.DealRepository
	owner string
}

func (d notifierDeals) FindByID(context.Context, coretenant.Scope, string) (domain.Deal, error) {
	return domain.Deal{ID: "d1", OwnerUserID: d.owner}, nil
}

type notifierMembers struct{ repository.MemberRepository }

func (notifierMembers) ListActive(context.Context, coretenant.Scope) ([]domain.OrganizationMember, error) {
	return []domain.OrganizationMember{
		{UserID: "u-owner", Name: "Sari", Email: "sari@example.com"},
		{UserID: "u-creator", Name: "Doni", Email: "doni@example.com"},
	}, nil
}

func newNotifier(owner string) (*QuotationNotifier, *capturePublisher) {
	pub := &capturePublisher{}
	return NewQuotationNotifier(pub, notifierDeals{owner: owner}, notifierMembers{}, "Zyad Cloud", "https://app.test/", "id-ID"), pub
}

func TestNotifierSendsRevisionToDealOwner(t *testing.T) {
	n, pub := newNotifier("u-owner")
	deal := "d1"
	q := domain.Quotation{ID: "q1", QuotationNumber: "QUO-1", DealID: &deal, CreatedBy: "u-creator"}
	err := n.Notify(context.Background(), coretenant.Scope{}, q, domain.QuotationResponse{
		Action: domain.QuotationResponseRevisionRequested, ResponderName: "Budi\r\nBcc: x",
		Categories: []domain.RevisionCategory{domain.RevisionPrice, domain.RevisionOther}, Note: "<b>mahal</b>",
	})
	if err != nil || len(pub.events) != 1 {
		t.Fatalf("err=%v events=%d", err, len(pub.events))
	}
	e := pub.events[0]
	if e.Type != "crm.quotation_revision_requested" || e.UserID != "u-owner" || e.Recipient.Email != "sari@example.com" || e.Locale != "id-ID" {
		t.Fatalf("event=%+v", e)
	}
	if e.Payload["categories"] != "Harga/diskon, Lainnya" || e.Payload["note"] != "<b>mahal</b>" ||
		e.Payload["deal_url"] != "https://app.test/app/crm/deals/d1" || e.Payload["responder_name"] != "Budi Bcc: x" ||
		e.Payload["user_name"] != "Sari" || e.Payload["quotation_number"] != "QUO-1" || e.Payload["app_name"] != "Zyad Cloud" {
		t.Fatalf("payload=%+v", e.Payload)
	}
}

func TestNotifierApprovedAndFallbackToCreator(t *testing.T) {
	n, pub := newNotifier("")
	q := domain.Quotation{ID: "q1", QuotationNumber: "QUO-1", CreatedBy: "u-creator"}
	if err := n.Notify(context.Background(), coretenant.Scope{}, q, domain.QuotationResponse{Action: domain.QuotationResponseApproved, ResponderName: "Budi"}); err != nil {
		t.Fatal(err)
	}
	e := pub.events[0]
	if e.Type != "crm.quotation_approved" || e.UserID != "u-creator" || e.Payload["deal_url"] != "https://app.test/app/crm/quotations/q1" {
		t.Fatalf("event=%+v", e)
	}
}
