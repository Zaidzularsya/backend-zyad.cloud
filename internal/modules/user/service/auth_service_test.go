package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

func TestAuthServiceLoginSuccess(t *testing.T) {
	passwordHash, err := coreauth.HashPassword("secret-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	repo := &fakeLoginRepository{
		user: LoginUser{
			ID:           "user-1",
			Name:         "Super Admin",
			Email:        "admin@example.com",
			Username:     "admin",
			PasswordHash: passwordHash,
			Status:       model.UserStatusActive,
			Roles:        []string{"super_admin"},
			Permissions:  []string{"user.read"},
		},
		sessionID: "session-1",
	}
	svc := newTestAuthService(t, repo)

	result, err := svc.Login(context.Background(), dto.LoginRequest{
		Identifier: "admin@example.com",
		Password:   "secret-password",
		RememberMe: true,
		DeviceName: "test-device",
	}, LoginHistoryRecord{IPAddress: "127.0.0.1", UserAgent: "test-agent"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}
	if result.User.ID != "user-1" {
		t.Fatalf("expected user id user-1, got %s", result.User.ID)
	}
	if repo.createdSession.RefreshTokenHash == "" {
		t.Fatal("expected refresh token hash stored in session")
	}
	if repo.lastLoginUserID != "user-1" {
		t.Fatalf("expected last login update for user-1, got %s", repo.lastLoginUserID)
	}
	if len(repo.histories) == 0 || !repo.histories[len(repo.histories)-1].Success {
		t.Fatal("expected successful login history")
	}
}

func TestAuthServiceLoginRejectsWrongPassword(t *testing.T) {
	passwordHash, err := coreauth.HashPassword("secret-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	repo := &fakeLoginRepository{
		user: LoginUser{
			ID:           "user-1",
			PasswordHash: passwordHash,
			Status:       model.UserStatusActive,
		},
	}
	svc := newTestAuthService(t, repo)

	_, err = svc.Login(context.Background(), dto.LoginRequest{
		Identifier: "admin@example.com",
		Password:   "wrong-password",
	}, LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected login error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_INVALID_CREDENTIALS" {
		t.Fatalf("expected AUTH_INVALID_CREDENTIALS, got %v", err)
	}
	if len(repo.histories) == 0 || repo.histories[0].Success {
		t.Fatal("expected failed login history")
	}
}

func TestAuthServiceLoginRejectsInactiveStatus(t *testing.T) {
	passwordHash, err := coreauth.HashPassword("secret-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	repo := &fakeLoginRepository{
		user: LoginUser{
			ID:           "user-1",
			PasswordHash: passwordHash,
			Status:       model.UserStatusSuspended,
		},
	}
	svc := newTestAuthService(t, repo)

	_, err = svc.Login(context.Background(), dto.LoginRequest{
		Identifier: "admin@example.com",
		Password:   "secret-password",
	}, LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected login error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_ACCOUNT_SUSPENDED" {
		t.Fatalf("expected AUTH_ACCOUNT_SUSPENDED, got %v", err)
	}
}

func TestAuthServiceLogoutRevokesSession(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	accessToken, err := svc.accessTokenManager.Generate(coreauth.Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		TokenType: coreauth.TokenTypeAccess,
	}, time.Minute)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	if err := svc.Logout(context.Background(), accessToken, LoginHistoryRecord{IPAddress: "127.0.0.1"}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if repo.revokedSessionID != "session-1" {
		t.Fatalf("expected revoked session session-1, got %s", repo.revokedSessionID)
	}
	if len(repo.histories) != 2 {
		t.Fatalf("expected logout and token revoked history, got %d records", len(repo.histories))
	}
	if repo.histories[0].Event != model.LoginEventLogout {
		t.Fatalf("expected logout event, got %s", repo.histories[0].Event)
	}
}

func TestAuthServiceLogoutRejectsInvalidToken(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.Logout(context.Background(), "invalid-token", LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected logout error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func TestAuthServiceRefreshTokenRotatesToken(t *testing.T) {
	oldRefreshToken := "old-refresh-token"
	repo := &fakeLoginRepository{
		refreshSession: RefreshSession{
			TokenID:          "token-1",
			SessionID:        "session-1",
			UserID:           "user-1",
			Email:            "admin@example.com",
			Username:         "admin",
			Status:           model.UserStatusActive,
			SessionExpiresAt: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
			Roles:            []string{"super_admin"},
			Permissions:      []string{"user.read"},
		},
	}
	svc := newTestAuthService(t, repo)

	result, err := svc.RefreshToken(context.Background(), dto.RefreshTokenRequest{
		RefreshToken: oldRefreshToken,
	}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}
	if result.RefreshToken == oldRefreshToken {
		t.Fatal("expected rotated refresh token")
	}
	if repo.rotation.OldTokenID != "token-1" {
		t.Fatalf("expected old token token-1, got %s", repo.rotation.OldTokenID)
	}
	if len(repo.histories) == 0 || repo.histories[0].Event != model.LoginEventTokenRefresh {
		t.Fatal("expected token refresh history")
	}
}

func TestAuthServiceRefreshTokenRevokesSessionOnReuse(t *testing.T) {
	revokedAt := time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC)
	repo := &fakeLoginRepository{
		refreshSession: RefreshSession{
			TokenID:           "token-1",
			SessionID:         "session-1",
			UserID:            "user-1",
			Status:            model.UserStatusActive,
			TokenRevokedAt:    &revokedAt,
			TokenReplacedByID: "token-2",
			SessionExpiresAt:  time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
		},
	}
	svc := newTestAuthService(t, repo)

	_, err := svc.RefreshToken(context.Background(), dto.RefreshTokenRequest{RefreshToken: "old-refresh-token"}, LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if repo.revokedSessionID != "session-1" {
		t.Fatalf("expected reused token to revoke session-1, got %s", repo.revokedSessionID)
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_TOKEN_REVOKED" {
		t.Fatalf("expected AUTH_TOKEN_REVOKED, got %v", err)
	}
}

func TestAuthServiceRefreshTokenRejectsMissingToken(t *testing.T) {
	repo := &fakeLoginRepository{findRefreshErr: ErrRefreshTokenNotFound}
	svc := newTestAuthService(t, repo)

	_, err := svc.RefreshToken(context.Background(), dto.RefreshTokenRequest{RefreshToken: "missing-token"}, LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected refresh error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_TOKEN_REVOKED" {
		t.Fatalf("expected AUTH_TOKEN_REVOKED, got %v", err)
	}
}

func TestAuthServiceCurrentUser(t *testing.T) {
	repo := &fakeLoginRepository{
		currentUser: CurrentUser{
			ID:       "user-1",
			Name:     "Super Admin",
			Email:    "admin@example.com",
			Username: "admin",
			Status:   model.UserStatusActive,
			Profile: CurrentUserProfile{
				Timezone: "Asia/Jakarta",
				Language: "id",
			},
			Roles:       []string{"super_admin"},
			Permissions: []string{"user.read"},
			Session: CurrentSession{
				ID:        "session-1",
				ExpiresAt: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := newTestAuthService(t, repo)

	accessToken, err := svc.accessTokenManager.Generate(coreauth.Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		TokenType: coreauth.TokenTypeAccess,
	}, time.Minute)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	result, err := svc.CurrentUser(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if result.ID != "user-1" {
		t.Fatalf("expected user-1, got %s", result.ID)
	}
	if result.Session.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", result.Session.ID)
	}
	if len(result.Permissions) != 1 || result.Permissions[0] != "user.read" {
		t.Fatalf("expected user.read permission, got %#v", result.Permissions)
	}
}

func TestAuthServiceCurrentUserRejectsInvalidToken(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	_, err := svc.CurrentUser(context.Background(), "invalid-token")
	if err == nil {
		t.Fatal("expected current user error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func TestAuthServiceCurrentUserRejectsRevokedSession(t *testing.T) {
	revokedAt := time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC)
	repo := &fakeLoginRepository{
		currentUser: CurrentUser{
			ID:     "user-1",
			Status: model.UserStatusActive,
			Session: CurrentSession{
				ID:        "session-1",
				ExpiresAt: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
				RevokedAt: &revokedAt,
			},
		},
	}
	svc := newTestAuthService(t, repo)

	accessToken, err := svc.accessTokenManager.Generate(coreauth.Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		TokenType: coreauth.TokenTypeAccess,
	}, time.Minute)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	_, err = svc.CurrentUser(context.Background(), accessToken)
	if err == nil {
		t.Fatal("expected current user error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func newTestAuthService(t *testing.T, repo LoginRepository) *AuthService {
	t.Helper()

	svc, err := NewAuthService(repo, config.Config{
		App: config.AppConfig{Name: "zyad.cloud"},
		Auth: config.AuthConfig{
			Secret:                     "access-secret-with-enough-entropy",
			RefreshSecret:              "refresh-secret-with-enough-entropy",
			ExpiresIn:                  "15m",
			RefreshExpiresIn:           "7d",
			RememberMeRefreshExpiresIn: "30d",
		},
	})
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	}
	return svc
}

type fakeLoginRepository struct {
	user             LoginUser
	findErr          error
	refreshSession   RefreshSession
	findRefreshErr   error
	currentUser      CurrentUser
	findCurrentErr   error
	sessionID        string
	createdSession   NewSession
	rotation         RefreshTokenRotation
	revokedSessionID string
	lastLoginUserID  string
	histories        []LoginHistoryRecord
}

func (r *fakeLoginRepository) FindLoginUserByIdentifier(context.Context, string) (LoginUser, error) {
	if r.findErr != nil {
		return LoginUser{}, r.findErr
	}
	return r.user, nil
}

func (r *fakeLoginRepository) FindRefreshSessionByTokenHash(context.Context, string) (RefreshSession, error) {
	if r.findRefreshErr != nil {
		return RefreshSession{}, r.findRefreshErr
	}
	return r.refreshSession, nil
}

func (r *fakeLoginRepository) FindCurrentUser(context.Context, string, string) (CurrentUser, error) {
	if r.findCurrentErr != nil {
		return CurrentUser{}, r.findCurrentErr
	}
	return r.currentUser, nil
}

func (r *fakeLoginRepository) CreateSession(_ context.Context, session NewSession) (string, error) {
	r.createdSession = session
	if r.sessionID == "" {
		return "session-1", nil
	}
	return r.sessionID, nil
}

func (r *fakeLoginRepository) RevokeSession(_ context.Context, sessionID string, _ time.Time) error {
	r.revokedSessionID = sessionID
	return nil
}

func (r *fakeLoginRepository) RotateRefreshToken(_ context.Context, rotation RefreshTokenRotation) error {
	r.rotation = rotation
	return nil
}

func (r *fakeLoginRepository) UpdateLastLogin(_ context.Context, userID string, _ time.Time) error {
	r.lastLoginUserID = userID
	return nil
}

func (r *fakeLoginRepository) RecordLoginHistory(_ context.Context, history LoginHistoryRecord) error {
	r.histories = append(r.histories, history)
	return nil
}
