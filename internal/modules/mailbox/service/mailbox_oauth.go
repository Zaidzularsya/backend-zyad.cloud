package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
)

// oauthStateTTL bounds how long the user may stay on Microsoft's sign-in
// page before the round trip is rejected.
const oauthStateTTL = 10 * time.Minute

// oauthStateKeySuffix separates the state's encryption key from the one
// protecting stored secrets.
const oauthStateKeySuffix = "|mailbox-oauth-state"

// oauthState travels through Microsoft's redirect as the opaque `state`
// parameter, encrypted (it carries the PKCE verifier). Binding it to the
// user and organization is what stops a forged callback from attaching
// someone else's Microsoft account to the caller (or vice versa): the
// callback page posts it back with the caller's own session.
type oauthState struct {
	UserID         string `json:"u"`
	OrganizationID string `json:"o"`
	MailboxID      string `json:"m,omitempty"`
	DisplayName    string `json:"d,omitempty"`
	Verifier       string `json:"v"`
	ExpiresAt      int64  `json:"e"`
}

// MicrosoftOAuthInput starts a connect (MailboxID empty) or a reconnect of
// an existing mailbox.
type MicrosoftOAuthInput struct {
	MailboxID   string
	DisplayName string
}

// MicrosoftOAuthResult is a connected mailbox; Created is false when an
// existing mailbox (reconnect, or the same address connected with a
// password before) was switched to the new grant.
type MicrosoftOAuthResult struct {
	Mailbox domain.Mailbox
	Created bool
}

func (s *MailboxService) MicrosoftOAuthEnabled() bool {
	return s.microsoft != nil
}

