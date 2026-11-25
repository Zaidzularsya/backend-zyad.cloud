package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/organization/model"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
)

type payoutOwnersFake struct {
	list []crmdomain.OrganizationMember
}

func (f payoutOwnersFake) ListOwners(context.Context, string) ([]crmdomain.OrganizationMember, error) {
	return f.list, nil
}

type payoutOrgsFake struct{}

func (payoutOrgsFake) FindByID(context.Context, string) (model.Organization, error) {
	return model.Organization{Name: "PT Contoh"}, nil
}

type payoutPublisherFake struct {
	events []notificationpublisher.Event
	err    error
}

func (f *payoutPublisherFake) Publish(_ context.Context, e notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	f.events = append(f.events, e)
	return notificationdomain.OutboxEvent{}, f.err
}

func TestPayoutNotifierPublishesMaskedPayloadPerOwner(t *testing.T) {
	pub := &payoutPublisherFake{}
	n := payoutNotifier{
		owners:      payoutOwnersFake{list: []crmdomain.OrganizationMember{{UserID: "u1", Name: "A", Email: "a@x.test"}, {UserID: "u2", Name: "B", Email: "b@x.test"}}},
		orgs:        payoutOrgsFake{},
		publisher:   pub,
		appName:     "Zyad",
		frontendURL: "https://app.test/",
		locale:      "id-ID",
	}
	n.NotifyPayoutAccount(context.Background(), "receivable.payout_account_rejected", receivablerepo.PayoutAccount{
		OrganizationID: "o1", BankName: "BCA", AccountNumber: "1234567890", AccountHolderName: "Budi", ReviewNote: "salah\nnama",
	})
	if len(pub.events) != 2 || pub.events[0].Type != "receivable.payout_account_rejected" {
		t.Fatalf("events = %+v", pub.events)
	}
	p := pub.events[0].Payload
	if p["account_number_masked"] != "••••7890" || p["organization_name"] != "PT Contoh" || p["settlement_url"] != "https://app.test/app/account/bank" || p["review_note"] != "salah nama" {
		t.Fatalf("payload = %+v", p)
	}
	for _, v := range p {
		if s, ok := v.(string); ok && strings.Contains(s, "1234567890") {
			t.Fatal("nomor rekening utuh bocor ke payload")
		}
	}
}

func TestPayoutNotifierPublishFailureDoesNotPanic(t *testing.T) {
	pub := &payoutPublisherFake{err: errors.New("down")}
	n := payoutNotifier{owners: payoutOwnersFake{list: []crmdomain.OrganizationMember{{UserID: "u1", Email: "a@x.test"}}}, orgs: payoutOrgsFake{}, publisher: pub}
	n.NotifyPayoutAccount(context.Background(), "receivable.payout_account_verified", receivablerepo.PayoutAccount{OrganizationID: "o1"})
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
}

type payoutOrgsFailingFake struct{}

func (payoutOrgsFailingFake) FindByID(context.Context, string) (model.Organization, error) {
	return model.Organization{}, errors.New("db down")
}

func TestPayoutNotifierFallsBackWhenOrganizationLookupFails(t *testing.T) {
	pub := &payoutPublisherFake{}
	n := payoutNotifier{owners: payoutOwnersFake{list: []crmdomain.OrganizationMember{{UserID: "u1", Email: "a@x.test"}}}, orgs: payoutOrgsFailingFake{}, publisher: pub}
	n.NotifyPayoutAccount(context.Background(), "receivable.payout_account_verified", receivablerepo.PayoutAccount{OrganizationID: "o1"})
	if len(pub.events) != 1 || pub.events[0].Payload["organization_name"] != "-" {
		t.Fatalf("events = %+v, want organization_name \"-\"", pub.events)
	}
}
