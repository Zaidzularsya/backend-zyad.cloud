package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/middleware"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

var ErrLoginUserNotFound = errors.New("login user not found")
var ErrRefreshTokenNotFound = errors.New("refresh token not found")
var ErrCurrentUserNotFound = errors.New("current user not found")
var ErrPasswordResetUserNotFound = errors.New("password reset user not found")
var ErrPasswordResetTokenNotFound = errors.New("password reset token not found")
var ErrPasswordChangeUserNotFound = errors.New("password change user not found")

const passwordResetRequestedEvent = "auth.password_reset_requested"
const passwordChangedEvent = "auth.password_changed"

type LoginRepository interface {
	FindLoginUserByIdentifier(ctx context.Context, identifier string) (LoginUser, error)
	FindPasswordResetUserByEmail(ctx context.Context, email string) (PasswordResetUser, error)
	FindPasswordResetTokenByHash(ctx context.Context, tokenHash string) (PasswordResetToken, error)
	FindPasswordChangeUser(ctx context.Context, userID string) (PasswordChangeUser, error)
	FindRefreshSessionByTokenHash(ctx context.Context, tokenHash string) (RefreshSession, error)
	FindCurrentUser(ctx context.Context, userID string, sessionID string) (CurrentUser, error)
	CreateSession(ctx context.Context, session NewSession) (string, error)
	CreatePasswordResetToken(ctx context.Context, token NewPasswordResetToken) error
	CompletePasswordReset(ctx context.Context, tokenHash string, passwordHash string, usedAt time.Time) (PasswordResetUser, error)
	ChangePassword(ctx context.Context, change PasswordChange) error
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error
	RotateRefreshToken(ctx context.Context, rotation RefreshTokenRotation) error
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	RecordLoginHistory(ctx context.Context, history LoginHistoryRecord) error
}

type NotificationEventPublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

type LoginUser struct {
	ID           string
	Name         string
	Email        string
	Username     string
	PasswordHash string
	Status       model.UserStatus
	DeletedAt    *time.Time
	Roles        []string
	Permissions  []string
}

type NewSession struct {
	UserID           string
	RefreshTokenHash string
	DeviceName       string
	UserAgent        string
	IPAddress        string
	ExpiresAt        time.Time
}

type PasswordResetUser struct {
	ID     string
	Name   string
	Email  string
	Status model.UserStatus
}

type PasswordChangeUser struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
}

type PasswordChange struct {
	UserID             string
	CurrentSessionID   string
	PasswordHash       string
	LogoutOtherDevices bool
	ChangedAt          time.Time
	IPAddress          string
	UserAgent          string
}

type NewPasswordResetToken struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
}

type PasswordResetToken struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type RefreshSession struct {
	TokenID           string
	SessionID         string
	UserID            string
	Name              string
	Email             string
	Username          string
	Status            model.UserStatus
	TokenRevokedAt    *time.Time
	TokenReplacedByID string
	SessionRevokedAt  *time.Time
	SessionExpiresAt  time.Time
	Roles             []string
	Permissions       []string
}

type RefreshTokenRotation struct {
	OldTokenID        string
	SessionID         string
	UserID            string
	NewTokenHash      string
	RotatedAt         time.Time
	NewTokenExpiresAt time.Time
}

type LoginHistoryRecord struct {
	UserID     string
	Identifier string
	Event      model.LoginEvent
	Success    bool
	IPAddress  string
	UserAgent  string
	DeviceName string
	Reason     string
}

type CurrentUser struct {
	ID              string
	Name            string
	Email           string
	Username        string
	Phone           string
	Status          model.UserStatus
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	Profile         CurrentUserProfile
	Roles           []string
	Permissions     []string
	Session         CurrentSession
}

type CurrentUserProfile struct {
	AvatarURL  string
	Bio        string
	JobTitle   string
	Department string
	Company    string
	Address    string
	Timezone   string
	Language   string
}

