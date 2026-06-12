package dispatcher

import (
	"context"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
)

func TestDispatcherInterface(t *testing.T) {
	var dispatcher Dispatcher = fakeDispatcher{}

	result, err := dispatcher.Send(context.Background(), Message{
		Channel:     domain.ChannelEmail,
		Destination: "admin@example.test",
		Subject:     "Hello",
		Body:        "World",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if dispatcher.Channel() != domain.ChannelEmail {
		t.Fatalf("Channel() = %q, want %q", dispatcher.Channel(), domain.ChannelEmail)
	}
	if result.Provider != "fake" {
		t.Fatalf("Provider = %q, want fake", result.Provider)
	}
}

type fakeDispatcher struct{}

func (fakeDispatcher) Channel() domain.Channel {
	return domain.ChannelEmail
}

func (fakeDispatcher) Send(context.Context, Message) (ProviderResult, error) {
	return ProviderResult{
		Provider:          "fake",
		ProviderMessageID: "message-1",
		RawResponse:       map[string]any{"ok": true},
	}, nil
}
