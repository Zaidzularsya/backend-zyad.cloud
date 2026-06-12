//go:build integration

package repository

import (
	"context"
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
