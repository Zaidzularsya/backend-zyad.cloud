package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
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

func TestAuthServiceGoogleAuthExistingIdentitySuccess(t *testing.T) {
	repo := &fakeLoginRepository{
		googleUserBySubject: GoogleAuthUser{
			ID:          "user-1",
			Name:        "Jane Doe",
			Email:       "jane@example.com",
			Username:    "jane",
			Status:      model.UserStatusActive,
			Roles:       []string{"member"},
			Permissions: []string{"dashboard.read"},
		},
		findGoogleEmailErr: ErrGoogleEmailUserNotFound,
		sessionID:          "session-google",
	}
	svc := newTestAuthService(t, repo)
	svc.googleEnabled = true
	svc.googleVerifier = &fakeGoogleVerifier{claims: coreauth.GoogleIDTokenClaims{
		Subject:       "google-sub-1",
		Email:         "jane@example.com",
		EmailVerified: true,
		Name:          "Jane Doe",
	}}

	result, err := svc.GoogleAuth(context.Background(), dto.GoogleAuthRequest{
		IDToken:    "valid-google-token",
		RememberMe: true,
		DeviceName: "Chrome",
	}, LoginHistoryRecord{IPAddress: "127.0.0.1", UserAgent: "test-agent"})
	if err != nil {
		t.Fatalf("google auth: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("expected auth tokens")
	}
	if result.IsNewUser {
		t.Fatal("existing google identity should not be marked new")
	}
	if repo.googleSubjectLookup != "google-sub-1" {
		t.Fatalf("subject lookup = %q", repo.googleSubjectLookup)
	}
	if repo.lastLoginUserID != "user-1" {
		t.Fatalf("last login user = %q", repo.lastLoginUserID)
	}
	if len(repo.histories) == 0 || repo.histories[len(repo.histories)-1].Reason != "google" {
		t.Fatalf("expected google login history, got %#v", repo.histories)
	}
}

func TestAuthServiceGoogleAuthAutoRegistersActiveUser(t *testing.T) {
	repo := &fakeLoginRepository{
		findGoogleSubjectErr: ErrGoogleIdentityNotFound,
		findGoogleEmailErr:   ErrGoogleEmailUserNotFound,
		createdGoogleUser: GoogleAuthUser{
			ID:          "user-new",
			Name:        "New User",
			Email:       "new@example.com",
			Username:    "new",
			Status:      model.UserStatusActive,
			Roles:       []string{"member"},
			Permissions: []string{},
		},
		sessionID: "session-new",
	}
	svc := newTestAuthService(t, repo)
	svc.googleEnabled = true
	svc.googleAutoRegister = true
	svc.googleDefaultRole = "member"
	svc.googleDefaultStatus = model.UserStatusActive
	svc.googleVerifier = &fakeGoogleVerifier{claims: coreauth.GoogleIDTokenClaims{
		Subject:       "google-sub-new",
		Email:         "New@Example.com",
		EmailVerified: true,
		Name:          "New User",
		Picture:       "https://example.com/avatar.png",
	}}

	result, err := svc.GoogleAuth(context.Background(), dto.GoogleAuthRequest{IDToken: "valid-google-token"}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("google auth register: %v", err)
	}
	if !result.IsNewUser {
		t.Fatal("expected new user flag")
	}
	if repo.googleRegistration.Subject != "google-sub-new" {
		t.Fatalf("registration subject = %q", repo.googleRegistration.Subject)
	}
	if repo.googleRegistration.Email != "new@example.com" {
		t.Fatalf("registration email = %q", repo.googleRegistration.Email)
	}
	if repo.googleRegistration.Status != model.UserStatusActive {
		t.Fatalf("registration status = %q", repo.googleRegistration.Status)
	}
	if repo.googleRegistration.RoleSlug != "member" {
		t.Fatalf("registration role = %q", repo.googleRegistration.RoleSlug)
	}
	if result.User.Status != "active" {
		t.Fatalf("response status = %q", result.User.Status)
	}
}

func TestAuthServiceGoogleAuthAutoRegistersAndProvisionsWorkspace(t *testing.T) {
	repo := &fakeLoginRepository{
		findGoogleSubjectErr: ErrGoogleIdentityNotFound,
		findGoogleEmailErr:   ErrGoogleEmailUserNotFound,
		createdGoogleUser: GoogleAuthUser{
			ID:          "user-new",
			Name:        "New Workspace Owner",
			Email:       "new@example.com",
			Username:    "new",
			Status:      model.UserStatusActive,
			Roles:       []string{"member"},
			Permissions: []string{},
		},
		sessionID: "session-new",
	}
	provisioner := &fakeGoogleWorkspaceProvisioner{}
	svc := newTestAuthService(t, repo)
	svc.googleEnabled = true
	svc.googleAutoRegister = true
	svc.googleDefaultRole = "member"
	svc.googleDefaultStatus = model.UserStatusActive
	svc.googleWorkspace = provisioner
	svc.googleVerifier = &fakeGoogleVerifier{claims: coreauth.GoogleIDTokenClaims{
		Subject:       "google-sub-new",
		Email:         "new@example.com",
		EmailVerified: true,
		Name:          "New Workspace Owner",
	}}

	_, err := svc.GoogleAuth(
		context.Background(),
		dto.GoogleAuthRequest{IDToken: "valid-google-token"},
		LoginHistoryRecord{IPAddress: "127.0.0.1", UserAgent: "test-agent"},
	)
	if err != nil {
		t.Fatalf("google auth register: %v", err)
	}
	if provisioner.userID != "user-new" ||
		provisioner.sessionID != "session-new" ||
		provisioner.workspaceName != "New Workspace Owner" ||
		provisioner.metadata.IPAddress != "127.0.0.1" {
		t.Fatalf("provisioner = %#v", provisioner)
	}
}

func TestAuthServiceGoogleAuthRejectsDisabledConfig(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)
	svc.googleVerifier = &fakeGoogleVerifier{}

	_, err := svc.GoogleAuth(context.Background(), dto.GoogleAuthRequest{IDToken: "valid-google-token"}, LoginHistoryRecord{})
	if err == nil {
		t.Fatal("expected google disabled error")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_GOOGLE_DISABLED" {
		t.Fatalf("expected AUTH_GOOGLE_DISABLED, got %v", err)
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

func TestAuthServiceAuthenticateAccessTokenRejectsInactiveUser(t *testing.T) {
	repo := &fakeLoginRepository{
		currentUser: CurrentUser{
			ID:     "user-1",
			Status: model.UserStatusInactive,
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

	_, err = svc.AuthenticateAccessToken(context.Background(), accessToken)
	if err == nil {
		t.Fatal("expected inactive user error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_ACCOUNT_INACTIVE" {
		t.Fatalf("expected AUTH_ACCOUNT_INACTIVE, got %v", err)
	}
}

func TestAuthServiceForgotPasswordCreatesResetTokenAndPublishesNotification(t *testing.T) {
	repo := &fakeLoginRepository{
		passwordResetUser: PasswordResetUser{
			ID:     "user-1",
			Name:   "Jane Doe",
			Email:  "jane@example.com",
			Status: model.UserStatusActive,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ForgotPassword(context.Background(), dto.ForgotPasswordRequest{
		Email: " Jane@Example.com ",
	}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	if repo.passwordResetToken.UserID != "user-1" {
		t.Fatalf("reset token user = %q, want user-1", repo.passwordResetToken.UserID)
	}
	if repo.passwordResetToken.TokenHash == "" {
		t.Fatal("reset token hash is empty")
	}
	if repo.passwordResetToken.ExpiresAt != time.Date(2026, 6, 11, 10, 15, 0, 0, time.UTC) {
		t.Fatalf("reset token expires = %s", repo.passwordResetToken.ExpiresAt)
	}
	if publisher.event.Type != passwordResetRequestedEvent {
		t.Fatalf("published event = %q, want %q", publisher.event.Type, passwordResetRequestedEvent)
	}
	if publisher.event.Recipient.Email != "jane@example.com" {
		t.Fatalf("recipient email = %q", publisher.event.Recipient.Email)
	}
	resetURL, _ := publisher.event.Payload["reset_url"].(string)
	if resetURL == "" || !strings.HasPrefix(resetURL, "https://app.example.test/reset-password?token=") {
		t.Fatalf("reset_url = %q", resetURL)
	}
}

func TestAuthServiceForgotPasswordReturnsSuccessForUnknownEmail(t *testing.T) {
	repo := &fakeLoginRepository{findPasswordResetErr: ErrPasswordResetUserNotFound}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ForgotPassword(context.Background(), dto.ForgotPasswordRequest{Email: "missing@example.com"}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("forgot password unknown email: %v", err)
	}
	if repo.passwordResetToken.TokenHash != "" {
		t.Fatal("unexpected reset token for unknown email")
	}
	if publisher.called {
		t.Fatal("unexpected notification for unknown email")
	}
}

func TestAuthServiceForgotPasswordSkipsInactiveUser(t *testing.T) {
	repo := &fakeLoginRepository{
		passwordResetUser: PasswordResetUser{
			ID:     "user-1",
			Name:   "Jane Doe",
			Email:  "jane@example.com",
			Status: model.UserStatusSuspended,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ForgotPassword(context.Background(), dto.ForgotPasswordRequest{Email: "jane@example.com"}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("forgot password inactive user: %v", err)
	}
	if repo.passwordResetToken.TokenHash != "" {
		t.Fatal("unexpected reset token for inactive user")
	}
	if publisher.called {
		t.Fatal("unexpected notification for inactive user")
	}
}

func TestAuthServiceValidateResetTokenSuccess(t *testing.T) {
	rawToken := "valid-reset-token"
	repo := &fakeLoginRepository{
		passwordResetTokenFound: PasswordResetToken{
			ID:        "token-1",
			UserID:    "user-1",
			ExpiresAt: time.Date(2026, 6, 11, 10, 15, 0, 0, time.UTC),
		},
	}
	svc := newTestAuthService(t, repo)

	result, err := svc.ValidateResetToken(context.Background(), dto.ValidateResetTokenRequest{Token: rawToken})
	if err != nil {
		t.Fatalf("validate reset token: %v", err)
	}
	if !result.Valid {
		t.Fatal("expected token to be valid")
	}
	if result.ExpiresAt != "2026-06-11T10:15:00Z" {
		t.Fatalf("ExpiresAt = %q", result.ExpiresAt)
	}
	if repo.findPasswordResetTokenHash != coreauth.HashToken(rawToken) {
		t.Fatalf("FindPasswordResetTokenByHash hash = %q", repo.findPasswordResetTokenHash)
	}
}

func TestAuthServiceValidateResetTokenRejectsMissingToken(t *testing.T) {
	repo := &fakeLoginRepository{findPasswordResetTokenErr: ErrPasswordResetTokenNotFound}
	svc := newTestAuthService(t, repo)

	_, err := svc.ValidateResetToken(context.Background(), dto.ValidateResetTokenRequest{Token: "missing-token"})
	if err == nil {
		t.Fatal("expected reset token validation error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_RESET_TOKEN_INVALID" {
		t.Fatalf("expected AUTH_RESET_TOKEN_INVALID, got %v", err)
	}
}

func TestAuthServiceValidateResetTokenRejectsExpiredToken(t *testing.T) {
	repo := &fakeLoginRepository{
		passwordResetTokenFound: PasswordResetToken{
			ID:        "token-1",
			UserID:    "user-1",
			ExpiresAt: time.Date(2026, 6, 11, 9, 59, 0, 0, time.UTC),
		},
	}
	svc := newTestAuthService(t, repo)

	_, err := svc.ValidateResetToken(context.Background(), dto.ValidateResetTokenRequest{Token: "expired-token"})
	if err == nil {
		t.Fatal("expected expired reset token error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_RESET_TOKEN_INVALID" {
		t.Fatalf("expected AUTH_RESET_TOKEN_INVALID, got %v", err)
	}
}

func TestAuthServiceValidateResetTokenRejectsUsedToken(t *testing.T) {
	usedAt := time.Date(2026, 6, 11, 10, 5, 0, 0, time.UTC)
	repo := &fakeLoginRepository{
		passwordResetTokenFound: PasswordResetToken{
			ID:        "token-1",
			UserID:    "user-1",
			ExpiresAt: time.Date(2026, 6, 11, 10, 15, 0, 0, time.UTC),
			UsedAt:    &usedAt,
		},
	}
	svc := newTestAuthService(t, repo)

	_, err := svc.ValidateResetToken(context.Background(), dto.ValidateResetTokenRequest{Token: "used-token"})
	if err == nil {
		t.Fatal("expected used reset token error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_RESET_TOKEN_INVALID" {
		t.Fatalf("expected AUTH_RESET_TOKEN_INVALID, got %v", err)
	}
}

func TestAuthServiceResetPasswordCompletesResetAndPublishesNotification(t *testing.T) {
	repo := &fakeLoginRepository{
		completedPasswordResetUser: PasswordResetUser{
			ID:     "user-1",
			Name:   "Jane Doe",
			Email:  "jane@example.com",
			Status: model.UserStatusActive,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ResetPassword(context.Background(), dto.ResetPasswordRequest{
		Token:                "valid-reset-token",
		Password:             "new-secret-password",
		PasswordConfirmation: "new-secret-password",
	})
	if err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if repo.completedPasswordResetTokenHash != coreauth.HashToken("valid-reset-token") {
		t.Fatalf("reset token hash = %q", repo.completedPasswordResetTokenHash)
	}
	if !coreauth.VerifyPassword("new-secret-password", repo.completedPasswordHash) {
		t.Fatal("stored password hash does not match new password")
	}
	if repo.completedPasswordResetAt != time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("password reset time = %s", repo.completedPasswordResetAt)
	}
	if publisher.event.Type != passwordChangedEvent {
		t.Fatalf("published event = %q, want %q", publisher.event.Type, passwordChangedEvent)
	}
	if publisher.event.Recipient.Email != "jane@example.com" {
		t.Fatalf("recipient email = %q", publisher.event.Recipient.Email)
	}
}

func TestAuthServiceResetPasswordRejectsInvalidToken(t *testing.T) {
	repo := &fakeLoginRepository{completePasswordResetErr: ErrPasswordResetTokenNotFound}
	svc := newTestAuthService(t, repo)

	err := svc.ResetPassword(context.Background(), dto.ResetPasswordRequest{
		Token:                "invalid-token",
		Password:             "new-secret-password",
		PasswordConfirmation: "new-secret-password",
	})
	if err == nil {
		t.Fatal("expected reset password error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_RESET_TOKEN_INVALID" {
		t.Fatalf("expected AUTH_RESET_TOKEN_INVALID, got %v", err)
	}
}

func TestAuthServiceResetPasswordRejectsPasswordPolicyViolation(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.ResetPassword(context.Background(), dto.ResetPasswordRequest{
		Token:                "valid-token",
		Password:             "short",
		PasswordConfirmation: "short",
	})
	if err == nil {
		t.Fatal("expected password policy error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_PASSWORD_POLICY_FAILED" {
		t.Fatalf("expected AUTH_PASSWORD_POLICY_FAILED, got %v", err)
	}
	if repo.completedPasswordHash != "" {
		t.Fatal("repository should not be called for invalid password")
	}
}

func TestAuthServiceChangePasswordSuccess(t *testing.T) {
	oldPasswordHash, err := coreauth.HashPassword("old-secret-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo := &fakeLoginRepository{
		passwordChangeUser: PasswordChangeUser{
			ID:           "user-1",
			Name:         "Jane Doe",
			Email:        "jane@example.com",
			PasswordHash: oldPasswordHash,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err = svc.ChangePassword(
		context.Background(),
		"user-1",
		"session-current",
		dto.ChangePasswordRequest{
			CurrentPassword:         "old-secret-password",
			NewPassword:             "new-secret-password",
			NewPasswordConfirmation: "new-secret-password",
			LogoutOtherDevices:      true,
		},
		LoginHistoryRecord{IPAddress: "127.0.0.1", UserAgent: "test-agent"},
	)
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if repo.passwordChange.CurrentSessionID != "session-current" {
		t.Fatalf("current session = %q", repo.passwordChange.CurrentSessionID)
	}
	if !repo.passwordChange.LogoutOtherDevices {
		t.Fatal("expected logout other devices")
	}
	if !coreauth.VerifyPassword("new-secret-password", repo.passwordChange.PasswordHash) {
		t.Fatal("new password hash does not match")
	}
	if repo.passwordChange.IPAddress != "127.0.0.1" || repo.passwordChange.UserAgent != "test-agent" {
		t.Fatalf("audit metadata = %#v", repo.passwordChange)
	}
	if publisher.event.Type != passwordChangedEvent {
		t.Fatalf("published event = %q, want %q", publisher.event.Type, passwordChangedEvent)
	}
}

func TestAuthServiceChangePasswordRejectsInvalidCurrentPassword(t *testing.T) {
	oldPasswordHash, err := coreauth.HashPassword("old-secret-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo := &fakeLoginRepository{
		passwordChangeUser: PasswordChangeUser{
			ID:           "user-1",
			PasswordHash: oldPasswordHash,
		},
	}
	svc := newTestAuthService(t, repo)

	err = svc.ChangePassword(
		context.Background(),
		"user-1",
		"session-current",
		dto.ChangePasswordRequest{
			CurrentPassword:         "wrong-password",
			NewPassword:             "new-secret-password",
			NewPasswordConfirmation: "new-secret-password",
		},
		LoginHistoryRecord{},
	)
	if err == nil {
		t.Fatal("expected invalid current password error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_CURRENT_PASSWORD_INVALID" {
		t.Fatalf("expected AUTH_CURRENT_PASSWORD_INVALID, got %v", err)
	}
	if repo.passwordChange.PasswordHash != "" {
		t.Fatal("repository change should not be called")
	}
}

func TestAuthServiceChangePasswordRejectsPasswordPolicyViolation(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.ChangePassword(
		context.Background(),
		"user-1",
		"session-current",
		dto.ChangePasswordRequest{
			CurrentPassword:         "old-secret-password",
			NewPassword:             "short",
			NewPasswordConfirmation: "short",
		},
		LoginHistoryRecord{},
	)
	if err == nil {
		t.Fatal("expected password policy error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_PASSWORD_POLICY_FAILED" {
		t.Fatalf("expected AUTH_PASSWORD_POLICY_FAILED, got %v", err)
	}
}

func newTestAuthService(t *testing.T, repo LoginRepository) *AuthService {
	t.Helper()

	svc, err := NewAuthService(repo, config.Config{
		App: config.AppConfig{Name: "zyad.cloud", FrontendURL: "https://app.example.test"},
		Auth: config.AuthConfig{
			Secret:                     "access-secret-with-enough-entropy",
			RefreshSecret:              "refresh-secret-with-enough-entropy",
			ExpiresIn:                  "15m",
			RefreshExpiresIn:           "7d",
			RememberMeRefreshExpiresIn: "30d",
			ResetTokenExpiresIn:        "15m",
			VerificationTokenExpiresIn: "24h",
			PasswordMinLength:          8,
		},
		Notification: config.NotificationConfig{
			DefaultLocale: "id-ID",
			MaxAttempts:   3,
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
	user                            LoginUser
	findErr                         error
	googleUserBySubject             GoogleAuthUser
	googleUserByEmail               GoogleAuthUser
	createdGoogleUser               GoogleAuthUser
	findGoogleSubjectErr            error
	findGoogleEmailErr              error
	linkGoogleErr                   error
	createGoogleErr                 error
	googleSubjectLookup             string
	googleEmailLookup               string
	googleLink                      GoogleIdentityLink
	googleRegistration              GoogleUserRegistration
	passwordResetUser               PasswordResetUser
	findPasswordResetErr            error
	passwordResetTokenFound         PasswordResetToken
	findPasswordResetTokenHash      string
	findPasswordResetTokenErr       error
	passwordChangeUser              PasswordChangeUser
	findPasswordChangeErr           error
	passwordChange                  PasswordChange
	changePasswordErr               error
	refreshSession                  RefreshSession
	findRefreshErr                  error
	currentUser                     CurrentUser
	findCurrentErr                  error
	sessionID                       string
	createdSession                  NewSession
	passwordResetToken              NewPasswordResetToken
	completedPasswordResetUser      PasswordResetUser
	completedPasswordResetTokenHash string
	completedPasswordHash           string
	completedPasswordResetAt        time.Time
	completePasswordResetErr        error
	rotation                        RefreshTokenRotation
	revokedSessionID                string
	lastLoginUserID                 string
	histories                       []LoginHistoryRecord
	// new session management fields
	sessions         []CurrentSession
	listSessionsErr  error
	revokedUserID    string
	revokeSessionErr error
	excludeSessionID string
	revokeAllErr     error
	// email verification mock fields
	emailVerificationUser          EmailVerificationTargetUser
	findEmailVerificationUserErr   error
	emailVerificationToken         NewEmailVerificationToken
	createVerificationTokenErr     error
	emailVerificationTokenFound    EmailVerificationToken
	findVerificationTokenErr       error
	completedVerificationUser      EmailVerificationUser
	completeVerificationErr        error
	completedVerificationTokenHash string
	completedVerificationTime      time.Time
}

type fakeGoogleVerifier struct {
	claims coreauth.GoogleIDTokenClaims
	err    error
}

type fakeGoogleWorkspaceProvisioner struct {
	userID        string
	sessionID     string
	workspaceName string
	metadata      LoginHistoryRecord
	err           error
}

func (p *fakeGoogleWorkspaceProvisioner) ProvisionGoogleWorkspace(
	_ context.Context,
	userID string,
	sessionID string,
	workspaceName string,
	metadata LoginHistoryRecord,
) error {
	p.userID = userID
	p.sessionID = sessionID
	p.workspaceName = workspaceName
	p.metadata = metadata
	return p.err
}

func (v *fakeGoogleVerifier) Verify(context.Context, string) (coreauth.GoogleIDTokenClaims, error) {
	if v.err != nil {
		return coreauth.GoogleIDTokenClaims{}, v.err
	}
	return v.claims, nil
}

func (r *fakeLoginRepository) FindLoginUserByIdentifier(context.Context, string) (LoginUser, error) {
	if r.findErr != nil {
		return LoginUser{}, r.findErr
	}
	return r.user, nil
}

func (r *fakeLoginRepository) FindGoogleUserBySubject(_ context.Context, subject string) (GoogleAuthUser, error) {
	r.googleSubjectLookup = subject
	if r.findGoogleSubjectErr != nil {
		return GoogleAuthUser{}, r.findGoogleSubjectErr
	}
	return r.googleUserBySubject, nil
}

func (r *fakeLoginRepository) FindGoogleUserByEmail(_ context.Context, email string) (GoogleAuthUser, error) {
	r.googleEmailLookup = email
	if r.findGoogleEmailErr != nil {
		return GoogleAuthUser{}, r.findGoogleEmailErr
	}
	return r.googleUserByEmail, nil
}

func (r *fakeLoginRepository) LinkGoogleIdentity(_ context.Context, link GoogleIdentityLink) (GoogleAuthUser, error) {
	r.googleLink = link
	if r.linkGoogleErr != nil {
		return GoogleAuthUser{}, r.linkGoogleErr
	}
	user := r.googleUserByEmail
	if user.ID == "" {
		user = GoogleAuthUser{
			ID:       link.UserID,
			Email:    link.ProviderEmail,
			Status:   model.UserStatusActive,
			Username: "linked",
		}
	}
	return user, nil
}

func (r *fakeLoginRepository) CreateGoogleUser(_ context.Context, registration GoogleUserRegistration) (GoogleAuthUser, error) {
	r.googleRegistration = registration
	if r.createGoogleErr != nil {
		return GoogleAuthUser{}, r.createGoogleErr
	}
	return r.createdGoogleUser, nil
}

func (r *fakeLoginRepository) FindPasswordResetUserByEmail(context.Context, string) (PasswordResetUser, error) {
	if r.findPasswordResetErr != nil {
		return PasswordResetUser{}, r.findPasswordResetErr
	}
	return r.passwordResetUser, nil
}

func (r *fakeLoginRepository) FindPasswordResetTokenByHash(_ context.Context, tokenHash string) (PasswordResetToken, error) {
	r.findPasswordResetTokenHash = tokenHash
	if r.findPasswordResetTokenErr != nil {
		return PasswordResetToken{}, r.findPasswordResetTokenErr
	}
	return r.passwordResetTokenFound, nil
}

func (r *fakeLoginRepository) FindPasswordChangeUser(context.Context, string) (PasswordChangeUser, error) {
	if r.findPasswordChangeErr != nil {
		return PasswordChangeUser{}, r.findPasswordChangeErr
	}
	return r.passwordChangeUser, nil
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

func (r *fakeLoginRepository) CreatePasswordResetToken(_ context.Context, token NewPasswordResetToken) error {
	r.passwordResetToken = token
	return nil
}

func (r *fakeLoginRepository) CompletePasswordReset(
	_ context.Context,
	tokenHash string,
	passwordHash string,
	usedAt time.Time,
) (PasswordResetUser, error) {
	r.completedPasswordResetTokenHash = tokenHash
	r.completedPasswordHash = passwordHash
	r.completedPasswordResetAt = usedAt
	if r.completePasswordResetErr != nil {
		return PasswordResetUser{}, r.completePasswordResetErr
	}
	return r.completedPasswordResetUser, nil
}

func (r *fakeLoginRepository) ChangePassword(_ context.Context, change PasswordChange) error {
	r.passwordChange = change
	return r.changePasswordErr
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

func (r *fakeLoginRepository) ListSessions(_ context.Context, userID string) ([]CurrentSession, error) {
	if r.listSessionsErr != nil {
		return nil, r.listSessionsErr
	}
	return r.sessions, nil
}

func (r *fakeLoginRepository) RevokeUserSession(_ context.Context, sessionID string, userID string, _ time.Time) error {
	r.revokedSessionID = sessionID
	r.revokedUserID = userID
	return r.revokeSessionErr
}

func (r *fakeLoginRepository) RevokeAllSessions(_ context.Context, userID string, excludeSessionID string, _ time.Time) error {
	r.revokedUserID = userID
	r.excludeSessionID = excludeSessionID
	return r.revokeAllErr
}

func (r *fakeLoginRepository) CreateEmailVerificationToken(_ context.Context, token NewEmailVerificationToken) error {
	r.emailVerificationToken = token
	return r.createVerificationTokenErr
}

func (r *fakeLoginRepository) FindEmailVerificationTokenByHash(_ context.Context, tokenHash string) (EmailVerificationToken, error) {
	if r.findVerificationTokenErr != nil {
		return EmailVerificationToken{}, r.findVerificationTokenErr
	}
	return r.emailVerificationTokenFound, nil
}

func (r *fakeLoginRepository) CompleteEmailVerification(_ context.Context, tokenHash string, verifiedAt time.Time) (EmailVerificationUser, error) {
	r.completedVerificationTokenHash = tokenHash
	r.completedVerificationTime = verifiedAt
	if r.completeVerificationErr != nil {
		return EmailVerificationUser{}, r.completeVerificationErr
	}
	return r.completedVerificationUser, nil
}

func (r *fakeLoginRepository) FindEmailVerificationUserByEmail(_ context.Context, email string) (EmailVerificationTargetUser, error) {
	if r.findEmailVerificationUserErr != nil {
		return EmailVerificationTargetUser{}, r.findEmailVerificationUserErr
	}
	return r.emailVerificationUser, nil
}

type fakeNotificationPublisher struct {
	called bool
	event  notificationpublisher.Event
	err    error
}

func (p *fakeNotificationPublisher) Publish(_ context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	p.called = true
	p.event = event
	if p.err != nil {
		return notificationdomain.OutboxEvent{}, p.err
	}
	return notificationdomain.OutboxEvent{ID: "event-1"}, nil
}

func TestAuthServiceListSessionsSuccess(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	expectedSessions := []CurrentSession{
		{
			ID:         "session-1",
			DeviceName: "iPhone",
			IPAddress:  "192.168.1.1",
			UserAgent:  "Mozilla/5.0",
			ExpiresAt:  now.Add(24 * time.Hour),
		},
		{
			ID:         "session-2",
			DeviceName: "MacBook",
			IPAddress:  "192.168.1.2",
			UserAgent:  "Mozilla/5.0",
			ExpiresAt:  now.Add(48 * time.Hour),
		},
	}

	repo := &fakeLoginRepository{
		sessions: expectedSessions,
	}
	svc := newTestAuthService(t, repo)

	result, err := svc.ListSessions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result))
	}
	if result[0].ID != "session-1" || result[1].ID != "session-2" {
		t.Fatalf("sessions mapping incorrect: %+v", result)
	}
}

func TestAuthServiceListSessionsValidationError(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	_, err := svc.ListSessions(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected validation error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestAuthServiceListSessionsRepositoryError(t *testing.T) {
	repo := &fakeLoginRepository{
		listSessionsErr: errors.New("db error"),
	}
	svc := newTestAuthService(t, repo)

	_, err := svc.ListSessions(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected repository error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_SESSION_LIST_FAILED" {
		t.Fatalf("expected AUTH_SESSION_LIST_FAILED, got %v", err)
	}
}

func TestAuthServiceRevokeSessionSuccess(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.RevokeSession(context.Background(), "session-1", "user-1")
	if err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	if repo.revokedSessionID != "session-1" || repo.revokedUserID != "user-1" {
		t.Fatalf("expected session-1 and user-1 to be revoked, got %s and %s", repo.revokedSessionID, repo.revokedUserID)
	}
}

func TestAuthServiceRevokeSessionValidationError(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.RevokeSession(context.Background(), "", "user-1")
	if err == nil {
		t.Fatal("expected validation error for empty sessionID")
	}

	err = svc.RevokeSession(context.Background(), "session-1", "  ")
	if err == nil {
		t.Fatal("expected validation error for empty userID")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestAuthServiceRevokeSessionNotFoundError(t *testing.T) {
	repo := &fakeLoginRepository{
		revokeSessionErr: ErrSessionNotFound,
	}
	svc := newTestAuthService(t, repo)

	err := svc.RevokeSession(context.Background(), "session-1", "user-1")
	if err == nil {
		t.Fatal("expected session not found error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_SESSION_NOT_FOUND" {
		t.Fatalf("expected AUTH_SESSION_NOT_FOUND, got %v", err)
	}
}

func TestAuthServiceLogoutAllSuccess(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.LogoutAll(context.Background(), "user-1", "session-current")
	if err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}

	if repo.revokedUserID != "user-1" || repo.excludeSessionID != "session-current" {
		t.Fatalf("expected user-1 and exclude session-current, got %s and %s", repo.revokedUserID, repo.excludeSessionID)
	}
}

func TestAuthServiceLogoutAllValidationError(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.LogoutAll(context.Background(), "  ", "")
	if err == nil {
		t.Fatal("expected validation error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestAuthServiceLogoutAllRepositoryError(t *testing.T) {
	repo := &fakeLoginRepository{
		revokeAllErr: errors.New("db error"),
	}
	svc := newTestAuthService(t, repo)

	err := svc.LogoutAll(context.Background(), "user-1", "session-current")
	if err == nil {
		t.Fatal("expected repository error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_LOGOUT_ALL_FAILED" {
		t.Fatalf("expected AUTH_LOGOUT_ALL_FAILED, got %v", err)
	}
}

func TestAuthServiceVerifyEmailSuccess(t *testing.T) {
	repo := &fakeLoginRepository{
		completedVerificationUser: EmailVerificationUser{
			ID:    "user-1",
			Name:  "Verify User",
			Email: "verify@example.com",
		},
	}
	svc := newTestAuthService(t, repo)

	err := svc.VerifyEmail(context.Background(), dto.VerifyEmailRequest{Token: "valid-verification-token"})
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	expectedHash := coreauth.HashToken("valid-verification-token")
	if repo.completedVerificationTokenHash != expectedHash {
		t.Fatalf("expected token hash %s, got %s", expectedHash, repo.completedVerificationTokenHash)
	}
	if repo.completedVerificationTime.IsZero() {
		t.Fatal("expected non-zero verification time")
	}
}

func TestAuthServiceVerifyEmailValidationError(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.VerifyEmail(context.Background(), dto.VerifyEmailRequest{Token: "   "})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestAuthServiceVerifyEmailTokenInvalidOrExpired(t *testing.T) {
	repo := &fakeLoginRepository{
		completeVerificationErr: ErrEmailVerificationTokenNotFound,
	}
	svc := newTestAuthService(t, repo)

	err := svc.VerifyEmail(context.Background(), dto.VerifyEmailRequest{Token: "expired-token"})
	if err == nil {
		t.Fatal("expected token invalid/expired error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_VERIFY_TOKEN_INVALID" {
		t.Fatalf("expected AUTH_VERIFY_TOKEN_INVALID, got %v", err)
	}
}

func TestAuthServiceResendVerificationEmailSuccess(t *testing.T) {
	repo := &fakeLoginRepository{
		emailVerificationUser: EmailVerificationTargetUser{
			ID:     "user-1",
			Name:   "Pending User",
			Email:  "pending@example.com",
			Status: model.UserStatusPending,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ResendVerificationEmail(context.Background(), dto.ResendVerificationEmailRequest{
		Email: "PENDING@example.com",
	})
	if err != nil {
		t.Fatalf("ResendVerificationEmail: %v", err)
	}

	if repo.emailVerificationToken.UserID != "user-1" || repo.emailVerificationToken.Email != "pending@example.com" {
		t.Fatalf("expected token for user-1 and pending@example.com, got %+v", repo.emailVerificationToken)
	}
	if repo.emailVerificationToken.TokenHash == "" {
		t.Fatal("expected verification token hash stored")
	}
	if !publisher.called {
		t.Fatal("expected email verification notification published")
	}
	if publisher.event.Type != emailVerificationRequestedEvent {
		t.Fatalf("expected event type %s, got %s", emailVerificationRequestedEvent, publisher.event.Type)
	}
}

func TestAuthServiceResendVerificationEmailValidationError(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)

	err := svc.ResendVerificationEmail(context.Background(), dto.ResendVerificationEmailRequest{Email: "   "})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestAuthServiceResendVerificationEmailAlreadyVerified(t *testing.T) {
	now := time.Now()
	repo := &fakeLoginRepository{
		emailVerificationUser: EmailVerificationTargetUser{
			ID:              "user-1",
			Name:            "Active User",
			Email:           "active@example.com",
			Status:          model.UserStatusActive,
			EmailVerifiedAt: &now,
		},
	}
	svc := newTestAuthService(t, repo)

	err := svc.ResendVerificationEmail(context.Background(), dto.ResendVerificationEmailRequest{
		Email: "active@example.com",
	})
	if err == nil {
		t.Fatal("expected already verified error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_EMAIL_ALREADY_VERIFIED" {
		t.Fatalf("expected AUTH_EMAIL_ALREADY_VERIFIED, got %v", err)
	}
}

func TestAuthServiceResendVerificationEmailGenericSuccessForInactiveStatus(t *testing.T) {
	repo := &fakeLoginRepository{
		emailVerificationUser: EmailVerificationTargetUser{
			ID:     "user-1",
			Name:   "Banned User",
			Email:  "banned@example.com",
			Status: model.UserStatusBanned,
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ResendVerificationEmail(context.Background(), dto.ResendVerificationEmailRequest{
		Email: "banned@example.com",
	})
	if err != nil {
		t.Fatalf("expected generic success, got error: %v", err)
	}

	if repo.emailVerificationToken.TokenHash != "" {
		t.Fatal("unexpected verification token generated for banned user")
	}
	if publisher.called {
		t.Fatal("unexpected notification published for banned user")
	}
}

func TestAuthServiceResendVerificationEmailGenericSuccessForUnknownEmail(t *testing.T) {
	repo := &fakeLoginRepository{
		findEmailVerificationUserErr: ErrLoginUserNotFound,
	}
	publisher := &fakeNotificationPublisher{}
	svc := newTestAuthService(t, repo)
	svc.SetNotificationPublisher(publisher)

	err := svc.ResendVerificationEmail(context.Background(), dto.ResendVerificationEmailRequest{
		Email: "unknown@example.com",
	})
	if err != nil {
		t.Fatalf("expected generic success, got error: %v", err)
	}

	if repo.emailVerificationToken.TokenHash != "" {
		t.Fatal("unexpected verification token generated for unknown email")
	}
	if publisher.called {
		t.Fatal("unexpected notification published for unknown email")
	}
}
