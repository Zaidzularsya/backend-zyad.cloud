package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/dispatcher"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/repository"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
)

func TestNotificationServiceSendByTemplateCreatesLogBeforeDispatch(t *testing.T) {
	logRepo := newFakeNotificationLogRepo()
	sender := &fakeDispatcher{channel: domain.ChannelEmail}
	service := NewNotificationService(
		&fakeTemplateRepo{template: activePaymentTemplate()},
		logRepo,
		&fakePreferenceChecker{enabled: true},
		notificationtemplate.NewRenderer(nil),
		sender,
	)

	log, err := service.SendByTemplate(context.Background(), domain.Notification{
		EventID:        "event-1",
		EventType:      "payment.paid",
		TemplateCode:   "payment.paid",
		OrganizationID: "org-1",
		Channel:        domain.ChannelEmail,
		UserID:         "user-1",
		Recipient: domain.NotificationRecipient{
			Type:   "user",
			UserID: "user-1",
			Name:   "Budi",
			Email:  "budi@example.test",
		},
		Payload: paymentPayload(),
	})
	if err != nil {
		t.Fatalf("SendByTemplate() error = %v", err)
	}

	if log.Status != domain.LogStatusSent {
		t.Fatalf("log.Status = %q, want %q", log.Status, domain.LogStatusSent)
	}
	if sender.calls != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", sender.calls)
	}
	if sender.messages[0].Destination != "budi@example.test" {
		t.Fatalf("message destination = %q", sender.messages[0].Destination)
	}
	if sender.messages[0].Body != "Budi paid INV-2026-0001" {
		t.Fatalf("message body = %q", sender.messages[0].Body)
	}
	if len(logRepo.calls) < 3 || logRepo.calls[0] != "create" || logRepo.calls[1] != "mark_processing" || logRepo.calls[2] != "mark_sent" {
		t.Fatalf("log repo calls = %#v", logRepo.calls)
	}
}

func TestNotificationServiceSendDirectCancelsWhenPreferenceDisabled(t *testing.T) {
	logRepo := newFakeNotificationLogRepo()
	sender := &fakeDispatcher{channel: domain.ChannelEmail}
	service := NewNotificationService(
		nil,
		logRepo,
		&fakePreferenceChecker{enabled: false},
		nil,
		sender,
	)

	log, err := service.SendDirect(context.Background(), DirectNotificationRequest{
		EventType: "payment.paid",
		Channel:   domain.ChannelEmail,
		Recipient: domain.NotificationRecipient{
			UserID: "user-1",
			Email:  "budi@example.test",
		},
		Subject: "Hello",
		Body:    "Welcome",
	})
	if err != nil {
		t.Fatalf("SendDirect() error = %v", err)
	}

	if log.Status != domain.LogStatusCancelled {
		t.Fatalf("log.Status = %q, want %q", log.Status, domain.LogStatusCancelled)
	}
	if sender.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", sender.calls)
	}
	if len(logRepo.calls) < 2 || logRepo.calls[0] != "create" || logRepo.calls[1] != "mark_cancelled" {
		t.Fatalf("log repo calls = %#v", logRepo.calls)
	}
}

func TestNotificationServiceSendDirectMarksProviderErrorFailed(t *testing.T) {
	logRepo := newFakeNotificationLogRepo()
	sender := &fakeDispatcher{
		channel: domain.ChannelEmail,
		err:     errors.New("smtp unavailable"),
	}
	service := NewNotificationService(nil, logRepo, nil, nil, sender)
	service.now = func() time.Time {
		return time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	}

	log, err := service.SendDirect(context.Background(), DirectNotificationRequest{
		EventType: "payment.paid",
		Channel:   domain.ChannelEmail,
		Recipient: domain.NotificationRecipient{
			UserID: "user-1",
			Email:  "budi@example.test",
		},
		Subject: "Payment paid",
		Body:    "Paid",
	})
	if err == nil {
		t.Fatal("SendDirect() error = nil, want error")
	}

	if log.Status != domain.LogStatusFailed {
		t.Fatalf("log.Status = %q, want %q", log.Status, domain.LogStatusFailed)
	}
	if log.Attempts != 1 {
		t.Fatalf("log.Attempts = %d, want 1", log.Attempts)
	}
	if log.NextRetryAt == nil || !log.NextRetryAt.Equal(time.Date(2026, 6, 12, 10, 1, 0, 0, time.UTC)) {
		t.Fatalf("log.NextRetryAt = %v", log.NextRetryAt)
	}
}

