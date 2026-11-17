package mail

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"
)

// authenticate logs in with the mechanism that fits cfg and the server:
// XOAUTH2 when cfg carries an OAuth access token (Microsoft 365 /
// Outlook.com reject basic auth), otherwise PLAIN, falling back to LOGIN
// for servers that only advertise LOGIN (smtp.office365.com answers PLAIN
// with "504 5.7.4 Unrecognized authentication type").
func authenticate(client *smtp.Client, cfg SMTPConfig, host string) error {
	if cfg.OAuthToken != "" {
		if err := client.Auth(xoauth2Auth{user: cfg.User, token: cfg.OAuthToken}); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		return nil
	}
	if cfg.User == "" {
		return nil
	}
	var auth smtp.Auth = smtp.PlainAuth("", cfg.User, cfg.Password, host)
	if _, mechanisms := client.Extension("AUTH"); mechanisms != "" &&
		!hasMechanism(mechanisms, "PLAIN") && hasMechanism(mechanisms, "LOGIN") {
		// LOGIN sends the password like PLAIN does, so it gets the same
		// TLS requirement (smtp.PlainAuth also exempts localhost).
		if _, isTLS := client.TLSConnectionState(); !isTLS && !isLocalhost(host) {
			return errors.New("smtp auth: refusing LOGIN over an unencrypted connection")
		}
		auth = loginAuth{user: cfg.User, password: cfg.Password}
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	return nil
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func hasMechanism(advertised, mechanism string) bool {
	for _, item := range strings.Fields(advertised) {
		if strings.EqualFold(item, mechanism) {
			return true
		}
	}
	return false
}

// XOAuth2InitialResponse is the SASL XOAUTH2 client message, shared by SMTP
// and IMAP (https://learn.microsoft.com/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth).
func XOAuth2InitialResponse(user, token string) []byte {
	return []byte("user=" + user + "\x01auth=Bearer " + token + "\x01\x01")
}

type xoauth2Auth struct {
	user, token string
}

func (a xoauth2Auth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", XOAuth2InitialResponse(a.user, a.token), nil
}

// Next answers the server's error challenge (a base64 JSON status) with an
// empty response, after which the server returns the final failure code.
func (a xoauth2Auth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return []byte{}, nil
	}
	return nil, nil
}

type loginAuth struct {
	user, password string
}

func (a loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.HasPrefix(prompt, "username"):
		return []byte(a.user), nil
	case strings.HasPrefix(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN challenge %q", fromServer)
	}
}

// XOAuth2SASL is a SASL client (github.com/emersion/go-sasl Client) for
// IMAP AUTHENTICATE XOAUTH2.
type XOAuth2SASL struct {
	User, Token string
}

func (c XOAuth2SASL) Start() (string, []byte, error) {
	return "XOAUTH2", XOAuth2InitialResponse(c.User, c.Token), nil
}

func (c XOAuth2SASL) Next([]byte) ([]byte, error) {
	return []byte{}, nil
}
