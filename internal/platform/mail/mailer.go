package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"

	"zyad.cloud/internal/config"
)

type Message struct {
	To       string
	Subject  string
	Body     string
	Metadata map[string]any
}

type Result struct {
	Provider  string
	MessageID string
	Raw       map[string]any
	SentAt    time.Time
}

type Mailer interface {
	Send(ctx context.Context, message Message) (Result, error)
}

type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	Secure   bool
	TLS      bool
}

type SMTPMailer struct {
	config SMTPConfig
}

func NewMailerFromConfig(cfg config.MailConfig) Mailer {
	if strings.TrimSpace(cfg.Host) == "" {
		return NewNoopMailer()
	}

	return NewSMTPMailer(SMTPConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		From:     cfg.From,
		Secure:   cfg.Secure,
		TLS:      cfg.TLS,
	})
}

func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &SMTPMailer{config: cfg}
}

func (m *SMTPMailer) Send(ctx context.Context, message Message) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	cfg := m.config
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return Result{}, errors.New("mail host is required")
	}

	envelopeFrom, headerFrom := resolveSender(cfg)
	if envelopeFrom == "" {
		return Result{}, errors.New("mail from is required")
	}

	to := strings.TrimSpace(message.To)
	if to == "" {
		return Result{}, errors.New("mail recipient is required")
	}

	address := net.JoinHostPort(host, fmt.Sprintf("%d", cfg.Port))
	client, err := dialSMTP(ctx, cfg, address)
	if err != nil {
		return Result{}, err
	}
	defer client.Close()

	if cfg.User != "" {
		auth := smtp.PlainAuth("", cfg.User, cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return Result{}, fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(envelopeFrom); err != nil {
		return Result{}, fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return Result{}, fmt.Errorf("smtp recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return Result{}, fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(buildMessage(headerFrom, to, message.Subject, message.Body)); err != nil {
		_ = writer.Close()
		return Result{}, fmt.Errorf("smtp write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return Result{}, fmt.Errorf("smtp close message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return Result{}, fmt.Errorf("smtp quit: %w", err)
	}

	sentAt := time.Now().UTC()
	return Result{
		Provider:  "smtp",
		MessageID: fmt.Sprintf("smtp-%d", sentAt.UnixNano()),
		Raw: map[string]any{
			"to":      to,
			"subject": message.Subject,
		},
		SentAt: sentAt,
	}, nil
}

func dialSMTP(ctx context.Context, cfg SMTPConfig, address string) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if cfg.Secure {
		conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName: cfg.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", err)
		}
		return smtp.NewClient(conn, cfg.Host)
	}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}

	if cfg.TLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{
				ServerName: cfg.Host,
				MinVersion: tls.VersionTLS12,
			}); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}

	return client, nil
}

func buildMessage(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func resolveSender(cfg SMTPConfig) (envelopeFrom string, headerFrom string) {
	from := strings.TrimSpace(cfg.From)
	user := strings.TrimSpace(cfg.User)

	if isEmailAddress(from) {
		return from, (&netmail.Address{Address: from}).String()
	}
	if from != "" && isEmailAddress(user) {
		return user, (&netmail.Address{Name: from, Address: user}).String()
	}
	if user != "" {
		return user, (&netmail.Address{Address: user}).String()
	}
	return "", ""
}

func isEmailAddress(value string) bool {
	address, err := netmail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

type NoopMailer struct {
	Provider string
}

func NewNoopMailer() NoopMailer {
	return NoopMailer{Provider: "noop"}
}

func (m NoopMailer) Send(_ context.Context, message Message) (Result, error) {
	provider := m.Provider
	if provider == "" {
		provider = "noop"
	}

	return Result{
		Provider:  provider,
		MessageID: "noop",
		Raw: map[string]any{
			"to":      message.To,
			"subject": message.Subject,
			"noop":    true,
		},
		SentAt: time.Now().UTC(),
	}, nil
}