type CurrentSession struct {
	ID         string
	DeviceName string
	IPAddress  string
	LastUsedAt *time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

type AuthService struct {
	repo                  LoginRepository
	notificationPublisher NotificationEventPublisher
	accessTokenManager    *coreauth.TokenManager
	refreshTokenManager   *coreauth.TokenManager
	accessTTL             time.Duration
	refreshTTL            time.Duration
	rememberMeTTL         time.Duration
	resetTokenTTL         time.Duration
	passwordMinLength     int
	appName               string
	frontendURL           string
	notificationLocale    string
	notificationAttempts  int
	now                   func() time.Time
}

func NewAuthService(repo LoginRepository, cfg config.Config) (*AuthService, error) {
	accessManager, err := coreauth.NewTokenManager(cfg.Auth.Secret, cfg.App.Name)
	if err != nil {
		return nil, fmt.Errorf("access token manager: %w", err)
	}
	refreshManager, err := coreauth.NewTokenManager(cfg.Auth.RefreshSecret, cfg.App.Name)
	if err != nil {
		return nil, fmt.Errorf("refresh token manager: %w", err)
	}

	accessTTL, err := parseConfigDuration(cfg.Auth.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("parse access token ttl: %w", err)
	}
	refreshTTL, err := parseConfigDuration(cfg.Auth.RefreshExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("parse refresh token ttl: %w", err)
	}
	rememberMeTTL, err := parseConfigDuration(cfg.Auth.RememberMeRefreshExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("parse remember me refresh token ttl: %w", err)
	}
	resetTokenTTL, err := parseConfigDuration(cfg.Auth.ResetTokenExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("parse reset token ttl: %w", err)
	}

	return &AuthService{
		repo:                 repo,
		accessTokenManager:   accessManager,
		refreshTokenManager:  refreshManager,
		accessTTL:            accessTTL,
		refreshTTL:           refreshTTL,
		rememberMeTTL:        rememberMeTTL,
		resetTokenTTL:        resetTokenTTL,
		passwordMinLength:    cfg.Auth.PasswordMinLength,
		appName:              cfg.App.Name,
		frontendURL:          cfg.App.FrontendURL,
		notificationLocale:   cfg.Notification.DefaultLocale,
		notificationAttempts: cfg.Notification.MaxAttempts,
		now:                  time.Now,
	}, nil
}

func (s *AuthService) SetNotificationPublisher(publisher NotificationEventPublisher) {
	s.notificationPublisher = publisher
}

func (s *AuthService) Login(ctx context.Context, req dto.LoginRequest, metadata LoginHistoryRecord) (dto.LoginResponse, error) {
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || req.Password == "" {
		return dto.LoginResponse{}, coreerrors.New("VALIDATION_ERROR", "identifier and password are required", http.StatusUnprocessableEntity)
	}

	user, err := s.repo.FindLoginUserByIdentifier(ctx, identifier)
	if err != nil {
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			Identifier: identifier,
			Event:      model.LoginEventFailedLogin,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     "invalid_credentials",
		})
		if !errors.Is(err, ErrLoginUserNotFound) {
			return dto.LoginResponse{}, coreerrors.Wrap("AUTH_LOGIN_FAILED", "failed to login", http.StatusInternalServerError, err)
		}
		return dto.LoginResponse{}, coreerrors.New("AUTH_INVALID_CREDENTIALS", "invalid identifier or password", http.StatusUnauthorized)
	}

	if !coreauth.VerifyPassword(req.Password, user.PasswordHash) {
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			UserID:     user.ID,
			Identifier: identifier,
			Event:      model.LoginEventFailedLogin,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     "invalid_credentials",
		})
		return dto.LoginResponse{}, coreerrors.New("AUTH_INVALID_CREDENTIALS", "invalid identifier or password", http.StatusUnauthorized)
	}

	if !userCanLogin(user) {
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			UserID:     user.ID,
			Identifier: identifier,
			Event:      model.LoginEventFailedLogin,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     string(user.Status),
		})
		return dto.LoginResponse{}, inactiveUserError(user.Status)
	}

	refreshTTL := s.refreshTTL
	if req.RememberMe {
		refreshTTL = s.rememberMeTTL
	}
	now := s.now().UTC()

	refreshToken, err := coreauth.NewRandomToken(32)
	if err != nil {
		return dto.LoginResponse{}, coreerrors.Wrap("AUTH_TOKEN_CREATE_FAILED", "failed to create refresh token", http.StatusInternalServerError, err)
	}
	refreshTokenHash := coreauth.HashToken(refreshToken)
	sessionID, err := s.repo.CreateSession(ctx, NewSession{
		UserID:           user.ID,
		RefreshTokenHash: refreshTokenHash,
		DeviceName:       firstNonEmpty(req.DeviceName, metadata.DeviceName),
		UserAgent:        metadata.UserAgent,
		IPAddress:        metadata.IPAddress,
		ExpiresAt:        now.Add(refreshTTL),
	})
	if err != nil {
		return dto.LoginResponse{}, coreerrors.Wrap("AUTH_SESSION_CREATE_FAILED", "failed to create session", http.StatusInternalServerError, err)
	}

	accessToken, err := s.accessTokenManager.Generate(coreauth.Claims{
		UserID:      user.ID,
		SessionID:   sessionID,
		Email:       user.Email,
		Username:    user.Username,
		Roles:       user.Roles,
		Permissions: user.Permissions,
		TokenType:   coreauth.TokenTypeAccess,
	}, s.accessTTL)
	if err != nil {
		return dto.LoginResponse{}, coreerrors.Wrap("AUTH_TOKEN_CREATE_FAILED", "failed to create access token", http.StatusInternalServerError, err)
	}

	if err := s.repo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		return dto.LoginResponse{}, coreerrors.Wrap("AUTH_LAST_LOGIN_UPDATE_FAILED", "failed to update last login", http.StatusInternalServerError, err)
	}
	s.recordLoginAttempt(ctx, LoginHistoryRecord{
		UserID:     user.ID,
		Identifier: identifier,
		Event:      model.LoginEventLogin,
		Success:    true,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		DeviceName: metadata.DeviceName,
	})

	return dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
		User: dto.AuthUserView{
			ID:          user.ID,
			Name:        user.Name,
			Email:       user.Email,
			Username:    user.Username,
			Status:      string(user.Status),
			Roles:       user.Roles,
			Permissions: user.Permissions,
		},
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, accessToken string, metadata LoginHistoryRecord) error {
	claims, err := s.accessTokenManager.Parse(strings.TrimSpace(accessToken))
	if err != nil {
		return coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}
	if claims.TokenType != coreauth.TokenTypeAccess || claims.UserID == "" || claims.SessionID == "" {
		return coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}

	now := s.now().UTC()
	if err := s.repo.RevokeSession(ctx, claims.SessionID, now); err != nil {
		return coreerrors.Wrap("AUTH_SESSION_REVOKE_FAILED", "failed to revoke session", http.StatusInternalServerError, err)
	}

	s.recordLoginAttempt(ctx, LoginHistoryRecord{
		UserID:     claims.UserID,
		Event:      model.LoginEventLogout,
		Success:    true,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		DeviceName: metadata.DeviceName,
	})
	s.recordLoginAttempt(ctx, LoginHistoryRecord{
		UserID:     claims.UserID,
		Event:      model.LoginEventTokenRevoked,
		Success:    true,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		DeviceName: metadata.DeviceName,
	})

	return nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req dto.RefreshTokenRequest, metadata LoginHistoryRecord) (dto.TokenResponse, error) {
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		return dto.TokenResponse{}, coreerrors.New("VALIDATION_ERROR", "refresh token is required", http.StatusUnprocessableEntity)
	}

	now := s.now().UTC()
	refreshTokenHash := coreauth.HashToken(refreshToken)
	session, err := s.repo.FindRefreshSessionByTokenHash(ctx, refreshTokenHash)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return dto.TokenResponse{}, coreerrors.New("AUTH_TOKEN_REVOKED", "refresh token is invalid or revoked", http.StatusUnauthorized)
		}
		return dto.TokenResponse{}, coreerrors.Wrap("AUTH_REFRESH_FAILED", "failed to refresh token", http.StatusInternalServerError, err)
	}

	if session.TokenRevokedAt != nil || session.TokenReplacedByID != "" {
		_ = s.repo.RevokeSession(ctx, session.SessionID, now)
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			UserID:     session.UserID,
			Event:      model.LoginEventTokenRevoked,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     "refresh_token_reuse",
		})
		return dto.TokenResponse{}, coreerrors.New("AUTH_TOKEN_REVOKED", "refresh token has been reused", http.StatusUnauthorized)
	}

	if session.SessionRevokedAt != nil || !session.SessionExpiresAt.After(now) {
		return dto.TokenResponse{}, coreerrors.New("AUTH_TOKEN_REVOKED", "session is expired or revoked", http.StatusUnauthorized)
	}
	if !session.Status.CanLogin() {
		return dto.TokenResponse{}, inactiveUserError(session.Status)
	}

	newRefreshToken, err := coreauth.NewRandomToken(32)
	if err != nil {
		return dto.TokenResponse{}, coreerrors.Wrap("AUTH_TOKEN_CREATE_FAILED", "failed to create refresh token", http.StatusInternalServerError, err)
	}
	newRefreshTokenHash := coreauth.HashToken(newRefreshToken)
	newRefreshExpiresAt := now.Add(s.refreshTTL)

	if err := s.repo.RotateRefreshToken(ctx, RefreshTokenRotation{
		OldTokenID:        session.TokenID,
		SessionID:         session.SessionID,
		UserID:            session.UserID,
		NewTokenHash:      newRefreshTokenHash,
		RotatedAt:         now,
		NewTokenExpiresAt: newRefreshExpiresAt,
	}); err != nil {
		return dto.TokenResponse{}, coreerrors.Wrap("AUTH_REFRESH_FAILED", "failed to rotate refresh token", http.StatusInternalServerError, err)
	}

	accessToken, err := s.accessTokenManager.Generate(coreauth.Claims{
		UserID:      session.UserID,
		SessionID:   session.SessionID,
		Email:       session.Email,
		Username:    session.Username,
		Roles:       session.Roles,
		Permissions: session.Permissions,
		TokenType:   coreauth.TokenTypeAccess,
	}, s.accessTTL)
	if err != nil {
		return dto.TokenResponse{}, coreerrors.Wrap("AUTH_TOKEN_CREATE_FAILED", "failed to create access token", http.StatusInternalServerError, err)
	}

	s.recordLoginAttempt(ctx, LoginHistoryRecord{
		UserID:     session.UserID,
		Event:      model.LoginEventTokenRefresh,
		Success:    true,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		DeviceName: metadata.DeviceName,
	})

	return dto.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req dto.ForgotPasswordRequest, _ LoginHistoryRecord) error {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return coreerrors.New("VALIDATION_ERROR", "email is required", http.StatusUnprocessableEntity)
	}

	user, err := s.repo.FindPasswordResetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrPasswordResetUserNotFound) {
			return nil
		}
		return coreerrors.Wrap("AUTH_FORGOT_PASSWORD_FAILED", "failed to request password reset", http.StatusInternalServerError, err)
	}

	if !user.Status.CanLogin() {
		return nil
	}

	token, err := coreauth.NewRandomToken(32)
	if err != nil {
		return coreerrors.Wrap("AUTH_RESET_TOKEN_CREATE_FAILED", "failed to create reset token", http.StatusInternalServerError, err)
	}

	expiresAt := s.now().UTC().Add(s.resetTokenTTL)
	if err := s.repo.CreatePasswordResetToken(ctx, NewPasswordResetToken{
		UserID:    user.ID,
		TokenHash: coreauth.HashToken(token),
		ExpiresAt: expiresAt,
	}); err != nil {
		return coreerrors.Wrap("AUTH_RESET_TOKEN_STORE_FAILED", "failed to store reset token", http.StatusInternalServerError, err)
	}

	if s.notificationPublisher == nil {
		return nil
	}

	_, err = s.notificationPublisher.Publish(ctx, notificationpublisher.Event{
		Type:   passwordResetRequestedEvent,
		UserID: user.ID,
		Recipient: notificationdomain.NotificationRecipient{
			Type:   "user",
			UserID: user.ID,
			Name:   user.Name,
			Email:  user.Email,
		},
		Payload: map[string]any{
			"app_name":   firstNonEmpty(s.appName, "Zyad Cloud"),
			"user_id":    user.ID,
			"user_name":  user.Name,
			"user_email": user.Email,
			"email":      user.Email,
			"reset_url":  s.resetPasswordURL(token),
			"expired_at": expiresAt.Format(time.RFC3339),
		},
		Locale:      s.notificationLocale,
		MaxAttempts: s.notificationAttempts,
	})
	if err != nil {
		return coreerrors.Wrap("AUTH_RESET_NOTIFICATION_FAILED", "failed to queue reset notification", http.StatusInternalServerError, err)
	}

	return nil
}

