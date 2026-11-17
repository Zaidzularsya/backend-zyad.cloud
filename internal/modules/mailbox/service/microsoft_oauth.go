package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Microsoft identity platform OAuth for Outlook.com and Microsoft 365
// mailboxes, which no longer accept basic auth for SMTP/IMAP. The scopes
// are the Exchange Online ones documented for SASL XOAUTH2:
// https://learn.microsoft.com/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth
var microsoftScopes = []string{
	"openid", "email", "profile", "offline_access",
	"https://outlook.office.com/SMTP.Send",
	"https://outlook.office.com/IMAP.AccessAsUser.All",
}

// consumerTenantID is the Entra tenant every personal Microsoft account
// (Outlook.com, Hotmail, Live) signs in from.
const consumerTenantID = "9188040d-6c67-4c5b-b112-36a304b66dad"

// Server settings Microsoft documents for OAuth clients
// (support.microsoft.com "POP, IMAP, and SMTP settings for Outlook.com").
const (
	microsoftConsumerSMTPHost = "smtp-mail.outlook.com"
	microsoftWorkSMTPHost     = "smtp.office365.com"
	microsoftIMAPHost         = "outlook.office365.com"
)

// OAuthToken is a token response with the signed-in account it belongs to.
type OAuthToken struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	// Email/Username/Name/TenantID come from the id_token and are only set
	// by Exchange (a refresh response carries no id_token we rely on).
	Email    string
	Username string
	Name     string
	TenantID string
}

// ErrOAuthGrantRevoked: the refresh token no longer works (password
// changed, consent revoked, 90 days unused); the user must reconnect.
var ErrOAuthGrantRevoked = errors.New("microsoft authorization expired or was revoked; reconnect the account")

// MicrosoftOAuth is the authorization-code client (with PKCE).
type MicrosoftOAuth interface {
	AuthCodeURL(state, verifier, loginHint string) string
	Exchange(ctx context.Context, code, verifier string) (OAuthToken, error)
	Refresh(ctx context.Context, refreshToken string) (OAuthToken, error)
}

type MicrosoftOAuthConfig struct {
	ClientID     string
	ClientSecret string
	// Tenant is "common" (personal + work accounts), "organizations",
	// "consumers" or one tenant id.
	Tenant      string
	RedirectURL string
}

type microsoftOAuth struct {
	config oauth2.Config
	client *http.Client
}

// NewMicrosoftOAuth returns nil when no client id is configured; the
// Microsoft connect option is then unavailable.
func NewMicrosoftOAuth(cfg MicrosoftOAuthConfig) MicrosoftOAuth {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil
	}
	tenant := strings.TrimSpace(cfg.Tenant)
	if tenant == "" {
		tenant = "common"
	}
	base := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0"
	return &microsoftOAuth{
		config: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       microsoftScopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:   base + "/authorize",
				TokenURL:  base + "/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *microsoftOAuth) AuthCodeURL(state, verifier, loginHint string) string {
	options := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(verifier),
		// Always show the account picker: the user may be signed in to
		// Microsoft with another account than the one to connect.
		oauth2.SetAuthURLParam("prompt", "select_account"),
	}
	if loginHint != "" {
		options = append(options, oauth2.SetAuthURLParam("login_hint", loginHint))
	}
	return m.config.AuthCodeURL(state, options...)
}

func (m *microsoftOAuth) Exchange(ctx context.Context, code, verifier string) (OAuthToken, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.client)
	token, err := m.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return OAuthToken{}, describeOAuthError(err)
	}
	result := fromOAuth2Token(token)
	idToken, _ := token.Extra("id_token").(string)
	claims, err := parseIDTokenClaims(idToken)
	if err != nil {
		return OAuthToken{}, err
	}
	result.Username = strings.ToLower(strings.TrimSpace(claims.PreferredUsername))
	result.Email = strings.ToLower(strings.TrimSpace(claims.Email))
	if result.Email == "" {
		result.Email = result.Username
	}
	if result.Username == "" {
		result.Username = result.Email
	}
	result.Name = strings.TrimSpace(claims.Name)
	result.TenantID = claims.TenantID
	if result.Email == "" {
		return OAuthToken{}, errors.New("microsoft did not return the account's email address")
	}
	return result, nil
}

func (m *microsoftOAuth) Refresh(ctx context.Context, refreshToken string) (OAuthToken, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.client)
	token, err := m.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
			return OAuthToken{}, ErrOAuthGrantRevoked
		}
		return OAuthToken{}, describeOAuthError(err)
	}
	return fromOAuth2Token(token), nil
}

func fromOAuth2Token(token *oauth2.Token) OAuthToken {
	return OAuthToken{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Expiry: token.Expiry}
}

// describeOAuthError keeps Microsoft's error code and description (e.g.
// AADSTS65001 consent required) but not the raw response body.
func describeOAuthError(err error) error {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode != "" {
		description, _, _ := strings.Cut(retrieveErr.ErrorDescription, "\r\n")
		return fmt.Errorf("microsoft oauth: %s: %s", retrieveErr.ErrorCode, description)
	}
	return fmt.Errorf("microsoft oauth: %w", err)
}

type idTokenClaims struct {
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	TenantID          string `json:"tid"`
}

// parseIDTokenClaims reads the id_token payload without checking its
// signature: it came straight from the token endpoint over TLS in the
// code exchange, which OpenID Connect Core 3.1.3.7 accepts in place of
// signature validation. It is only used to name the mailbox.
func parseIDTokenClaims(idToken string) (idTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return idTokenClaims{}, errors.New("microsoft did not return an id_token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, fmt.Errorf("decode id_token: %w", err)
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return idTokenClaims{}, fmt.Errorf("decode id_token: %w", err)
	}
	return claims, nil
}

func microsoftSMTPHost(tenantID string) string {
	if tenantID == consumerTenantID {
		return microsoftConsumerSMTPHost
	}
	return microsoftWorkSMTPHost
}
