package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/docpdf"
)

type fakeNotifSettings struct{ sender string }

func (f fakeNotifSettings) Get(context.Context, coretenant.Scope) (receivabledomain.Settings, error) {
	return receivabledomain.Settings{DefaultSenderUserID: f.sender}, nil
}

type fakeNotifAccounts struct{ acc receivabledomain.Account }

func (f fakeNotifAccounts) Get(context.Context, coretenant.Scope, string) (receivabledomain.Account, error) {
	return f.acc, nil
}

type fakePayLinker struct{ calls []string }

func (f *fakePayLinker) Link(_ context.Context, _ coretenant.Scope, invoiceID, _ string) (string, time.Time, error) {
	f.calls = append(f.calls, invoiceID)
	return "https://pay.example/" + invoiceID, time.Time{}, nil
}

type fakeEmail struct {
	in  []receivableservice.EmailInput
	err error
}

func (f *fakeEmail) SendInvoiceEmail(_ context.Context, _ coretenant.Scope, in receivableservice.EmailInput) (string, error) {
	f.in = append(f.in, in)
	return "e1", f.err
}

type fakeWA struct {
	in []receivableservice.WhatsAppInput
}

func (f *fakeWA) SendInvoiceWhatsApp(_ context.Context, _ coretenant.Scope, in receivableservice.WhatsAppInput) (string, string, error) {
	f.in = append(f.in, in)
	return "w1", "628", nil
}

type fakeNotifIssuer struct{}

func (fakeNotifIssuer) OrganizationProfile(context.Context, coretenant.Scope) (docpdf.Party, error) {
	return docpdf.Party{Name: "ISP Jaya"}, nil
}

func newTestNotifier(sender string, acc receivabledomain.Account, em *fakeEmail, wa *fakeWA) (*mikrotikCustomerNotifier, *fakePayLinker) {
	pl := &fakePayLinker{}
	return &mikrotikCustomerNotifier{
		settings: fakeNotifSettings{sender: sender}, accounts: fakeNotifAccounts{acc: acc}, links: pl,
		email: em, whatsapp: wa, issuer: fakeNotifIssuer{},
		now: func() time.Time { return time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC) },
	}, pl
}

var testInvoices = []service.TriggerInvoice{{ID: "i1", Number: "INV-1", DueDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}}

func TestCustomerNotifier_NoSenderConfigured_SkipsWithoutError(t *testing.T) {
	em, wa := &fakeEmail{}, &fakeWA{}
	n, pl := newTestNotifier("", receivabledomain.Account{Name: "A", Email: "a@x.id", SourceType: receivabledomain.SourceTypeCRMContact, SourceID: "c1"}, em, wa)
	if err := n.NotifyIsolated(context.Background(), mkScope("org-1"), "acc-1", testInvoices); err != nil {
		t.Fatal(err)
	}
	if err := n.NotifyRestored(context.Background(), mkScope("org-1"), "acc-1"); err != nil {
		t.Fatal(err)
	}
	if len(em.in)+len(wa.in)+len(pl.calls) != 0 {
		t.Fatal("nothing should be sent or linked without a sender")
	}
}

func TestCustomerNotifier_NoEmailNoPhone_Skips(t *testing.T) {
	em, wa := &fakeEmail{}, &fakeWA{}
	n, _ := newTestNotifier("u1", receivabledomain.Account{Name: "A"}, em, wa)
	if err := n.NotifyIsolated(context.Background(), mkScope("org-1"), "acc-1", testInvoices); err != nil {
		t.Fatal(err)
	}
	if len(em.in)+len(wa.in) != 0 {
		t.Fatal("no channel should be attempted")
	}
}

