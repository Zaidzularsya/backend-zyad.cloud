package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
	platformmail "zyad.cloud/internal/platform/mail"
)

const testSecret = "test-secret-key-for-mailbox-tests"

type fakeMailboxes struct {
	repository.MailboxRepository
	mailboxes map[string]domain.Mailbox
	created   []repository.MailboxParams
}

func (f *fakeMailboxes) GetForUser(_ context.Context, _ coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	mailbox, ok := f.mailboxes[id]
	if !ok || mailbox.UserID != userID {
		return domain.Mailbox{}, pgx.ErrNoRows
	}
	return mailbox, nil
}

func (f *fakeMailboxes) Create(_ context.Context, _ coretenant.Scope, userID string, p repository.MailboxParams) (domain.Mailbox, error) {
	f.created = append(f.created, p)
	return domain.Mailbox{ID: "mb-new", UserID: userID, EmailAddress: p.EmailAddress, SecretEncrypted: p.SecretEncrypted}, nil
}

type fakeMessages struct {
	repository.MessageRepository
	byKey       map[string]domain.Message
	byID        map[string]domain.Message
	attachments map[string][]string
	seq         int
}

func newFakeMessages() *fakeMessages {
	return &fakeMessages{byKey: map[string]domain.Message{}, byID: map[string]domain.Message{}, attachments: map[string][]string{}}
}

func (f *fakeMessages) CreateOutbound(_ context.Context, _ coretenant.Scope, p repository.CreateOutboundParams) (domain.Message, bool, error) {
	if existing, ok := f.byKey[p.ClientRequestID]; ok && p.ClientRequestID != "" {
		return existing, false, nil
	}
	f.seq++
	message := domain.Message{
		ID: "msg-" + string(rune('0'+f.seq)), MailboxID: p.MailboxID, Status: domain.MessageStatusQueued,
		To: p.To, Subject: p.Subject, BodyText: p.BodyText, BodyHTML: p.BodyHTML,
		RelatedEntityType: p.RelatedEntityType, RelatedEntityID: p.RelatedEntityID,
	}
	f.byID[message.ID] = message
	if p.ClientRequestID != "" {
		f.byKey[p.ClientRequestID] = message
	}
	return message, true, nil
}

func (f *fakeMessages) AddAttachment(_ context.Context, _ coretenant.Scope, messageID, assetObjectID string) error {
	f.attachments[messageID] = append(f.attachments[messageID], assetObjectID)
	return nil
}

func (f *fakeMessages) setStatus(id string, status domain.MessageStatus, errMessage string) {
	message := f.byID[id]
	message.Status, message.Error = status, errMessage
	f.byID[id] = message
}

func (f *fakeMessages) MarkSent(_ context.Context, _ coretenant.Scope, id string, _ time.Time) error {
	f.setStatus(id, domain.MessageStatusSent, "")
	return nil
}

func (f *fakeMessages) MarkFailed(_ context.Context, _ coretenant.Scope, id, errorMessage string) error {
	f.setStatus(id, domain.MessageStatusFailed, errorMessage)
	return nil
}

func (f *fakeMessages) GetForUser(_ context.Context, _ coretenant.Scope, _ string, id string) (domain.Message, error) {
	message, ok := f.byID[id]
	if !ok {
		return domain.Message{}, pgx.ErrNoRows
	}
	return message, nil
}

type fakeStorage struct {
	uploaded []assetservice.UploadObjectParams
}

func (f *fakeStorage) UploadObject(_ context.Context, _ coretenant.Scope, params assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error) {
	if !params.AllowedMimeTypes[params.MimeType] {
		return assetdomain.AssetObject{}, assetservice.ErrInvalidMimeType
	}
	_, _ = io.Copy(io.Discard, content)
	f.uploaded = append(f.uploaded, params)
	return assetdomain.AssetObject{ID: "obj-" + params.Filename}, nil
}

func (f *fakeStorage) DeleteObject(context.Context, coretenant.Scope, string) error { return nil }

func (f *fakeStorage) DownloadURL(context.Context, coretenant.Scope, string) (assetservice.DownloadResult, error) {
	return assetservice.DownloadResult{}, nil
}

type fakeCRM struct {
	entities   map[string]bool
	activities []EmailActivityInput
}

func (f *fakeCRM) RequireEntity(_ context.Context, _ coretenant.Scope, _ domain.RelatedEntityType, id string) error {
	if !f.entities[id] {
		return mailboxmodule.ErrEntityNotFound
	}
	return nil
}

func (f *fakeCRM) RecordEmailActivity(_ context.Context, _ coretenant.Scope, input EmailActivityInput) error {
	f.activities = append(f.activities, input)
	return nil
}

type sendHarness struct {
	svc      *MessageService
	messages *fakeMessages
	storage  *fakeStorage
	crm      *fakeCRM
	sends    []string // recipients per delivery, joined
	sendErr  error
}

