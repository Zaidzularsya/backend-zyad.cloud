package dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/mail"
)

func TestEmailDispatcherSend(t *testing.T) {
	sentAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	mailer := &fakeMailer{
		result: mail.Result{
			Provider:  "smtp",
			MessageID: "message-1",
			Raw:       map[string]any{"accepted": true},
			SentAt:    sentAt,
		},
	}
	dispatcher := NewEmailDispatcher(mailer)

	result, err := dispatcher.Send(context.Background(), Message{
		Channel:     domain.ChannelEmail,
		Destination: "admin@example.test",
		Subject:     "Hello",
		Body:        "World",
		Metadata:    map[string]any{"template_code": "auth.password_reset"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if dispatcher.Channel() != domain.ChannelEmail {
		t.Fatalf("Channel() = %q, want %q", dispatcher.Channel(), domain.ChannelEmail)
	}
	if mailer.message.To != "admin@example.test" {
		t.Fatalf("mailer To = %q, want admin@example.test", mailer.message.To)
	}
	if result.Provider != "smtp" {
		t.Fatalf("Provider = %q, want smtp", result.Provider)
	}
	if result.ProviderMessageID != "message-1" {
		t.Fatalf("ProviderMessageID = %q, want message-1", result.ProviderMessageID)
	}
	if !result.SentAt.Equal(sentAt) {
		t.Fatalf("SentAt = %v, want %v", result.SentAt, sentAt)
	}
}

func TestEmailDispatcherRequiresDestination(t *testing.T) {
	dispatcher := NewEmailDispatcher(&fakeMailer{})

	_, err := dispatcher.Send(context.Background(), Message{Subject: "Hello"})
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Send() error = %v, want ErrInvalidMessage", err)
	}
}

func TestEmailDispatcherRequiresSubject(t *testing.T) {
	dispatcher := NewEmailDispatcher(&fakeMailer{})

	_, err := dispatcher.Send(context.Background(), Message{Destination: "admin@example.test"})
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Send() error = %v, want ErrInvalidMessage", err)
	}
}

func TestEmailDispatcherMapsProviderError(t *testing.T) {
	dispatcher := NewEmailDispatcher(&fakeMailer{err: errors.New("smtp down")})

	_, err := dispatcher.Send(context.Background(), Message{
		Destination: "admin@example.test",
		Subject:     "Hello",
	})
	if !errors.Is(err, ErrProviderFailed) {
		t.Fatalf("Send() error = %v, want ErrProviderFailed", err)
	}
}

type fakeMailer struct {
	message mail.Message
	result  mail.Result
	err     error
}

func (m *fakeMailer) Send(_ context.Context, message mail.Message) (mail.Result, error) {
	m.message = message
	if m.err != nil {
		return mail.Result{}, m.err
	}
	return m.result, nil
}