func (s *AuthService) ValidateResetToken(ctx context.Context, req dto.ValidateResetTokenRequest) (dto.ValidateResetTokenResponse, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return dto.ValidateResetTokenResponse{}, coreerrors.New("VALIDATION_ERROR", "token is required", http.StatusUnprocessableEntity)
	}

	resetToken, err := s.repo.FindPasswordResetTokenByHash(ctx, coreauth.HashToken(token))
	if err != nil {
		if errors.Is(err, ErrPasswordResetTokenNotFound) {
			return dto.ValidateResetTokenResponse{}, resetTokenInvalidError()
		}
		return dto.ValidateResetTokenResponse{}, coreerrors.Wrap("AUTH_RESET_TOKEN_VALIDATE_FAILED", "failed to validate reset token", http.StatusInternalServerError, err)
	}

	if resetToken.UsedAt != nil || !resetToken.ExpiresAt.After(s.now().UTC()) {
		return dto.ValidateResetTokenResponse{}, resetTokenInvalidError()
	}

	return dto.ValidateResetTokenResponse{
		Valid:     true,
		ExpiresAt: resetToken.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) error {
	token := strings.TrimSpace(req.Token)
	if token == "" || req.Password == "" || req.PasswordConfirmation == "" {
		return coreerrors.New("VALIDATION_ERROR", "token, password, and password confirmation are required", http.StatusUnprocessableEntity)
	}
	if req.Password != req.PasswordConfirmation {
		return coreerrors.New("AUTH_PASSWORD_CONFIRMATION_MISMATCH", "password confirmation does not match", http.StatusUnprocessableEntity)
	}
	if len(req.Password) < s.passwordMinLength {
		return coreerrors.New(
			"AUTH_PASSWORD_POLICY_FAILED",
			fmt.Sprintf("password must be at least %d characters", s.passwordMinLength),
			http.StatusUnprocessableEntity,
		)
	}

	passwordHash, err := coreauth.HashPassword(req.Password)
	if err != nil {
		return coreerrors.Wrap("AUTH_PASSWORD_HASH_FAILED", "failed to hash password", http.StatusInternalServerError, err)
	}

	changedAt := s.now().UTC()
	user, err := s.repo.CompletePasswordReset(ctx, coreauth.HashToken(token), passwordHash, changedAt)
	if err != nil {
		if errors.Is(err, ErrPasswordResetTokenNotFound) {
			return resetTokenInvalidError()
		}
		return coreerrors.Wrap("AUTH_RESET_PASSWORD_FAILED", "failed to reset password", http.StatusInternalServerError, err)
	}

	// The password reset is already committed, so a queue failure must not
	// make the client retry with a token that has been consumed.
	s.publishPasswordChanged(ctx, user.ID, user.Name, user.Email, changedAt)

	return nil
}