func newSendHarness(t *testing.T) *sendHarness {
	t.Helper()
	encrypted, err := corecrypto.EncryptSecret(testSecret, "app-password")
	if err != nil {
		t.Fatal(err)
	}
	h := &sendHarness{messages: newFakeMessages(), storage: &fakeStorage{}, crm: &fakeCRM{entities: map[string]bool{"contact-1": true}}}
	mailboxes := &fakeMailboxes{mailboxes: map[string]domain.Mailbox{
		"mb-1": {ID: "mb-1", UserID: "user-1", EmailAddress: "sales@zyad.test", SecretEncrypted: encrypted,
			SMTPHost: "smtp.zyad.test", SMTPPort: 587, SMTPSecurity: domain.SecuritySTARTTLS, Status: domain.MailboxStatusActive},
	}}
	h.svc = NewMessageService(mailboxes, h.messages, h.storage, h.crm, MessageServiceOptions{
		SecretKey:   testSecret,
		Synchronous: true,
		Send: func(_ context.Context, cfg platformmail.SMTPConfig, from string, recipients []string, raw []byte) error {
			if cfg.Password != "app-password" || !cfg.RequireTLS || cfg.DialControl == nil {
				t.Errorf("unexpected smtp config: password=%q requireTLS=%v", cfg.Password, cfg.RequireTLS)
			}
			if !strings.Contains(string(raw), "Subject: ") {
				t.Error("raw message has no subject header")
			}
			h.sends = append(h.sends, strings.Join(recipients, ","))
			return h.sendErr
		},
	})
	return h
}

var testScope = coretenant.Scope{}

func (h *sendHarness) send(input SendInput) (domain.Message, error) {
	return h.svc.Send(context.Background(), testScope, "user-1", "mb-1", input)
}

