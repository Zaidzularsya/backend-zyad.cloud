package dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/whatsapp"
)

func TestWhatsAppDispatcherSend(t *testing.T) {
	sentAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	client := &fakeWhatsAppClient{
		result: whatsapp.Result{
			Provider:  "whatsapp-api",
			MessageID: "message-1",
			Raw:       map[string]any{"accepted": true},
			SentAt:    sentAt,
		},
	}
	dispatcher := NewWhatsAppDispatcher(client)

	result, err := dispatcher.Send(context.Background(), Message{
		Channel:     domain.ChannelWhatsApp,
		Destination: "+628123456789",
		Subject:     "Ignored subject",
		Body:        "Hello WhatsApp",
		Metadata:    map[string]any{"template_code": "lead.created"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if dispatcher.Channel() != domain.ChannelWhatsApp {
		t.Fatalf("Channel() = %q, want %q", dispatcher.Channel(), domain.ChannelWhatsApp)
	}
	if client.message.To != "+628123456789" {
		t.Fatalf("client To = %q, want +628123456789", client.message.To)
	}
	if client.message.Text != "Hello WhatsApp" {
		t.Fatalf("client Text = %q, want Hello WhatsApp", client.message.Text)
	}
	if result.Provider != "whatsapp-api" {
		t.Fatalf("Provider = %q, want whatsapp-api", result.Provider)
	}
	if result.ProviderMessageID != "message-1" {
		t.Fatalf("ProviderMessageID = %q, want message-1", result.ProviderMessageID)
	}
	if !result.SentAt.Equal(sentAt) {
		t.Fatalf("SentAt = %v, want %v", result.SentAt, sentAt)
	}
}

func TestWhatsAppDispatcherIgnoresSubject(t *testing.T) {
	client := &fakeWhatsAppClient{}
	dispatcher := NewWhatsAppDispatcher(client)

	_, err := dispatcher.Send(context.Background(), Message{
		Destination: "+628123456789",
		Subject:     "",
		Body:        "Subject is not required",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestWhatsAppDispatcherRequiresDestination(t *testing.T) {
	dispatcher := NewWhatsAppDispatcher(&fakeWhatsAppClient{})

	_, err := dispatcher.Send(context.Background(), Message{Body: "Hello"})
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Send() error = %v, want ErrInvalidMessage", err)
	}
}

func TestWhatsAppDispatcherMapsProviderError(t *testing.T) {
	dispatcher := NewWhatsAppDispatcher(&fakeWhatsAppClient{err: errors.New("whatsapp down")})

	_, err := dispatcher.Send(context.Background(), Message{
		Destination: "+628123456789",
		Body:        "Hello",
	})
	if !errors.Is(err, ErrProviderFailed) {
		t.Fatalf("Send() error = %v, want ErrProviderFailed", err)
	}
}

type fakeWhatsAppClient struct {
	message whatsapp.Message
	result  whatsapp.Result
	err     error
}

func (c *fakeWhatsAppClient) SendText(_ context.Context, message whatsapp.Message) (whatsapp.Result, error) {
	c.message = message
	if c.err != nil {
		return whatsapp.Result{}, c.err
	}
	return c.result, nil
}
