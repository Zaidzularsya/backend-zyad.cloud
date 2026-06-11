package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

var ErrLoginUserNotFound = errors.New("login user not found")
var ErrRefreshTokenNotFound = errors.New("refresh token not found")
var ErrCurrentUserNotFound = errors.New("current user not found")

type LoginRepository interface {
	FindLoginUserByIdentifier(ctx context.Context, identifier string) (LoginUser, error)
	FindRefreshSessionByTokenHash(ctx context.Context, tokenHash string) (RefreshSession, error)
	FindCurrentUser(ctx context.Context, userID string, sessionID string) (CurrentUser, error)
	CreateSession(ctx context.Context, session NewSession) (string, error)
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error
	RotateRefreshToken(ctx context.Context, rotation RefreshTokenRotation) error
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	RecordLoginHistory(ctx context.Context, history LoginHistoryRecord) error
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
	repo                LoginRepository
	accessTokenManager  *coreauth.TokenManager
	refreshTokenManager *coreauth.TokenManager
	accessTTL           time.Duration
	refreshTTL          time.Duration
	rememberMeTTL       time.Duration
	now                 func() time.Time
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

	return &AuthService{
		repo:                repo,
		accessTokenManager:  accessManager,
		refreshTokenManager: refreshManager,
		accessTTL:           accessTTL,
		refreshTTL:          refreshTTL,
		rememberMeTTL:       rememberMeTTL,
		now:                 time.Now,
	}, nil
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

func (s *AuthService) CurrentUser(ctx context.Context, accessToken string) (dto.CurrentUserResponse, error) {
	claims, err := s.accessTokenManager.Parse(strings.TrimSpace(accessToken))
	if err != nil {
		return dto.CurrentUserResponse{}, coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}
	if claims.TokenType != coreauth.TokenTypeAccess || claims.UserID == "" || claims.SessionID == "" {
		return dto.CurrentUserResponse{}, coreerrors.New("UNAUTHORIZED", "invalid access token", http.StatusUnauthorized)
	}

	currentUser, err := s.repo.FindCurrentUser(ctx, claims.UserID, claims.SessionID)
	if err != nil {
		if errors.Is(err, ErrCurrentUserNotFound) {
			return dto.CurrentUserResponse{}, coreerrors.New("UNAUTHORIZED", "current user session is not active", http.StatusUnauthorized)
		}
		return dto.CurrentUserResponse{}, coreerrors.Wrap("AUTH_CURRENT_USER_FAILED", "failed to get current user", http.StatusInternalServerError, err)
	}
	if currentUser.Session.RevokedAt != nil || !currentUser.Session.ExpiresAt.After(s.now().UTC()) {
		return dto.CurrentUserResponse{}, coreerrors.New("UNAUTHORIZED", "current user session is not active", http.StatusUnauthorized)
	}
	if currentUser.Status != model.UserStatusDeleted && !currentUser.Status.IsValid() {
		return dto.CurrentUserResponse{}, coreerrors.New("AUTH_ACCOUNT_INACTIVE", "account is not active", http.StatusForbidden)
	}

	return currentUserResponse(currentUser), nil
}

func (s *AuthService) recordLoginAttempt(ctx context.Context, history LoginHistoryRecord) {
	_ = s.repo.RecordLoginHistory(ctx, history)
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