func TestSendDeliversAndLogsContactActivity(t *testing.T) {
	h := newSendHarness(t)
	message, err := h.send(SendInput{
		To: []string{"Lisa <LISA@example.com>"}, Bcc: []string{"boss@example.com"},
		Subject: "Penawaran", BodyHTML: "<p>Halo</p><script>alert(1)</script>",
		RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: "contact-1",
		Files: []FileInput{{Filename: "Offer.DOCX", Size: 3, Content: strings.NewReader("abc")}},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if message.Status != domain.MessageStatusSent {
		t.Fatalf("status = %s, want sent", message.Status)
	}
	if strings.Contains(message.BodyHTML, "script") {
		t.Errorf("body not sanitized: %q", message.BodyHTML)
	}
	if len(h.sends) != 1 || h.sends[0] != "lisa@example.com,boss@example.com" {
		t.Errorf("deliveries = %v, want bcc included and lowercased", h.sends)
	}
	if len(h.storage.uploaded) != 1 || !strings.Contains(h.storage.uploaded[0].MimeType, "wordprocessingml") {
		t.Errorf("uploaded = %+v, want the docx with its office mime type", h.storage.uploaded)
	}
	if len(h.crm.activities) != 1 || h.crm.activities[0].EntityID != "contact-1" {
		t.Errorf("activities = %+v, want one for contact-1", h.crm.activities)
	}
}

func TestSendIsIdempotentPerRequestKey(t *testing.T) {
	h := newSendHarness(t)
	input := SendInput{ClientRequestID: "key-1", To: []string{"a@example.com"}, Subject: "Hi"}
	first, err := h.send(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.send(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(h.sends) != 1 {
		t.Fatalf("ids %s/%s, deliveries %d; want one message delivered once", first.ID, second.ID, len(h.sends))
	}
}

func TestSendRecordsFailureWithoutActivity(t *testing.T) {
	h := newSendHarness(t)
	h.sendErr = errors.New("535 authentication failed")
	message, err := h.send(SendInput{
		To: []string{"a@example.com"}, Subject: "Hi",
		RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: "contact-1",
	})
	if err != nil {
		t.Fatalf("Send() error = %v, want the failure on the message instead", err)
	}
	if message.Status != domain.MessageStatusFailed || !strings.Contains(message.Error, "535") {
		t.Fatalf("message = %s %q, want failed with the smtp error", message.Status, message.Error)
	}
	if len(h.crm.activities) != 0 {
		t.Error("activity recorded for an email that was not sent")
	}
}

func TestSendValidation(t *testing.T) {
	cases := map[string]struct {
		input SendInput
		code  string
		err   error
	}{
		"no recipient":    {input: SendInput{Subject: "Hi"}, code: "VALIDATION_ERROR"},
		"bad recipient":   {input: SendInput{To: []string{"not-an-email"}, Subject: "Hi"}, code: "VALIDATION_ERROR"},
		"no subject":      {input: SendInput{To: []string{"a@example.com"}}, code: "VALIDATION_ERROR"},
		"unknown contact": {input: SendInput{To: []string{"a@example.com"}, Subject: "Hi", RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: "nope"}, err: mailboxmodule.ErrEntityNotFound},
		"exe attachment":  {input: SendInput{To: []string{"a@example.com"}, Subject: "Hi", Files: []FileInput{{Filename: "x.exe", Size: 1, Content: strings.NewReader("x")}}}, err: mailboxmodule.ErrAttachmentType},
		"too large":       {input: SendInput{To: []string{"a@example.com"}, Subject: "Hi", Files: []FileInput{{Filename: "a.pdf", Size: MaxTotalAttachmentBytes + 1, Content: strings.NewReader("x")}}}, err: mailboxmodule.ErrAttachmentsTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)
			_, err := h.send(tc.input)
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			var appErr *coreerrors.AppError
			if tc.code != "" && (!errors.As(err, &appErr) || appErr.Code != tc.code) {
				t.Fatalf("error = %v, want code %s", err, tc.code)
			}
			if len(h.sends) != 0 {
				t.Error("message delivered despite validation error")
			}
		})
	}
}

func TestSendRejectsOtherUsersMailbox(t *testing.T) {
	h := newSendHarness(t)
	_, err := h.svc.Send(context.Background(), testScope, "user-2", "mb-1", SendInput{To: []string{"a@example.com"}, Subject: "Hi"})
	if !errors.Is(err, mailboxmodule.ErrMailboxNotFound) {
		t.Fatalf("error = %v, want ErrMailboxNotFound", err)
	}
}

func TestCreateMailboxVerifiesAndEncrypts(t *testing.T) {
	mailboxes := &fakeMailboxes{}
	var verified bool
	svc := NewMailboxService(mailboxes, newFakeMessages(), &fakeStorage{}, MailboxServiceOptions{
		SecretKey: testSecret,
		Verify: func(_ context.Context, cfg platformmail.SMTPConfig) error {
			verified = cfg.Password == "app-password" && cfg.Secure && cfg.User == "sales@zyad.test"
			return nil
		},
	})
	_, err := svc.Create(context.Background(), testScope, "user-1", MailboxInput{
		EmailAddress: " Sales@Zyad.test ", Password: "app-password",
		SMTPHost: "smtp.zyad.test", SMTPPort: 465, SMTPSecurity: domain.SecuritySSL,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !verified {
		t.Fatal("SMTP settings were not verified with the given password")
	}
	stored := mailboxes.created[0]
	if stored.SecretEncrypted == "" || stored.SecretEncrypted == "app-password" {
		t.Fatalf("secret stored as %q, want encrypted", stored.SecretEncrypted)
	}
	if plain, _ := corecrypto.DecryptSecret(testSecret, stored.SecretEncrypted); plain != "app-password" {
		t.Fatalf("decrypted secret = %q", plain)
	}
}

func TestCreateMailboxRejectsInvalidSettings(t *testing.T) {
	svc := NewMailboxService(&fakeMailboxes{}, newFakeMessages(), &fakeStorage{}, MailboxServiceOptions{
		SecretKey: testSecret,
		Verify:    func(context.Context, platformmail.SMTPConfig) error { t.Fatal("verify called"); return nil },
	})
	base := MailboxInput{EmailAddress: "a@zyad.test", Password: "x", SMTPHost: "smtp.zyad.test", SMTPPort: 587, SMTPSecurity: domain.SecuritySTARTTLS}
	cases := map[string]func(MailboxInput) MailboxInput{
		"redis port":      func(in MailboxInput) MailboxInput { in.SMTPPort = 6379; return in },
		"plaintext":       func(in MailboxInput) MailboxInput { in.SMTPSecurity = "none"; return in },
		"no password":     func(in MailboxInput) MailboxInput { in.Password = ""; return in },
		"host with path":  func(in MailboxInput) MailboxInput { in.SMTPHost = "smtp.zyad.test/x"; return in },
		"partial imap":    func(in MailboxInput) MailboxInput { in.IMAPHost = "imap.zyad.test"; return in },
		"invalid address": func(in MailboxInput) MailboxInput { in.EmailAddress = "nope"; return in },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Create(context.Background(), testScope, "user-1", mutate(base)); err == nil {
				t.Fatal("Create() error = nil")
			}
		})
	}
}

func TestPublicDialControl(t *testing.T) {
	guard := publicDialControl(false)
	for _, address := range []string{"127.0.0.1:587", "10.0.9.2:587", "192.168.1.5:465", "169.254.169.254:25", "100.64.1.1:587", "[::1]:587"} {
		if err := guard("tcp", address, nil); !errors.Is(err, mailboxmodule.ErrHostNotAllowed) {
			t.Errorf("%s: error = %v, want ErrHostNotAllowed", address, err)
		}
	}
	if err := guard("tcp", "142.250.4.108:587", nil); err != nil {
		t.Errorf("public address rejected: %v", err)
	}
	if err := publicDialControl(true)("tcp", "127.0.0.1:1025", nil); err != nil {
		t.Errorf("allowPrivate still rejected: %v", err)
	}
}

func TestHTMLToText(t *testing.T) {
	got := htmlToText(sanitizeHTML("<p>Halo <b>Lisa</b>,</p><p>Harga &amp; syarat<br>terlampir.</p><script>x()</script>"))
	want := "Halo Lisa,\nHarga & syarat\nterlampir."
	if got != want {
		t.Fatalf("htmlToText() = %q, want %q", got, want)
	}
}