func TestNotificationServiceCancelOnlyPending(t *testing.T) {
	logRepo := newFakeNotificationLogRepo()
	logRepo.logs["log-1"] = domain.NotificationLog{
		ID:     "log-1",
		Status: domain.LogStatusPending,
	}
	service := NewNotificationService(nil, logRepo, nil, nil)

	log, err := service.Cancel(context.Background(), "log-1")
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if log.Status != domain.LogStatusCancelled {
		t.Fatalf("log.Status = %q, want %q", log.Status, domain.LogStatusCancelled)
	}
}

func TestNotificationServiceRetryRejectsPending(t *testing.T) {
	logRepo := newFakeNotificationLogRepo()
	logRepo.logs["log-1"] = domain.NotificationLog{
		ID:          "log-1",
		Channel:     domain.ChannelEmail,
		Destination: "budi@example.test",
		Subject:     "Payment paid",
		Body:        "Paid",
		Status:      domain.LogStatusPending,
		MaxAttempts: 3,
	}
	service := NewNotificationService(nil, logRepo, nil, nil, &fakeDispatcher{channel: domain.ChannelEmail})

	_, err := service.Retry(context.Background(), "log-1")
	if err == nil {
		t.Fatal("Retry() error = nil, want error")
	}
	if len(logRepo.calls) != 0 {
		t.Fatalf("log repo calls = %#v, want none", logRepo.calls)
	}
}

func activePaymentTemplate() domain.NotificationTemplate {
	return domain.NotificationTemplate{
		ID:              "template-1",
		Code:            "payment.paid",
		Name:            "Payment paid",
		Channel:         domain.ChannelEmail,
		Locale:          "id-ID",
		SubjectTemplate: "Payment {{invoice_number}}",
		BodyTemplate:    "{{customer_name}} paid {{invoice_number}}",
		AvailableVariables: []domain.NotificationVariable{
			{Key: "app_name", Required: true},
			{Key: "customer_name", Required: true},
			{Key: "invoice_number", Required: true},
			{Key: "amount", Required: true},
			{Key: "paid_at", Required: true},
		},
		Status:   domain.TemplateStatusActive,
		IsActive: true,
		Version:  2,
	}
}

func paymentPayload() map[string]any {
	return map[string]any{
		"app_name":       "Zyad Cloud",
		"customer_name":  "Budi",
		"invoice_number": "INV-2026-0001",
		"amount":         "Rp150.000",
		"paid_at":        "2026-06-12 10:00 WIB",
	}
}

type fakeTemplateRepo struct {
	template domain.NotificationTemplate
	err      error
}

func (r *fakeTemplateRepo) FindActiveByCodeChannelLocale(context.Context, string, domain.Channel, string) (domain.NotificationTemplate, error) {
	if r.err != nil {
		return domain.NotificationTemplate{}, r.err
	}
	return r.template, nil
}

type fakePreferenceChecker struct {
	enabled bool
	err     error
}

func (c *fakePreferenceChecker) IsEnabled(context.Context, string, string, string, domain.Channel) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	return c.enabled, nil
}

type fakeDispatcher struct {
	channel  domain.Channel
	err      error
	calls    int
	messages []dispatcher.Message
}

func (d *fakeDispatcher) Channel() domain.Channel {
	return d.channel
}

