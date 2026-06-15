//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	"zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestAuthRepositoryCompletePasswordResetIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewAuthRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("reset-%d@example.test", suffix)
	username := fmt.Sprintf("reset-%d", suffix)
	oldPasswordHash, err := coreauth.HashPassword("old-secret-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}

	var userID string
	err = db.QueryRow(ctx, `
		INSERT INTO users (name, email, username, password_hash, status)
		VALUES ('Reset Test', $1, $2, $3, 'active')
		RETURNING id
	`, email, username, oldPasswordHash).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	expiresAt := time.Now().UTC().Add(time.Hour)
	var sessionID string
	err = db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, fmt.Sprintf("session-hash-%d", suffix), expiresAt).Scan(&sessionID)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, sessionID, userID, fmt.Sprintf("refresh-hash-%d", suffix), expiresAt); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}

	resetTokenHash := fmt.Sprintf("reset-hash-%d", suffix)
	if _, err := db.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, resetTokenHash, expiresAt); err != nil {
		t.Fatalf("insert reset token: %v", err)
	}

	newPasswordHash, err := coreauth.HashPassword("new-secret-password")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	usedAt := time.Now().UTC()
	user, err := repo.CompletePasswordReset(ctx, resetTokenHash, newPasswordHash, usedAt)
	if err != nil {
		t.Fatalf("complete password reset: %v", err)
	}
	if user.ID != userID || user.Email != email {
		t.Fatalf("completed user = %#v", user)
	}

	var storedPasswordHash string
	var tokenUsed bool
	var sessionRevoked bool
	var refreshRevoked bool
	err = db.QueryRow(ctx, `
		SELECT
			u.password_hash,
			prt.used_at IS NOT NULL,
			s.revoked_at IS NOT NULL,
			rt.revoked_at IS NOT NULL
		FROM users u
		JOIN password_reset_tokens prt ON prt.user_id = u.id
		JOIN sessions s ON s.user_id = u.id
		JOIN refresh_tokens rt ON rt.session_id = s.id
		WHERE u.id = $1 AND prt.token_hash = $2
	`, userID, resetTokenHash).Scan(
		&storedPasswordHash,
		&tokenUsed,
		&sessionRevoked,
		&refreshRevoked,
	)
	if err != nil {
		t.Fatalf("verify password reset: %v", err)
	}
	if !coreauth.VerifyPassword("new-secret-password", storedPasswordHash) {
		t.Fatal("new password hash was not stored")
	}
	if !tokenUsed || !sessionRevoked || !refreshRevoked {
		t.Fatalf("token/session state = used:%v session_revoked:%v refresh_revoked:%v", tokenUsed, sessionRevoked, refreshRevoked)
	}

	_, err = repo.CompletePasswordReset(ctx, resetTokenHash, newPasswordHash, usedAt.Add(time.Second))
	if !errors.Is(err, service.ErrPasswordResetTokenNotFound) {
		t.Fatalf("second reset error = %v, want ErrPasswordResetTokenNotFound", err)
	}
}

func TestAuthRepositoryChangePasswordIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewAuthRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("change-%d@example.test", suffix)
	username := fmt.Sprintf("change-%d", suffix)
	oldPasswordHash, err := coreauth.HashPassword("old-secret-password")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}

	var userID string
	err = db.QueryRow(ctx, `
		INSERT INTO users (name, email, username, password_hash, status)
		VALUES ('Change Test', $1, $2, $3, 'active')
		RETURNING id
	`, email, username, oldPasswordHash).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	expiresAt := time.Now().UTC().Add(time.Hour)
	currentSessionID := insertAuthSession(t, ctx, db, userID, fmt.Sprintf("current-%d", suffix), expiresAt)
	otherSessionID := insertAuthSession(t, ctx, db, userID, fmt.Sprintf("other-%d", suffix), expiresAt)

	newPasswordHash, err := coreauth.HashPassword("new-secret-password")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	changedAt := time.Now().UTC()
	if err := repo.ChangePassword(ctx, service.PasswordChange{
		UserID:             userID,
		CurrentSessionID:   currentSessionID,
		PasswordHash:       newPasswordHash,
		LogoutOtherDevices: true,
		ChangedAt:          changedAt,
		IPAddress:          "127.0.0.1",
		UserAgent:          "integration-test",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	var storedPasswordHash string
	var currentSessionRevoked bool
	var currentRefreshRevoked bool
	var otherSessionRevoked bool
	var otherRefreshRevoked bool
	var auditCount int
	err = db.QueryRow(ctx, `
		SELECT
			u.password_hash,
			current_s.revoked_at IS NOT NULL,
			current_rt.revoked_at IS NOT NULL,
			other_s.revoked_at IS NOT NULL,
			other_rt.revoked_at IS NOT NULL,
			(
				SELECT count(*)
				FROM audit_logs
				WHERE actor_user_id = u.id
					AND event = 'password_changed'
			)
		FROM users u
		JOIN sessions current_s ON current_s.id = $2
		JOIN refresh_tokens current_rt ON current_rt.session_id = current_s.id
		JOIN sessions other_s ON other_s.id = $3
		JOIN refresh_tokens other_rt ON other_rt.session_id = other_s.id
		WHERE u.id = $1
	`, userID, currentSessionID, otherSessionID).Scan(
		&storedPasswordHash,
		&currentSessionRevoked,
		&currentRefreshRevoked,
		&otherSessionRevoked,
		&otherRefreshRevoked,
		&auditCount,
	)
	if err != nil {
		t.Fatalf("verify password change: %v", err)
	}
	if !coreauth.VerifyPassword("new-secret-password", storedPasswordHash) {
		t.Fatal("new password hash was not stored")
	}
	if currentSessionRevoked || currentRefreshRevoked {
		t.Fatal("current session and refresh token must remain active")
	}
	if !otherSessionRevoked || !otherRefreshRevoked {
		t.Fatal("other session and refresh token must be revoked")
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d, want 1", auditCount)
	}
}

func insertAuthSession(
	t *testing.T,
	ctx context.Context,
	db *database.Pool,
	userID string,
	tokenHash string,
	expiresAt time.Time,
) string {
	t.Helper()

	var sessionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, tokenHash, expiresAt).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, sessionID, userID, "refresh-"+tokenHash, expiresAt); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}
	return sessionID
}