func (s *AuthService) ChangePassword(
	ctx context.Context,
	userID string,
	sessionID string,
	req dto.ChangePasswordRequest,
	metadata LoginHistoryRecord,
) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" {
		return coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized)
	}
	if req.CurrentPassword == "" || req.NewPassword == "" || req.NewPasswordConfirmation == "" {
		return coreerrors.New(
			"VALIDATION_ERROR",
			"current password, new password, and new password confirmation are required",
			http.StatusUnprocessableEntity,
		)
	}
	if req.NewPassword != req.NewPasswordConfirmation {
		return coreerrors.New("AUTH_PASSWORD_CONFIRMATION_MISMATCH", "password confirmation does not match", http.StatusUnprocessableEntity)
	}
	if len(req.NewPassword) < s.passwordMinLength {
		return coreerrors.New(
			"AUTH_PASSWORD_POLICY_FAILED",
			fmt.Sprintf("password must be at least %d characters", s.passwordMinLength),
			http.StatusUnprocessableEntity,
		)
	}

	user, err := s.repo.FindPasswordChangeUser(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrPasswordChangeUserNotFound) {
			return coreerrors.New("UNAUTHORIZED", "authenticated user is not available", http.StatusUnauthorized)
		}
		return coreerrors.Wrap("AUTH_CHANGE_PASSWORD_FAILED", "failed to change password", http.StatusInternalServerError, err)
	}
	if !coreauth.VerifyPassword(req.CurrentPassword, user.PasswordHash) {
		return coreerrors.New("AUTH_CURRENT_PASSWORD_INVALID", "current password is invalid", http.StatusUnprocessableEntity)
	}

	passwordHash, err := coreauth.HashPassword(req.NewPassword)
	if err != nil {
		return coreerrors.Wrap("AUTH_PASSWORD_HASH_FAILED", "failed to hash password", http.StatusInternalServerError, err)
	}

	changedAt := s.now().UTC()
	if err := s.repo.ChangePassword(ctx, PasswordChange{
		UserID:             user.ID,
		CurrentSessionID:   sessionID,
		PasswordHash:       passwordHash,
		LogoutOtherDevices: req.LogoutOtherDevices,
		ChangedAt:          changedAt,
		IPAddress:          metadata.IPAddress,
		UserAgent:          metadata.UserAgent,
	}); err != nil {
		return coreerrors.Wrap("AUTH_CHANGE_PASSWORD_FAILED", "failed to change password", http.StatusInternalServerError, err)
	}

	s.publishPasswordChanged(ctx, user.ID, user.Name, user.Email, changedAt)
	return nil
}

