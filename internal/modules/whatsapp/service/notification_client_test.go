package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

type recordingSender struct{ sessions []string }

func (s *recordingSender) SendText(_ context.Context, message platformwhatsapp.Message) (platformwhatsapp.Result, error) {
	s.sessions = append(s.sessions, message.Session)
	return platformwhatsapp.Result{MessageID: "x"}, nil
}

func notificationHarness(t *testing.T) (*fakeSessionRepo, *fakeResolver, coretenant.Scope) {
	t.Helper()
	repo := newFakeSessionRepo()
	scope := testScope(t, testOrgID)
	return repo, &fakeResolver{scopes: map[string]coretenant.Context{testOrgID: scopeContext(t, testOrgID)}}, scope
}

func addSession(t *testing.T, repo *fakeSessionRepo, scope coretenant.Scope, name string, purpose domain.SessionPurpose, status domain.SessionStatus) {
	t.Helper()
	session, err := repo.Create(context.Background(), scope, repository.CreateSessionParams{Name: name, Purpose: purpose})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	session.Status = status
	repo.sessions[session.ID] = session
}

func TestNotificationClientUsesPlatformNotificationSession(t *testing.T) {
	repo, resolver, scope := notificationHarness(t)
	addSession(t, repo, scope, "zc_sales", domain.SessionPurposeSales, domain.SessionStatusWorking)
	addSession(t, repo, scope, "zc_notif_down", domain.SessionPurposeNotification, domain.SessionStatusFailed)
	addSession(t, repo, scope, "zc_notif", domain.SessionPurposeNotification, domain.SessionStatusWorking)
	sender := &recordingSender{}

	client := NewNotificationClient(sender, repo, resolver, testOrgID, "env_session", nil)
	if _, err := client.SendText(context.Background(), platformwhatsapp.Message{To: "628123", Text: "hi"}); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if sender.sessions[0] != "zc_notif" {
		t.Fatalf("session = %q, want the connected notification session", sender.sessions[0])
	}
}

func TestNotificationClientFallsBackToEnvSession(t *testing.T) {
	repo, resolver, scope := notificationHarness(t)
	addSession(t, repo, scope, "zc_sales", domain.SessionPurposeSales, domain.SessionStatusWorking)
	sender := &recordingSender{}

	client := NewNotificationClient(sender, repo, resolver, testOrgID, "env_session", nil)
	if _, err := client.SendText(context.Background(), platformwhatsapp.Message{To: "628123", Text: "hi"}); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if sender.sessions[0] != "env_session" {
		t.Fatalf("session = %q, want fallback", sender.sessions[0])
	}
}

func TestNotificationClientWithoutAnySession(t *testing.T) {
	client := NewNotificationClient(&recordingSender{}, nil, nil, "", "", nil)
	if _, err := client.SendText(context.Background(), platformwhatsapp.Message{To: "628123", Text: "hi"}); !errors.Is(err, ErrNoNotificationSession) {
		t.Fatalf("error = %v, want ErrNoNotificationSession", err)
	}
}

func TestNotificationClientKeepsExplicitSession(t *testing.T) {
	sender := &recordingSender{}
	client := NewNotificationClient(sender, nil, nil, "", "", nil)
	if _, err := client.SendText(context.Background(), platformwhatsapp.Message{Session: "zc_x", To: "628123", Text: "hi"}); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if sender.sessions[0] != "zc_x" {
		t.Fatalf("session = %q", sender.sessions[0])
	}
}
