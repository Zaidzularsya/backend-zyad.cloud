package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"testing"
)

// fakeSMTPServer advertises authMechanisms and records the AUTH exchange.
// It accepts any credentials.
func fakeSMTPServer(t *testing.T, authMechanisms string) (string, <-chan []string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	lines := make(chan []string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		var seen []string
		defer func() { lines <- seen }()
		write("220 fake ESMTP")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			seen = append(seen, line)
			switch upper := strings.ToUpper(line); {
			case strings.HasPrefix(upper, "EHLO"):
				write("250-fake")
				write("250 AUTH " + authMechanisms)
			case upper == "AUTH LOGIN":
				write("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
				if user, _ := reader.ReadString('\n'); user != "" {
					seen = append(seen, "user:"+decode(user))
				}
				write("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
				if password, _ := reader.ReadString('\n'); password != "" {
					seen = append(seen, "pass:"+decode(password))
				}
				write("235 ok")
			case strings.HasPrefix(upper, "AUTH "):
				write("235 ok")
			case upper == "QUIT":
				write("221 bye")
				return
			default:
				write("250 ok")
			}
		}
	}()
	return listener.Addr().String(), lines
}

func decode(line string) string {
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line))
	return string(raw)
}

func verifyAgainst(t *testing.T, mechanisms string, cfg SMTPConfig) []string {
	t.Helper()
	address, lines := fakeSMTPServer(t, mechanisms)
	_, port, _ := net.SplitHostPort(address)
	// "localhost" so PLAIN/LOGIN are allowed without TLS, as in net/smtp.
	cfg.Host = "localhost"
	cfg.Port, _ = strconv.Atoi(port)
	if err := VerifySMTP(context.Background(), cfg); err != nil {
		t.Fatalf("verify: %v", err)
	}
	return <-lines
}

func authLine(lines []string) string {
	for _, line := range lines {
		if strings.HasPrefix(strings.ToUpper(line), "AUTH ") {
			return line
		}
	}
	return ""
}

func TestAuthenticateFallsBackToLoginWhenPlainIsNotOffered(t *testing.T) {
	// smtp.office365.com advertises "AUTH LOGIN XOAUTH2"; PLAIN gets 504 5.7.4.
	lines := verifyAgainst(t, "LOGIN XOAUTH2", SMTPConfig{User: "lisa@contoso.com", Password: "secret"})
	if authLine(lines) != "AUTH LOGIN" {
		t.Fatalf("expected AUTH LOGIN, got %v", lines)
	}
	joined := strings.Join(lines, "|")
	if !strings.Contains(joined, "user:lisa@contoso.com") || !strings.Contains(joined, "pass:secret") {
		t.Fatalf("unexpected LOGIN exchange %v", lines)
	}
}

func TestAuthenticatePrefersPlain(t *testing.T) {
	lines := verifyAgainst(t, "PLAIN LOGIN", SMTPConfig{User: "u", Password: "p"})
	if !strings.HasPrefix(authLine(lines), "AUTH PLAIN") {
		t.Fatalf("expected AUTH PLAIN, got %v", lines)
	}
}

func TestAuthenticateUsesXOAuth2WithAccessToken(t *testing.T) {
	lines := verifyAgainst(t, "LOGIN XOAUTH2", SMTPConfig{User: "lisa@outlook.com", OAuthToken: "tok"})
	line := authLine(lines)
	if !strings.HasPrefix(line, "AUTH XOAUTH2 ") {
		t.Fatalf("expected AUTH XOAUTH2, got %v", lines)
	}
	if got := decode(strings.TrimPrefix(line, "AUTH XOAUTH2 ")); got != "user=lisa@outlook.com\x01auth=Bearer tok\x01\x01" {
		t.Fatalf("unexpected XOAUTH2 payload %q", got)
	}
}