func (s *AuthService) CurrentUser(ctx context.Context, accessToken string) (dto.CurrentUserResponse, error) {
	currentUser, err := s.currentUserFromAccessToken(ctx, accessToken)
	if err != nil {
		return dto.CurrentUserResponse{}, err
	}

	return currentUserResponse(currentUser), nil
}

func (s *AuthService) AuthenticateAccessToken(ctx context.Context, accessToken string) (middleware.AuthenticatedUser, error) {
	currentUser, err := s.currentUserFromAccessToken(ctx, accessToken)
	if err != nil {
		return middleware.AuthenticatedUser{}, err
	}

	return middleware.AuthenticatedUser{
		ID:          currentUser.ID,
		SessionID:   currentUser.Session.ID,
		Status:      string(currentUser.Status),
		Roles:       currentUser.Roles,
		Permissions: currentUser.Permissions,
	}, nil
}

func (s *AuthService) currentUserFromAccessToken(ctx context.Context, accessToken string) (CurrentUser, error) {
	claims, err := s.accessTokenManager.Parse(strings.TrimSpace(accessToken))
	if err != nil {
		return CurrentUser{}, coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}
	if claims.TokenType != coreauth.TokenTypeAccess || claims.UserID == "" || claims.SessionID == "" {
		return CurrentUser{}, coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}

	currentUser, err := s.repo.FindCurrentUser(ctx, claims.UserID, claims.SessionID)
	if err != nil {
		if errors.Is(err, ErrCurrentUserNotFound) {
			return CurrentUser{}, coreerrors.New("UNAUTHORIZED", "current user session is not active", http.StatusUnauthorized)
		}
		return CurrentUser{}, coreerrors.Wrap("AUTH_CURRENT_USER_FAILED", "failed to get current user", http.StatusInternalServerError, err)
	}
	if currentUser.Session.RevokedAt != nil || !currentUser.Session.ExpiresAt.After(s.now().UTC()) {
		return CurrentUser{}, coreerrors.New("UNAUTHORIZED", "current user session is not active", http.StatusUnauthorized)
	}
	if !currentUser.Status.CanLogin() {
		return CurrentUser{}, inactiveUserError(currentUser.Status)
	}

	return currentUser, nil
}