func (d *fakeDispatcher) Send(_ context.Context, message dispatcher.Message) (dispatcher.ProviderResult, error) {
	d.calls++
	d.messages = append(d.messages, message)
	if d.err != nil {
		return dispatcher.ProviderResult{}, d.err
	}
	return dispatcher.ProviderResult{
		Provider:          "fake",
		ProviderMessageID: "provider-1",
		RawResponse:       map[string]any{"ok": true},
		SentAt:            time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
	}, nil
}

type fakeNotificationLogRepo struct {
	nextID int
	logs   map[string]domain.NotificationLog
	calls  []string
}

func newFakeNotificationLogRepo() *fakeNotificationLogRepo {
	return &fakeNotificationLogRepo{
		nextID: 1,
		logs:   make(map[string]domain.NotificationLog),
	}
}

func (r *fakeNotificationLogRepo) CreatePending(_ context.Context, log *domain.NotificationLog) error {
	r.calls = append(r.calls, "create")
	if log.ID == "" {
		log.ID = "log-1"
	}
	if log.Status == "" {
		log.Status = domain.LogStatusPending
	}
	if log.MaxAttempts == 0 {
		log.MaxAttempts = 3
	}
	r.logs[log.ID] = *log
	return nil
}

func (r *fakeNotificationLogRepo) FindByID(_ context.Context, id string) (domain.NotificationLog, error) {
	log, ok := r.logs[id]
	if !ok {
		return domain.NotificationLog{}, errors.New("not found")
	}
	return log, nil
}

func (r *fakeNotificationLogRepo) List(context.Context, repository.NotificationLogListFilter) ([]domain.NotificationLog, error) {
	logs := make([]domain.NotificationLog, 0, len(r.logs))
	for _, log := range r.logs {
		logs = append(logs, log)
	}
	return logs, nil
}

func (r *fakeNotificationLogRepo) FindRetryable(context.Context, int) ([]domain.NotificationLog, error) {
	return nil, nil
}

func (r *fakeNotificationLogRepo) MarkProcessing(_ context.Context, id string) error {
	r.calls = append(r.calls, "mark_processing")
	log := r.logs[id]
	log.Status = domain.LogStatusProcessing
	r.logs[id] = log
	return nil
}

func (r *fakeNotificationLogRepo) MarkSent(_ context.Context, id string, provider string, providerMessageID string, providerResponse map[string]any) error {
	r.calls = append(r.calls, "mark_sent")
	log := r.logs[id]
	log.Status = domain.LogStatusSent
	log.Provider = provider
	log.ProviderMessageID = providerMessageID
	log.ProviderResponse = providerResponse
	sentAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	log.SentAt = &sentAt
	r.logs[id] = log
	return nil
}

func (r *fakeNotificationLogRepo) MarkFailed(_ context.Context, id string, errorMessage string, nextRetryAt *time.Time) error {
	r.calls = append(r.calls, "mark_failed")
	log := r.logs[id]
	log.Status = domain.LogStatusFailed
	log.Attempts++
	log.ErrorMessage = errorMessage
	log.NextRetryAt = nextRetryAt
	r.logs[id] = log
	return nil
}

func (r *fakeNotificationLogRepo) MarkDead(_ context.Context, id string, errorMessage string) error {
	r.calls = append(r.calls, "mark_dead")
	log := r.logs[id]
	log.Status = domain.LogStatusDead
	log.Attempts = log.MaxAttempts
	log.ErrorMessage = errorMessage
	r.logs[id] = log
	return nil
}

func (r *fakeNotificationLogRepo) MarkCancelled(_ context.Context, id string, errorMessage string) error {
	r.calls = append(r.calls, "mark_cancelled")
	log := r.logs[id]
	log.Status = domain.LogStatusCancelled
	log.ErrorMessage = errorMessage
	r.logs[id] = log
	return nil
}