func TestCustomerNotifier_SendsEmailAndWhatsApp(t *testing.T) {
	em, wa := &fakeEmail{}, &fakeWA{}
	n, _ := newTestNotifier("u1", receivabledomain.Account{Name: "Budi", Email: "b@x.id", SourceType: receivabledomain.SourceTypeCRMContact, SourceID: "c1"}, em, wa)
	if err := n.NotifyIsolated(context.Background(), mkScope("org-1"), "acc-1", testInvoices); err != nil {
		t.Fatal(err)
	}
	if len(em.in) != 1 || len(wa.in) != 1 {
		t.Fatalf("email=%d wa=%d", len(em.in), len(wa.in))
	}
	e := em.in[0]
	if e.To != "b@x.id" || e.SenderUserID != "u1" || e.ContactID != "c1" || e.Attachment != nil ||
		e.ClientRequestID != "mikrotik-isolated:acc-1:20261031" || !strings.Contains(e.BodyHTML, "https://pay.example/i1") {
		t.Fatalf("email input: %+v", e)
	}
	if wa.in[0].ContactID != "c1" || !strings.Contains(wa.in[0].Text, "INV-1") {
		t.Fatalf("wa input: %+v", wa.in[0])
	}
	if err := n.NotifyRestored(context.Background(), mkScope("org-1"), "acc-1"); err != nil {
		t.Fatal(err)
	}
	if em.in[1].ClientRequestID != "mikrotik-restored:acc-1:20261031" {
		t.Fatalf("restored request id: %s", em.in[1].ClientRequestID)
	}
}

func TestCustomerNotifier_EmailFailsStillTriesWhatsApp_ReturnsError(t *testing.T) {
	em, wa := &fakeEmail{err: errors.New("smtp down")}, &fakeWA{}
	n, _ := newTestNotifier("u1", receivabledomain.Account{Name: "Budi", Email: "b@x.id", SourceType: receivabledomain.SourceTypeCRMContact, SourceID: "c1"}, em, wa)
	err := n.NotifyIsolated(context.Background(), mkScope("org-1"), "acc-1", testInvoices)
	if err == nil || !strings.Contains(err.Error(), "smtp down") {
		t.Fatalf("want email error, got %v", err)
	}
	if len(wa.in) != 1 {
		t.Fatal("whatsapp must still be attempted")
	}
	// A ChannelError means "unavailable", not a failure.
	em.err = &receivableservice.ChannelError{Reason: "no mailbox"}
	if err := n.NotifyIsolated(context.Background(), mkScope("org-1"), "acc-1", testInvoices); err != nil {
		t.Fatalf("channel error must not fail: %v", err)
	}
}

type fakeAlertMembers struct{ perm string }

func (f *fakeAlertMembers) WithPermission(_ context.Context, _ coretenant.Scope, p string) ([]receivableservice.Member, error) {
	f.perm = p
	return []receivableservice.Member{{UserID: "u1", Name: "A", Email: "a@x.id"}, {UserID: "u2", Name: "B", Email: "b@x.id"}}, nil
}

type fakePublisher struct {
	events []notificationpublisher.Event
	failOn string
}

func (f *fakePublisher) Publish(_ context.Context, e notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	f.events = append(f.events, e)
	if e.UserID == f.failOn {
		return notificationdomain.OutboxEvent{}, errors.New("boom")
	}
	return notificationdomain.OutboxEvent{}, nil
}

func TestTenantAlerter_PublishesPerMemberWithPermission(t *testing.T) {
	members, pub := &fakeAlertMembers{}, &fakePublisher{failOn: "u1"}
	a := mikrotikTenantAlerter{members: members, publisher: pub, appName: "Zyad", frontendURL: "https://app.example/", locale: "id"}
	err := a.NotifyTaskFailed(context.Background(), mkScope("org-1"), service.TaskFailureEvent{
		TaskID: "t1", RouterName: "R1", SecretName: "pppoe-1", Action: "isolate", Reason: "gagal\nterhubung",
	})
	if err == nil {
		t.Fatal("per-member error should be returned after trying all members")
	}
	if members.perm != "mikrotik_task.read" || len(pub.events) != 2 {
		t.Fatalf("perm=%s events=%d", members.perm, len(pub.events))
	}
	for _, e := range pub.events {
		if e.Type != "mikrotik.isolation_task_failed" || e.OrganizationID != "org-1" || e.Locale != "id" {
			t.Fatalf("event: %+v", e)
		}
		for _, k := range []string{"host", "username", "password", "router_host"} {
			if _, ok := e.Payload[k]; ok {
				t.Fatalf("payload must not contain %q", k)
			}
		}
		if e.Payload["tasks_url"] != "https://app.example/app/mikrotik/tasks" || e.Payload["reason"] != "gagal terhubung" ||
			e.Payload["router_name"] != "R1" || e.Payload["secret_name"] != "pppoe-1" || e.Payload["action"] != "isolate" || e.Payload["app_name"] != "Zyad" {
			t.Fatalf("payload: %+v", e.Payload)
		}
	}
}