func (s *AuthService) recordLoginAttempt(ctx context.Context, history LoginHistoryRecord) {
	_ = s.repo.RecordLoginHistory(ctx, history)
}

func (s *AuthService) publishPasswordChanged(ctx context.Context, userID string, name string, email string, changedAt time.Time) {
	if s.notificationPublisher == nil {
		return
	}

	_, _ = s.notificationPublisher.Publish(ctx, notificationpublisher.Event{
		Type:   passwordChangedEvent,
		UserID: userID,
		Recipient: notificationdomain.NotificationRecipient{
			Type:   "user",
			UserID: userID,
			Name:   name,
			Email:  email,
		},
		Payload: map[string]any{
			"app_name":   firstNonEmpty(s.appName, "Zyad Cloud"),
			"user_id":    userID,
			"user_name":  name,
			"user_email": email,
			"email":      email,
			"changed_at": changedAt.Format(time.RFC3339),
		},
		Locale:      s.notificationLocale,
		MaxAttempts: s.notificationAttempts,
	})
}

func userCanLogin(user LoginUser) bool {
	return user.DeletedAt == nil && user.Status.CanLogin()
}

func inactiveUserError(status model.UserStatus) error {
	switch status {
	case model.UserStatusInactive:
		return coreerrors.New("AUTH_ACCOUNT_INACTIVE", "account is inactive", http.StatusForbidden)
	case model.UserStatusPending, model.UserStatusInvited:
		return coreerrors.New("AUTH_ACCOUNT_PENDING", "account is pending activation", http.StatusForbidden)
	case model.UserStatusSuspended:
		return coreerrors.New("AUTH_ACCOUNT_SUSPENDED", "account is suspended", http.StatusForbidden)
	case model.UserStatusBanned:
		return coreerrors.New("AUTH_ACCOUNT_BANNED", "account is banned", http.StatusForbidden)
	default:
		return coreerrors.New("AUTH_ACCOUNT_INACTIVE", "account is not active", http.StatusForbidden)
	}
}

