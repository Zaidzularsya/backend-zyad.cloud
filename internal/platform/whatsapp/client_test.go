package whatsapp

import (
	"context"
	"testing"
)

func TestNoopClientSendText(t *testing.T) {
	client := NewNoopClient()

	result, err := client.SendText(context.Background(), Message{
		To:   "+628123456789",
		Text: "Hello",
	})
	if err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if result.Provider != "noop" {
		t.Fatalf("Provider = %q, want noop", result.Provider)
	}
	if result.MessageID == "" {
		t.Fatal("MessageID is empty")
	}
	if result.Raw["noop"] != true {
		t.Fatalf("Raw noop = %#v, want true", result.Raw["noop"])
	}
}
