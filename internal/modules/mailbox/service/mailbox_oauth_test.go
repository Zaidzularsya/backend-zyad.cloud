package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
	platformmail "zyad.cloud/internal/platform/mail"
)

type fakeMicrosoft struct {
	exchange     OAuthToken
	exchangeErr  error
	refresh      OAuthToken
	refreshErr   error
	refreshCalls int
	lastVerifier string
}

func (f *fakeMicrosoft) AuthCodeURL(state, verifier, loginHint string) string {
	f.lastVerifier = verifier
	return "https://login.example/authorize?" + url.Values{"state": {state}, "login_hint": {loginHint}}.Encode()
}

func (f *fakeMicrosoft) Exchange(_ context.Context, _ string, verifier string) (OAuthToken, error) {
	if verifier != f.lastVerifier {
		return OAuthToken{}, errors.New("pkce verifier mismatch")
	}
	return f.exchange, f.exchangeErr
}

func (f *fakeMicrosoft) Refresh(context.Context, string) (OAuthToken, error) {
	f.refreshCalls++
	return f.refresh, f.refreshErr
}

// oauthMailboxes is an in-memory MailboxRepository for the OAuth flow.
type oauthMailboxes struct {
	repository.MailboxRepository
	byID    map[string]domain.Mailbox
	tokens  []repository.OAuthTokenParams
	updated []repository.MailboxParams
}

func (f *oauthMailboxes) GetForUser(_ context.Context, _ coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	mailbox, ok := f.byID[id]
	if !ok || mailbox.UserID != userID {
		return domain.Mailbox{}, pgx.ErrNoRows
	}
	return mailbox, nil
}

func (f *oauthMailboxes) ListByUser(_ context.Context, _ coretenant.Scope, userID string) ([]domain.Mailbox, error) {
	var out []domain.Mailbox
	for _, mailbox := range f.byID {
		if mailbox.UserID == userID {
			out = append(out, mailbox)
		}
	}
	return out, nil
}

func (f *oauthMailboxes) Create(_ context.Context, _ coretenant.Scope, userID string, p repository.MailboxParams) (domain.Mailbox, error) {
	for _, existing := range f.byID {
		if existing.UserID == userID && existing.EmailAddress == p.EmailAddress {
			return domain.Mailbox{}, repository.ErrDuplicateMailbox
		}
	}
	mailbox := mailboxFromParams("mb-new", userID, p)
	f.byID[mailbox.ID] = mailbox
	return mailbox, nil
}

func (f *oauthMailboxes) Update(_ context.Context, _ coretenant.Scope, userID, id string, p repository.MailboxParams) (domain.Mailbox, error) {
	if _, ok := f.byID[id]; !ok {
		return domain.Mailbox{}, pgx.ErrNoRows
	}
	f.updated = append(f.updated, p)
	mailbox := mailboxFromParams(id, userID, p)
	f.byID[id] = mailbox
	return mailbox, nil
}

func (f *oauthMailboxes) UpdateOAuthTokens(_ context.Context, _ coretenant.Scope, _ string, tokens repository.OAuthTokenParams) error {
	f.tokens = append(f.tokens, tokens)
	return nil
}

func mailboxFromParams(id, userID string, p repository.MailboxParams) domain.Mailbox {
	return domain.Mailbox{
		ID: id, UserID: userID, EmailAddress: p.EmailAddress, DisplayName: p.DisplayName, Username: p.Username,
		SecretEncrypted: p.SecretEncrypted, AuthMethod: p.AuthMethod,
		AccessTokenEncrypted: p.AccessTokenEncrypted, AccessTokenExpiresAt: p.AccessTokenExpiresAt,
		SMTPHost: p.SMTPHost, SMTPPort: p.SMTPPort, SMTPSecurity: p.SMTPSecurity,
		IMAPHost: p.IMAPHost, IMAPPort: p.IMAPPort, IMAPSecurity: p.IMAPSecurity,
	}
}

var oauthNow = time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)

func newOAuthHarness(t *testing.T) (*MailboxService, *oauthMailboxes, *fakeMicrosoft, *[]platformmail.SMTPConfig) {
	t.Helper()
	repo := &oauthMailboxes{byID: map[string]domain.Mailbox{}}
	microsoft := &fakeMicrosoft{exchange: OAuthToken{
		AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: oauthNow.Add(time.Hour),
		Email: "Lisa@Outlook.com", Username: "lisa@outlook.com", Name: "Lisa", TenantID: consumerTenantID,
	}}
	verified := &[]platformmail.SMTPConfig{}
	svc := NewMailboxService(repo, newFakeMessages(), &fakeStorage{}, MailboxServiceOptions{
		SecretKey: testSecret, MicrosoftOAuth: microsoft, Now: func() time.Time { return oauthNow },
		Verify: func(_ context.Context, cfg platformmail.SMTPConfig) error {
			*verified = append(*verified, cfg)
			return nil
		},
	})
	return svc, repo, microsoft, verified
}

