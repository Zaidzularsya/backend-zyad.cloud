package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SendEnvelope delivers a raw message (see BuildMIME) to every recipient,
// Bcc included, over the SMTP server in cfg.
func SendEnvelope(ctx context.Context, cfg SMTPConfig, from string, recipients []string, raw []byte) error {
	if len(recipients) == 0 {
		return errors.New("mail recipient is required")
	}
	client, err := openSMTP(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp recipient %s: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("smtp write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp close message: %w", err)
	}
	return client.Quit()
}

// VerifySMTP connects and authenticates without sending anything, to check
// a mailbox's settings before they are saved.
func VerifySMTP(ctx context.Context, cfg SMTPConfig) error {
	client, err := openSMTP(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Quit()
}

func openSMTP(ctx context.Context, cfg SMTPConfig) (*smtp.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, errors.New("mail host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}

	client, err := dialSMTP(ctx, cfg, net.JoinHostPort(host, fmt.Sprintf("%d", cfg.Port)))
	if err != nil {
		return nil, err
	}
	if cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, host)); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp auth: %w", err)
		}
	}
	return client, nil
}
