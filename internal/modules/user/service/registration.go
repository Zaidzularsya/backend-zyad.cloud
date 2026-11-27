package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

// bcrypt only hashes the first 72 bytes and newer versions reject longer input.
const maxPasswordBytes = 72

// SignupHooks lets other modules react to a newly created user (for example
// referral attribution). Implementations must not fail the sign-up.
type SignupHooks interface {
	OnUserRegistered(ctx context.Context, userID string, attribution *dto.AttributionInput)
}

func (s *AuthService) SetSignupHooks(h SignupHooks) {
	s.signupHooks = h
}

// notifyUserRegistered is best-effort: the user already exists, so a hook
// problem is logged and never surfaced to the client.
func (s *AuthService) notifyUserRegistered(ctx context.Context, userID string, attribution *dto.AttributionInput) {
	if s.signupHooks == nil {
		return
	}
	s.signupHooks.OnUserRegistered(ctx, userID, attribution)
}

// Register creates an active but unverified account from email + password and
// signs it in. Verification email is queued best-effort; the user can resend it.
func (s *AuthService) Register(ctx context.Context, req dto.RegisterRequest, metadata LoginHistoryRecord) (dto.GoogleAuthResponse, error) {
	if !s.passwordRegisterOn {
		return dto.GoogleAuthResponse{}, coreerrors.New("AUTH_REGISTRATION_DISABLED", "password registration is disabled", http.StatusForbidden)
	}
	intent, err := dto.NormalizeIntent(req.Intent)
	if err != nil {
		return dto.GoogleAuthResponse{}, err
	}
	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if name == "" || email == "" || req.Password == "" {
		return dto.GoogleAuthResponse{}, coreerrors.New("VALIDATION_ERROR", "name, email, and password are required", http.StatusUnprocessableEntity)
	}
	if len(req.Password) < s.passwordMinLength {
		return dto.GoogleAuthResponse{}, coreerrors.New(
			"AUTH_PASSWORD_POLICY_FAILED",
			fmt.Sprintf("password must be at least %d characters", s.passwordMinLength),
			http.StatusUnprocessableEntity,
		)
	}
	if len(req.Password) > maxPasswordBytes {
		return dto.GoogleAuthResponse{}, coreerrors.New(
			"AUTH_PASSWORD_POLICY_FAILED",
			fmt.Sprintf("password must be at most %d bytes", maxPasswordBytes),
			http.StatusUnprocessableEntity,
		)
	}
	passwordHash, err := coreauth.HashPassword(req.Password)
	if err != nil {
		return dto.GoogleAuthResponse{}, coreerrors.Wrap("AUTH_PASSWORD_HASH_FAILED", "failed to hash password", http.StatusInternalServerError, err)
	}

	now := s.now().UTC()
	user, err := s.repo.CreatePasswordUser(ctx, PasswordUserRegistration{
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		UsernameBase: googleUsernameBase(email),
		RoleSlug:     s.googleDefaultRole,
		IPAddress:    metadata.IPAddress,
		UserAgent:    metadata.UserAgent,
		CreatedAt:    now,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			return dto.GoogleAuthResponse{}, coreerrors.New("AUTH_EMAIL_TAKEN", "email is already registered", http.StatusConflict)
		case errors.Is(err, ErrGoogleDefaultRoleNotFound):
			return dto.GoogleAuthResponse{}, coreerrors.Wrap("AUTH_DEFAULT_ROLE_INVALID", "default role is invalid", http.StatusInternalServerError, err)
		}
		return dto.GoogleAuthResponse{}, coreerrors.Wrap("AUTH_REGISTER_FAILED", "failed to register user", http.StatusInternalServerError, err)
	}

	refreshTTL := s.refreshTTL
	if req.RememberMe {
		refreshTTL = s.rememberMeTTL
	}
	token, err := s.issueLoginTokens(ctx, loginTokenUser{
		ID:          user.ID,
		Name:        user.Name,
		Email:       user.Email,
		Username:    user.Username,
		Status:      user.Status,
		Roles:       user.Roles,
		Permissions: user.Permissions,
	}, refreshTTL, firstNonEmpty(req.DeviceName, metadata.DeviceName), metadata, now)
	if err != nil {
		return dto.GoogleAuthResponse{}, err
	}

	s.notifyUserRegistered(ctx, user.ID, req.Attribution)

	if err := s.issueEmailVerification(ctx, user.ID, user.Name, user.Email); err != nil {
		slog.Warn("register: verification email not queued", "user_id", user.ID, "error", err)
	}

	if intent == dto.IntentOperator && s.googleWorkspace != nil {
		if err := s.googleWorkspace.ProvisionGoogleWorkspace(
			ctx,
			user.ID,
			token.SessionID,
			firstNonEmpty(user.Name, user.Email),
			metadata,
		); err != nil {
			return dto.GoogleAuthResponse{}, err
		}
	}

	s.recordLoginAttempt(ctx, LoginHistoryRecord{
		UserID:     user.ID,
		Identifier: user.Email,
		Event:      model.LoginEventLogin,
		Success:    true,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		DeviceName: metadata.DeviceName,
		Reason:     "register",
	})

	return dto.GoogleAuthResponse{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresIn:    token.ExpiresIn,
		IsNewUser:    true,
		User:         token.User,
	}, nil
}