func startState(t *testing.T, svc *MailboxService, userID string, input MicrosoftOAuthInput) string {
	t.Helper()
	raw, err := svc.StartMicrosoftOAuth(context.Background(), testScope, userID, input)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query().Get("state")
}

func TestMicrosoftOAuthConnectsNewMailbox(t *testing.T) {
	svc, repo, _, verified := newOAuthHarness(t)
	state := startState(t, svc, "user-1", MicrosoftOAuthInput{})

	result, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-1", "code-1", state)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !result.Created {
		t.Fatal("expected a new mailbox")
	}
	mailbox := repo.byID[result.Mailbox.ID]
	if mailbox.AuthMethod != domain.AuthMethodMicrosoftOAuth || mailbox.EmailAddress != "lisa@outlook.com" || mailbox.DisplayName != "Lisa" {
		t.Fatalf("unexpected mailbox: %+v", mailbox)
	}
	if mailbox.SMTPHost != microsoftConsumerSMTPHost || mailbox.SMTPPort != 587 || mailbox.IMAPHost != microsoftIMAPHost {
		t.Fatalf("unexpected servers: %+v", mailbox)
	}
	if refresh, _ := corecrypto.DecryptSecret(testSecret, mailbox.SecretEncrypted); refresh != "refresh-1" {
		t.Fatalf("refresh token not stored encrypted, got %q", refresh)
	}
	if len(*verified) != 1 || (*verified)[0].OAuthToken != "access-1" || (*verified)[0].Password != "" {
		t.Fatalf("SMTP must be verified with XOAUTH2, got %+v", *verified)
	}
}

func TestMicrosoftOAuthRejectsStateOfAnotherUser(t *testing.T) {
	svc, _, _, _ := newOAuthHarness(t)
	state := startState(t, svc, "user-1", MicrosoftOAuthInput{})

	_, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-2", "code-1", state)
	if !errors.Is(err, mailboxmodule.ErrOAuthStateInvalid) {
		t.Fatalf("expected ErrOAuthStateInvalid, got %v", err)
	}
	if _, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-1", "code-1", "tampered"+state); !errors.Is(err, mailboxmodule.ErrOAuthStateInvalid) {
		t.Fatalf("tampered state: expected ErrOAuthStateInvalid, got %v", err)
	}
}

func TestMicrosoftOAuthRejectsExpiredState(t *testing.T) {
	svc, _, _, _ := newOAuthHarness(t)
	state := startState(t, svc, "user-1", MicrosoftOAuthInput{})
	svc.now = func() time.Time { return oauthNow.Add(oauthStateTTL + time.Second) }

	if _, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-1", "code-1", state); !errors.Is(err, mailboxmodule.ErrOAuthStateInvalid) {
		t.Fatalf("expected ErrOAuthStateInvalid, got %v", err)
	}
}

func TestMicrosoftOAuthUpgradesPasswordMailboxWithSameAddress(t *testing.T) {
	svc, repo, _, _ := newOAuthHarness(t)
	repo.byID["mb-1"] = domain.Mailbox{
		ID: "mb-1", UserID: "user-1", EmailAddress: "lisa@outlook.com", DisplayName: "Lisa Sales",
		AuthMethod: domain.AuthMethodPassword,
	}
	state := startState(t, svc, "user-1", MicrosoftOAuthInput{})

	result, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-1", "code-1", state)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Created || result.Mailbox.ID != "mb-1" {
		t.Fatalf("expected mb-1 to be switched in place, got %+v", result)
	}
	if got := repo.byID["mb-1"]; got.AuthMethod != domain.AuthMethodMicrosoftOAuth || got.DisplayName != "Lisa Sales" {
		t.Fatalf("unexpected mailbox after upgrade: %+v", got)
	}
}

func TestMicrosoftOAuthReconnectRejectsOtherAccount(t *testing.T) {
	svc, repo, _, _ := newOAuthHarness(t)
	repo.byID["mb-1"] = domain.Mailbox{ID: "mb-1", UserID: "user-1", EmailAddress: "sales@contoso.com"}
	state := startState(t, svc, "user-1", MicrosoftOAuthInput{MailboxID: "mb-1"})

	_, err := svc.CompleteMicrosoftOAuth(context.Background(), testScope, "user-1", "code-1", state)
	if err == nil || !strings.Contains(err.Error(), "signed in as lisa@outlook.com") || len(repo.updated) != 0 {
		t.Fatalf("expected account mismatch error without update, got %v", err)
	}
}