func TestAuthRepositoryPermissionSlugsReturnsGlobalScopeOnlyIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewAuthRepository(db)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Permission Scope Auth User', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("auth-permission-scope-%d@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	var organizationID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Auth Permission Scope', 'active', 'shared')
		RETURNING id
	`, fmt.Sprintf("auth-permission-scope-%d", suffix)).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}

	globalSlug := fmt.Sprintf("global.scope.%d", suffix)
	organizationSlug := fmt.Sprintf("organization.scope.%d", suffix)
	var globalPermissionID string
	var organizationPermissionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO permissions (permission_name, name, slug, module, action)
		VALUES ($1, $1, $1, 'global', 'scope')
		RETURNING id
	`, globalSlug).Scan(&globalPermissionID); err != nil {
		t.Fatalf("create global permission: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO permissions (permission_name, name, slug, module, action)
		VALUES ($1, $1, $1, 'organization', 'scope')
		RETURNING id
	`, organizationSlug).Scan(&organizationPermissionID); err != nil {
		t.Fatalf("create organization permission: %v", err)
	}

	var globalRoleID string
	var organizationRoleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug)
		VALUES ($1, $1)
		RETURNING id
	`, fmt.Sprintf("global-scope-%d", suffix)).Scan(&globalRoleID); err != nil {
		t.Fatalf("create global role: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug)
		VALUES ($1, $1)
		RETURNING id
	`, fmt.Sprintf("organization-scope-%d", suffix)).Scan(&organizationRoleID); err != nil {
		t.Fatalf("create organization role: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id, scope)
		VALUES ($1, $2, 'all'), ($3, $4, 'organization')
	`, globalRoleID, globalPermissionID,
		organizationRoleID, organizationPermissionID); err != nil {
		t.Fatalf("assign role permissions: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, organization_id)
		VALUES ($1, $2, NULL), ($1, $3, $4)
	`, userID, globalRoleID, organizationRoleID, organizationID); err != nil {
		t.Fatalf("assign user roles: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_roles WHERE user_id = $1`, userID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM roles WHERE id IN ($1, $2)`, globalRoleID, organizationRoleID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM permissions WHERE id IN ($1, $2)`, globalPermissionID, organizationPermissionID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organizationID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	permissions, err := repo.permissionSlugs(ctx, userID)
	if err != nil {
		t.Fatalf("permissionSlugs() error = %v", err)
	}
	if len(permissions) != 1 || permissions[0] != globalSlug {
		t.Fatalf("permissionSlugs() = %#v, want [%s]", permissions, globalSlug)
	}
}

func TestAuthRepositoryEmailVerificationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewAuthRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("verify-%d@example.test", suffix)
	username := fmt.Sprintf("verify-%d", suffix)

	var userID string
	err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, username, status)
		VALUES ('Pending User', $1, $2, 'pending')
		RETURNING id
	`, email, username).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	targetUser, err := repo.FindEmailVerificationUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("FindEmailVerificationUserByEmail: %v", err)
	}
	if targetUser.ID != userID || targetUser.Status != "pending" || targetUser.EmailVerifiedAt != nil {
		t.Fatalf("unexpected target user details: %+v", targetUser)
	}

	tokenHash := fmt.Sprintf("token-hash-%d", suffix)
	expiresAt := time.Now().UTC().Add(time.Hour)
	err = repo.CreateEmailVerificationToken(ctx, service.NewEmailVerificationToken{
		UserID:    userID,
		Email:     email,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken: %v", err)
	}

	tokenObj, err := repo.FindEmailVerificationTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("FindEmailVerificationTokenByHash: %v", err)
	}
	if tokenObj.UserID != userID || tokenObj.Email != email || tokenObj.UsedAt != nil {
		t.Fatalf("unexpected token details: %+v", tokenObj)
	}

	verifiedAt := time.Now().UTC()
	verifyUser, err := repo.CompleteEmailVerification(ctx, tokenHash, verifiedAt)
	if err != nil {
		t.Fatalf("CompleteEmailVerification: %v", err)
	}
	if verifyUser.ID != userID || verifyUser.Email != email {
		t.Fatalf("unexpected verified user: %+v", verifyUser)
	}

	var status string
	var emailVerifiedAt sql.NullTime
	var tokenUsedAt sql.NullTime
	err = db.QueryRow(ctx, `
		SELECT u.status, u.email_verified_at, evt.used_at
		FROM users u
		JOIN email_verification_tokens evt ON evt.user_id = u.id
		WHERE u.id = $1 AND evt.token_hash = $2
	`, userID, tokenHash).Scan(&status, &emailVerifiedAt, &tokenUsedAt)
	if err != nil {
		t.Fatalf("query updated details: %v", err)
	}

	if status != "active" {
		t.Fatalf("expected status 'active', got %q", status)
	}
	if !emailVerifiedAt.Valid || !tokenUsedAt.Valid {
		t.Fatalf("expected valid email_verified_at and token used_at, got %v and %v", emailVerifiedAt.Valid, tokenUsedAt.Valid)
	}
}