// StartMicrosoftOAuth returns the Microsoft sign-in URL to send the
// browser to.
func (s *MailboxService) StartMicrosoftOAuth(ctx context.Context, scope coretenant.Scope, userID string, input MicrosoftOAuthInput) (string, error) {
	if s.microsoft == nil {
		return "", mailboxmodule.ErrOAuthNotConfigured
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if len(displayName) > 150 {
		return "", validationError("display_name must be at most 150 characters")
	}
	loginHint := ""
	if input.MailboxID != "" {
		current, err := s.get(ctx, scope, userID, input.MailboxID)
		if err != nil {
			return "", err
		}
		loginHint = current.EmailAddress
	}

	verifier := oauth2.GenerateVerifier()
	encoded, err := json.Marshal(oauthState{
		UserID:         userID,
		OrganizationID: scope.OrganizationID(),
		MailboxID:      input.MailboxID,
		DisplayName:    displayName,
		Verifier:       verifier,
		ExpiresAt:      s.now().Add(oauthStateTTL).Unix(),
	})
	if err != nil {
		return "", err
	}
	state, err := corecrypto.EncryptSecret(s.secretKey+oauthStateKeySuffix, string(encoded))
	if err != nil {
		return "", err
	}
	return s.microsoft.AuthCodeURL(state, verifier, loginHint), nil
}

// CompleteMicrosoftOAuth exchanges the code Microsoft redirected back with,
// verifies SMTP with the new token, and stores the mailbox.
func (s *MailboxService) CompleteMicrosoftOAuth(ctx context.Context, scope coretenant.Scope, userID, code, rawState string) (MicrosoftOAuthResult, error) {
	if s.microsoft == nil {
		return MicrosoftOAuthResult{}, mailboxmodule.ErrOAuthNotConfigured
	}
	state, err := s.decodeOAuthState(rawState)
	if err != nil || state.UserID != userID || state.OrganizationID != scope.OrganizationID() {
		return MicrosoftOAuthResult{}, mailboxmodule.ErrOAuthStateInvalid
	}
	if strings.TrimSpace(code) == "" {
		return MicrosoftOAuthResult{}, validationError("code is required")
	}

	token, err := s.microsoft.Exchange(ctx, code, state.Verifier)
	if err != nil {
		return MicrosoftOAuthResult{}, mailboxmodule.OAuthFailed(err.Error())
	}
	if token.RefreshToken == "" {
		// Without offline_access consent the mailbox would stop working
		// after an hour.
		return MicrosoftOAuthResult{}, mailboxmodule.OAuthFailed("microsoft did not grant offline access")
	}

	displayName := state.DisplayName
	if displayName == "" {
		displayName = token.Name
	}
	input := normalizeMailboxInput(MailboxInput{
		EmailAddress: token.Email,
		DisplayName:  displayName,
		Username:     token.Username,
		SMTPHost:     microsoftSMTPHost(token.TenantID),
		SMTPPort:     587,
		SMTPSecurity: domain.SecuritySTARTTLS,
		IMAPHost:     microsoftIMAPHost,
		IMAPPort:     993,
		IMAPSecurity: domain.SecuritySSL,
	})
	if len(input.DisplayName) > 150 {
		input.DisplayName = input.DisplayName[:150]
	}
	if err := validateMailboxInput(input); err != nil {
		return MicrosoftOAuthResult{}, err
	}
	if err := s.checkConnection(ctx, input, mailCredentials{Username: input.Username, AccessToken: token.AccessToken}); err != nil {
		return MicrosoftOAuthResult{}, err
	}

	params, err := s.oauthMailboxParams(input, token)
	if err != nil {
		return MicrosoftOAuthResult{}, err
	}

	if state.MailboxID != "" {
		current, err := s.get(ctx, scope, userID, state.MailboxID)
		if err != nil {
			return MicrosoftOAuthResult{}, err
		}
		if current.EmailAddress != input.EmailAddress {
			return MicrosoftOAuthResult{}, mailboxmodule.OAuthAccountMismatch(current.EmailAddress, input.EmailAddress)
		}
		if state.DisplayName == "" && current.DisplayName != "" {
			params.DisplayName = current.DisplayName
		}
		mailbox, err := s.update(ctx, scope, userID, current.ID, params)
		return MicrosoftOAuthResult{Mailbox: mailbox}, err
	}

	mailbox, err := s.mailboxes.Create(ctx, scope, userID, params)
	if errors.Is(err, repository.ErrDuplicateMailbox) {
		// Same address connected before (e.g. with a password that
		// Microsoft now rejects): switch that mailbox to OAuth so its
		// email history stays attached.
		return s.replaceWithOAuth(ctx, scope, userID, params, state.DisplayName != "")
	}
	if err != nil {
		return MicrosoftOAuthResult{}, err
	}
	return MicrosoftOAuthResult{Mailbox: mailbox, Created: true}, nil
}

func (s *MailboxService) replaceWithOAuth(ctx context.Context, scope coretenant.Scope, userID string, params repository.MailboxParams, keepNewName bool) (MicrosoftOAuthResult, error) {
	existing, err := s.mailboxes.ListByUser(ctx, scope, userID)
	if err != nil {
		return MicrosoftOAuthResult{}, err
	}
	for _, current := range existing {
		if strings.EqualFold(current.EmailAddress, params.EmailAddress) {
			if !keepNewName && current.DisplayName != "" {
				params.DisplayName = current.DisplayName
			}
			mailbox, err := s.update(ctx, scope, userID, current.ID, params)
			return MicrosoftOAuthResult{Mailbox: mailbox}, err
		}
	}
	return MicrosoftOAuthResult{}, mailboxmodule.ErrMailboxDuplicate
}

func (s *MailboxService) update(ctx context.Context, scope coretenant.Scope, userID, id string, params repository.MailboxParams) (domain.Mailbox, error) {
	mailbox, err := s.mailboxes.Update(ctx, scope, userID, id, params)
	if errors.Is(err, repository.ErrDuplicateMailbox) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxDuplicate
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxNotFound
	}
	return mailbox, err
}

func (s *MailboxService) oauthMailboxParams(input MailboxInput, token OAuthToken) (repository.MailboxParams, error) {
	refreshEncrypted, err := corecrypto.EncryptSecret(s.secretKey, token.RefreshToken)
	if err != nil {
		return repository.MailboxParams{}, err
	}
	tokens, err := s.creds.encryptTokens(token)
	if err != nil {
		return repository.MailboxParams{}, err
	}
	params := toParams(input, refreshEncrypted)
	params.AuthMethod = domain.AuthMethodMicrosoftOAuth
	params.AccessTokenEncrypted = tokens.AccessTokenEncrypted
	expiresAt := tokens.AccessTokenExpiresAt
	params.AccessTokenExpiresAt = &expiresAt
	return params, nil
}

func (s *MailboxService) decodeOAuthState(raw string) (oauthState, error) {
	plain, err := corecrypto.DecryptSecret(s.secretKey+oauthStateKeySuffix, strings.TrimSpace(raw))
	if err != nil {
		return oauthState{}, err
	}
	var state oauthState
	if err := json.Unmarshal([]byte(plain), &state); err != nil {
		return oauthState{}, err
	}
	if state.Verifier == "" || s.now().Unix() > state.ExpiresAt {
		return oauthState{}, errors.New("oauth state expired")
	}
	return state, nil
}
