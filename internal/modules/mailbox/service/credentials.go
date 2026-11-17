package service

import (
	"context"
	"errors"
	"time"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
)

// accessTokenMinLifetime: a cached access token closer than this to expiry
// is refreshed first, so it can't expire during a slow SMTP/IMAP session.
const accessTokenMinLifetime = 3 * time.Minute

// mailCredentials is what SMTP/IMAP login needs: Password for password
// mailboxes, AccessToken (SASL XOAUTH2) for OAuth ones.
type mailCredentials struct {
	Username    string
	Password    string
	AccessToken string
}

var errMicrosoftOAuthUnavailable = errors.New("microsoft oauth is not configured on this server")

// credentialResolver turns a stored mailbox into login credentials,
// refreshing and persisting OAuth tokens as needed. Shared by sending,
// inbox sync and the connection test.
type credentialResolver struct {
	secretKey string
	mailboxes repository.MailboxRepository
	microsoft MicrosoftOAuth
	now       func() time.Time
}

func newCredentialResolver(secretKey string, mailboxes repository.MailboxRepository, microsoft MicrosoftOAuth) *credentialResolver {
	return &credentialResolver{secretKey: secretKey, mailboxes: mailboxes, microsoft: microsoft, now: time.Now}
}

func (r *credentialResolver) resolve(ctx context.Context, scope coretenant.Scope, mailbox domain.Mailbox) (mailCredentials, error) {
	if !mailbox.AuthMethod.IsOAuth() {
		password, err := corecrypto.DecryptSecret(r.secretKey, mailbox.SecretEncrypted)
		if err != nil {
			return mailCredentials{}, err
		}
		return mailCredentials{Username: mailbox.Username, Password: password}, nil
	}

	if mailbox.AccessTokenEncrypted != "" && mailbox.AccessTokenExpiresAt != nil &&
		mailbox.AccessTokenExpiresAt.After(r.now().Add(accessTokenMinLifetime)) {
		if token, err := corecrypto.DecryptSecret(r.secretKey, mailbox.AccessTokenEncrypted); err == nil {
			return mailCredentials{Username: mailbox.Username, AccessToken: token}, nil
		}
	}

	if r.microsoft == nil {
		return mailCredentials{}, errMicrosoftOAuthUnavailable
	}
	refreshToken, err := corecrypto.DecryptSecret(r.secretKey, mailbox.SecretEncrypted)
	if err != nil {
		return mailCredentials{}, err
	}
	token, err := r.microsoft.Refresh(ctx, refreshToken)
	if err != nil {
		return mailCredentials{}, err
	}
	params, err := r.encryptTokens(token)
	if err != nil {
		return mailCredentials{}, err
	}
	// Two refreshes racing (worker sync + a send) both get valid tokens;
	// Microsoft keeps earlier refresh tokens valid, so last write wins.
	if err := r.mailboxes.UpdateOAuthTokens(ctx, scope, mailbox.ID, params); err != nil {
		return mailCredentials{}, err
	}
	return mailCredentials{Username: mailbox.Username, AccessToken: token.AccessToken}, nil
}

// encryptTokens prepares a token set for storage. RefreshTokenEncrypted is
// empty when the response carried no new refresh token (keep the old one).
func (r *credentialResolver) encryptTokens(token OAuthToken) (repository.OAuthTokenParams, error) {
	accessEncrypted, err := corecrypto.EncryptSecret(r.secretKey, token.AccessToken)
	if err != nil {
		return repository.OAuthTokenParams{}, err
	}
	params := repository.OAuthTokenParams{
		AccessTokenEncrypted: accessEncrypted,
		AccessTokenExpiresAt: tokenExpiry(token, r.now()),
	}
	if token.RefreshToken != "" {
		if params.RefreshTokenEncrypted, err = corecrypto.EncryptSecret(r.secretKey, token.RefreshToken); err != nil {
			return repository.OAuthTokenParams{}, err
		}
	}
	return params, nil
}

// tokenExpiry stores UTC (the DB session is pinned to UTC). A response
// without expires_in is treated as already expired so it is refreshed on
// next use instead of trusted indefinitely.
func tokenExpiry(token OAuthToken, now time.Time) time.Time {
	if token.Expiry.IsZero() {
		return now.UTC()
	}
	return token.Expiry.UTC()
}
