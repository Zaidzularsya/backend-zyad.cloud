package dispatcher

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

func TestNoopDispatcherSend(t *testing.T) {
	dispatcher := NewNoopDispatcher(domain.ChannelWhatsApp)
	sentAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return sentAt }

	result, err := dispatcher.Send(context.Background(), Message{
		Destination: "+628123456789",
		Subject:     "Ignored",
		Body:        "Hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if dispatcher.Channel() != domain.ChannelWhatsApp {
		t.Fatalf("Channel() = %q, want %q", dispatcher.Channel(), domain.ChannelWhatsApp)
	}
	if result.Provider != "noop" {
		t.Fatalf("Provider = %q, want noop", result.Provider)
	}
	if result.ProviderMessageID == "" {
		t.Fatal("ProviderMessageID is empty")
	}
	if result.RawResponse["noop"] != true {
		t.Fatalf("RawResponse noop = %#v, want true", result.RawResponse["noop"])
	}
	if result.RawResponse["channel"] != string(domain.ChannelWhatsApp) {
		t.Fatalf("RawResponse channel = %#v", result.RawResponse["channel"])
	}
	if !result.SentAt.Equal(sentAt) {
		t.Fatalf("SentAt = %v, want %v", result.SentAt, sentAt)
	}
}

func TestNoopDispatcherDefaultsChannel(t *testing.T) {
	dispatcher := NewNoopDispatcher("")

	if dispatcher.Channel() != domain.ChannelInApp {
		t.Fatalf("Channel() = %q, want %q", dispatcher.Channel(), domain.ChannelInApp)
	}
}
