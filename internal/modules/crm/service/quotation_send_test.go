package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeSends struct {
	records []repository.RecordQuotationSendParams
	byReq   map[string]domain.QuotationSend
}

func (f *fakeSends) Record(_ context.Context, _ coretenant.Scope, p repository.RecordQuotationSendParams) (domain.QuotationSend, error) {
	f.records = append(f.records, p)
	s := domain.QuotationSend{ID: "s" + itoaTest(len(f.records)), Channel: p.Channel, Status: p.Status, Error: p.Error, Recipient: p.Recipient}
	if p.ClientRequestID != "" {
		f.byReq[p.ClientRequestID] = s
	}
	return s, nil
}
func (f *fakeSends) FindByClientRequest(_ context.Context, _ coretenant.Scope, _ string, id string) (domain.QuotationSend, error) {
	s, ok := f.byReq[id]
	if !ok {
		return domain.QuotationSend{}, pgx.ErrNoRows
	}
	return s, nil
}
func (f *fakeSends) ListByQuotation(context.Context, coretenant.Scope, string) ([]domain.QuotationSend, error) {
	return nil, nil
}

func itoaTest(n int) string { return string(rune('0' + n)) }

type fakeEmail struct {
	calls []QuotationEmailInput
	err   error
}

func (f *fakeEmail) SendQuotationEmail(_ context.Context, _ coretenant.Scope, in QuotationEmailInput) (string, error) {
	f.calls = append(f.calls, in)
	return "m1", f.err
}

type fakeContacts struct {
	repository.ContactRepository
	contact domain.Contact
}

func (f fakeContacts) FindByID(context.Context, coretenant.Scope, string) (domain.Contact, error) {
	return f.contact, nil
}

func sendFixture(contactEmail string) (*quotationService, *snapshotRepo, *fakeSends, *fakeEmail, *fakeActivities) {
	svc, repo, _, _, acts := docFixture()
	contactID := "c1"
	q := repo.stored["q1"]
	q.ContactID = &contactID
	repo.stored["q1"] = q
	sends := &fakeSends{byReq: map[string]domain.QuotationSend{}}
	email := &fakeEmail{}
	svc.docs.Contacts = fakeContacts{contact: domain.Contact{ID: "c1", FirstName: "Budi", Email: contactEmail}}
	WithQuotationChannels(QuotationChannelDeps{Sends: sends, Email: email})(svc)
	return svc, repo, sends, email, acts
}

func TestSendViaEmailMarksSentAndLogs(t *testing.T) {
	svc, repo, sends, email, acts := sendFixture("budi@example.com")
	res, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeTextPDF, ClientRequestID: "req-1", UserID: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Send.Status != domain.QuotationSendSent || res.Quotation.Status != domain.QuotationStatusSent || !repo.sent {
		t.Fatalf("res = %+v", res)
	}
	in := email.calls[0]
	if in.To != "budi@example.com" || in.Attachment == nil || string(in.Attachment.Content) != "%PDF-final" || in.Subject == "" || in.ContactID != "c1" {
		t.Fatalf("email input = %+v", in)
	}
	if len(sends.records) != 1 || len(acts.created) != 1 || acts.created[0].Type != domain.ActivityTypeEmail {
		t.Fatalf("records=%d activities=%+v", len(sends.records), acts.created)
	}
	// Retry dengan client_request_id sama: tidak kirim ulang.
	if _, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeTextPDF, ClientRequestID: "req-1", UserID: "u1",
	}); err != nil || len(email.calls) != 1 {
		t.Fatalf("idempotent resend calls=%d err=%v", len(email.calls), err)
	}
}

func TestSendViaFailureKeepsDraftAndLogsFailed(t *testing.T) {
	svc, repo, sends, email, _ := sendFixture("budi@example.com")
	email.err = errors.New("Email gagal dikirim.")
	res, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeText, ClientRequestID: "req-2", UserID: "u1",
	})
	if err != nil || res.Send.Status != domain.QuotationSendFailed || repo.sent || res.Quotation.Status != domain.QuotationStatusDraft {
		t.Fatalf("res = %+v err=%v", res, err)
	}
	if sends.records[0].Error != "Email gagal dikirim." {
		t.Fatalf("error = %q", sends.records[0].Error)
	}
}

func TestSendViaChannelUnavailableIsNotLogged(t *testing.T) {
	svc, repo, sends, _, _ := sendFixture("")
	_, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeText, ClientRequestID: "req-3", UserID: "u1",
	})
	if !errors.Is(err, ErrChannelUnavailable) || len(sends.records) != 0 || repo.sent {
		t.Fatalf("err=%v records=%d", err, len(sends.records))
	}
	_, err = svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelWhatsApp, Mode: domain.QuotationModeText, ClientRequestID: "req-4", UserID: "u1",
	})
	if !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("unconfigured whatsapp err = %v", err)
	}
}

func TestSendViaRejectsClosedQuotation(t *testing.T) {
	svc, repo, _, _, _ := sendFixture("budi@example.com")
	q := repo.stored["q1"]
	q.Status = domain.QuotationStatusApproved
	repo.stored["q1"] = q
	if _, err := svc.SendVia(context.Background(), coretenant.Scope{}, "q1", SendQuotationInput{
		Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeText, ClientRequestID: "r", UserID: "u1",
	}); !errors.Is(err, ErrQuotationNotSendable) {
		t.Fatalf("err = %v", err)
	}
}
