package mail

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/config"
)

func TestNoopMailerSend(t *testing.T) {
	mailer := NewNoopMailer()

	result, err := mailer.Send(context.Background(), Message{
		To:      "admin@example.test",
		Subject: "Hello",
		Body:    "World",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
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

func TestNewMailerFromConfigReturnsNoopWhenHostEmpty(t *testing.T) {
	mailer := NewMailerFromConfig(config.MailConfig{})
	if _, ok := mailer.(NoopMailer); !ok {
		t.Fatalf("mailer = %T, want NoopMailer", mailer)
	}
}

func TestNewMailerFromConfigReturnsSMTPWhenHostConfigured(t *testing.T) {
	mailer := NewMailerFromConfig(config.MailConfig{
		Host: "smtp.example.test",
		Port: 587,
		From: "noreply@example.test",
	})
	if _, ok := mailer.(*SMTPMailer); !ok {
		t.Fatalf("mailer = %T, want *SMTPMailer", mailer)
	}
}

func TestBuildMessage(t *testing.T) {
	message := string(buildMessage(
		"noreply@example.test",
		"user@example.test",
		"Selamat datang",
		"Halo",
	))

	for _, want := range []string{
		"From: noreply@example.test",
		"To: user@example.test",
		"Subject: Selamat datang",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"\r\n\r\nHalo",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestResolveSenderUsesMailFromAsDisplayName(t *testing.T) {
	envelopeFrom, headerFrom := resolveSender(SMTPConfig{
		User: "sender@example.test",
		From: "Zyad Cloud",
	})

	if envelopeFrom != "sender@example.test" {
		t.Fatalf("envelopeFrom = %q, want sender@example.test", envelopeFrom)
	}
	if headerFrom != `"Zyad Cloud" <sender@example.test>` {
		t.Fatalf("headerFrom = %q, want display name with sender address", headerFrom)
	}
}

func TestResolveSenderUsesMailFromAddressWhenEmail(t *testing.T) {
	envelopeFrom, headerFrom := resolveSender(SMTPConfig{
		User: "sender@example.test",
		From: "noreply@example.test",
	})

	if envelopeFrom != "noreply@example.test" {
		t.Fatalf("envelopeFrom = %q, want noreply@example.test", envelopeFrom)
	}
	if headerFrom != "<noreply@example.test>" {
		t.Fatalf("headerFrom = %q, want encoded noreply address", headerFrom)
	}
}