func resetTokenInvalidError() error {
	return coreerrors.New("AUTH_RESET_TOKEN_INVALID", "reset token is invalid or expired", http.StatusUnprocessableEntity)
}

func parseConfigDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("duration is required")
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		duration, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, err
		}
		return duration * 24, nil
	}
	return time.ParseDuration(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *AuthService) resetPasswordURL(token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.frontendURL), "/")
	if base == "" {
		base = "http://localhost:3001"
	}
	return base + "/reset-password?token=" + url.QueryEscape(token)
}

func currentUserResponse(user CurrentUser) dto.CurrentUserResponse {
	return dto.CurrentUserResponse{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		Username:        user.Username,
		Phone:           user.Phone,
		Status:          string(user.Status),
		EmailVerifiedAt: timeStringPtr(user.EmailVerifiedAt),
		PhoneVerifiedAt: timeStringPtr(user.PhoneVerifiedAt),
		Profile: dto.CurrentUserProfile{
			AvatarURL:  user.Profile.AvatarURL,
			Bio:        user.Profile.Bio,
			JobTitle:   user.Profile.JobTitle,
			Department: user.Profile.Department,
			Company:    user.Profile.Company,
			Address:    user.Profile.Address,
			Timezone:   user.Profile.Timezone,
			Language:   user.Profile.Language,
		},
		Roles:               user.Roles,
		Permissions:         user.Permissions,
		CurrentOrganization: nil,
		Session: dto.CurrentSession{
			ID:         user.Session.ID,
			DeviceName: user.Session.DeviceName,
			IPAddress:  user.Session.IPAddress,
			LastUsedAt: timeStringPtr(user.Session.LastUsedAt),
			ExpiresAt:  user.Session.ExpiresAt.UTC().Format(time.RFC3339),
		},
	}
}

func timeStringPtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