func TestMicrosoftOAuthNotConfigured(t *testing.T) {
	svc := NewMailboxService(&oauthMailboxes{byID: map[string]domain.Mailbox{}}, newFakeMessages(), &fakeStorage{}, MailboxServiceOptions{SecretKey: testSecret})
	if svc.MicrosoftOAuthEnabled() {
		t.Fatal("expected disabled without a client")
	}
	if _, err := svc.StartMicrosoftOAuth(context.Background(), testScope, "user-1", MicrosoftOAuthInput{}); !errors.Is(err, mailboxmodule.ErrOAuthNotConfigured) {
		t.Fatalf("expected ErrOAuthNotConfigured, got %v", err)
	}
}

func oauthMailbox(t *testing.T, accessToken string, expiresAt time.Time) domain.Mailbox {
	t.Helper()
	refresh, _ := corecrypto.EncryptSecret(testSecret, "refresh-old")
	access, _ := corecrypto.EncryptSecret(testSecret, accessToken)
	return domain.Mailbox{
		ID: "mb-1", UserID: "user-1", Username: "lisa@outlook.com", AuthMethod: domain.AuthMethodMicrosoftOAuth,
		SecretEncrypted: refresh, AccessTokenEncrypted: access, AccessTokenExpiresAt: &expiresAt,
	}
}

func TestCredentialResolverUsesCachedAccessToken(t *testing.T) {
	repo := &oauthMailboxes{}
	microsoft := &fakeMicrosoft{}
	resolver := newCredentialResolver(testSecret, repo, microsoft)
	resolver.now = func() time.Time { return oauthNow }

	creds, err := resolver.resolve(context.Background(), testScope, oauthMailbox(t, "cached", oauthNow.Add(30*time.Minute)))
	if err != nil || creds.AccessToken != "cached" || microsoft.refreshCalls != 0 {
		t.Fatalf("expected cached token without refresh, got %+v err=%v calls=%d", creds, err, microsoft.refreshCalls)
	}
}

func TestCredentialResolverRefreshesAndStoresRotatedToken(t *testing.T) {
	repo := &oauthMailboxes{}
	microsoft := &fakeMicrosoft{refresh: OAuthToken{AccessToken: "fresh", RefreshToken: "refresh-new", Expiry: oauthNow.Add(time.Hour)}}
	resolver := newCredentialResolver(testSecret, repo, microsoft)
	resolver.now = func() time.Time { return oauthNow }

	creds, err := resolver.resolve(context.Background(), testScope, oauthMailbox(t, "stale", oauthNow.Add(time.Minute)))
	if err != nil || creds.AccessToken != "fresh" || microsoft.refreshCalls != 1 {
		t.Fatalf("expected refresh, got %+v err=%v calls=%d", creds, err, microsoft.refreshCalls)
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("expected tokens persisted once, got %d", len(repo.tokens))
	}
	if refresh, _ := corecrypto.DecryptSecret(testSecret, repo.tokens[0].RefreshTokenEncrypted); refresh != "refresh-new" {
		t.Fatalf("rotated refresh token not stored, got %q", refresh)
	}
}

func TestCredentialResolverReportsRevokedGrant(t *testing.T) {
	microsoft := &fakeMicrosoft{refreshErr: ErrOAuthGrantRevoked}
	resolver := newCredentialResolver(testSecret, &oauthMailboxes{}, microsoft)
	resolver.now = func() time.Time { return oauthNow }

	if _, err := resolver.resolve(context.Background(), testScope, oauthMailbox(t, "stale", oauthNow)); !errors.Is(err, ErrOAuthGrantRevoked) {
		t.Fatalf("expected ErrOAuthGrantRevoked, got %v", err)
	}
}

func TestParseIDTokenClaims(t *testing.T) {
	// header.payload.signature; payload = {"email":"a@b.com","preferred_username":"a@b.com","tid":"t1","name":"A"}
	token := "eyJhbGciOiJub25lIn0.eyJlbWFpbCI6ImFAYi5jb20iLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiJhQGIuY29tIiwidGlkIjoidDEiLCJuYW1lIjoiQSJ9.sig"
	claims, err := parseIDTokenClaims(token)
	if err != nil || claims.Email != "a@b.com" || claims.TenantID != "t1" || claims.Name != "A" {
		t.Fatalf("unexpected claims %+v err=%v", claims, err)
	}
	if _, err := parseIDTokenClaims(""); err == nil {
		t.Fatal("expected error for missing id_token")
	}
}
