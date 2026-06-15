//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/user/model"
	"zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestUserRepositoryListUsersIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	roleSlug := fmt.Sprintf("list_test_%d", suffix)
	var roleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, is_system)
		VALUES ($1, $1, false)
		RETURNING id
	`, roleSlug).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}

	activeID := insertListUser(t, ctx, db, fmt.Sprintf("alpha-%d@example.test", suffix), "Alpha User", model.UserStatusActive, nil)
	pendingID := insertListUser(t, ctx, db, fmt.Sprintf("beta-%d@example.test", suffix), "Beta User", model.UserStatusPending, nil)
	deletedAt := time.Now().UTC()
	deletedID := insertListUser(t, ctx, db, fmt.Sprintf("deleted-%d@example.test", suffix), "Deleted User", model.UserStatusDeleted, &deletedAt)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{activeID, pendingID, deletedID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID); err != nil {
			t.Errorf("cleanup role: %v", err)
		}
	})

	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
	`, activeID, roleID); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	users, total, err := repo.ListUsers(ctx, service.UserListFilter{
		Page:      1,
		PerPage:   10,
		Search:    "Alpha",
		Role:      roleSlug,
		Sort:      "name",
		Direction: "asc",
	})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != activeID {
		t.Fatalf("users = %#v, total = %d", users, total)
	}
	if len(users[0].Roles) != 1 || users[0].Roles[0] != roleSlug {
		t.Fatalf("roles = %#v", users[0].Roles)
	}

	users, total, err = repo.ListUsers(ctx, service.UserListFilter{
		Page:           1,
		PerPage:        10,
		IncludeDeleted: true,
		Status:         model.UserStatusDeleted,
		Search:         fmt.Sprintf("deleted-%d", suffix),
		Sort:           "created_at",
		Direction:      "desc",
	})
	if err != nil {
		t.Fatalf("ListUsers(deleted) error = %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != deletedID {
		t.Fatalf("deleted users = %#v, total = %d", users, total)
	}

	_, total, err = repo.ListUsers(ctx, service.UserListFilter{
		Page:      2,
		PerPage:   1,
		Offset:    1,
		Sort:      "name",
		Direction: "asc",
	})
	if err != nil {
		t.Fatalf("ListUsers(pagination) error = %v", err)
	}
	if total < 2 {
		t.Fatalf("total = %d, want at least 2", total)
	}
}

func TestUserRepositoryFindUserDetailIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("detail-%d@example.test", suffix)
	userID := insertListUser(t, ctx, db, email, "Detail User", model.UserStatusActive, nil)

	roleSlug := fmt.Sprintf("detail_role_%d", suffix)
	var roleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, is_system)
		VALUES ('Detail Role', $1, false)
		RETURNING id
	`, roleSlug).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}

	permissionSlug := fmt.Sprintf("detail.permission.%d", suffix)
	var permissionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO permissions (permission_name, module, action, name, slug)
		VALUES ($1, 'detail', 'read', 'Detail Permission', $1)
		RETURNING id
	`, permissionSlug).Scan(&permissionID); err != nil {
		t.Fatalf("insert permission: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE target_user_id = $1 OR actor_user_id = $1`, userID); err != nil {
			t.Errorf("cleanup audit logs: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID); err != nil {
			t.Errorf("cleanup role: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM permissions WHERE id = $1`, permissionID); err != nil {
			t.Errorf("cleanup permission: %v", err)
		}
	})

	if _, err := db.Exec(ctx, `
		INSERT INTO user_profiles (user_id, bio, job_title, timezone, language)
		VALUES ($1, 'Profile bio', 'Administrator', 'Asia/Jakarta', 'id')
	`, userID); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
	`, userID, roleID); err != nil {
		t.Fatalf("insert user role: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_permissions (user_id, permission_id, effect)
		VALUES ($1, $2, 'deny')
	`, userID, permissionID); err != nil {
		t.Fatalf("insert user permission: %v", err)
	}

	now := time.Now().UTC()
	insertDetailSession(t, ctx, db, userID, fmt.Sprintf("active-%d", suffix), now.Add(time.Hour), nil)
	revokedAt := now.Add(-time.Minute)
	insertDetailSession(t, ctx, db, userID, fmt.Sprintf("revoked-%d", suffix), now.Add(time.Hour), &revokedAt)
	insertDetailSession(t, ctx, db, userID, fmt.Sprintf("expired-%d", suffix), now.Add(-time.Hour), nil)
	if _, err := db.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, actor_user_id, target_user_id, target_type, target_id, ip_address, created_at
		)
		VALUES ('user', 'profile_updated', $1, $1, 'user', $1, '127.0.0.1', $2)
	`, userID, now); err != nil {
		t.Fatalf("insert audit log: %v", err)
	}

	user, err := repo.FindUserDetail(ctx, userID, false)
	if err != nil {
		t.Fatalf("FindUserDetail() error = %v", err)
	}
	if user.Profile.JobTitle != "Administrator" || user.Profile.Timezone != "Asia/Jakarta" {
		t.Fatalf("profile = %#v", user.Profile)
	}
	if len(user.Roles) != 1 || user.Roles[0].Slug != roleSlug {
		t.Fatalf("roles = %#v", user.Roles)
	}
	if len(user.Permissions) != 1 || user.Permissions[0].Effect != model.PermissionEffectDeny {
		t.Fatalf("permissions = %#v", user.Permissions)
	}
	if user.Sessions.Total != 3 || user.Sessions.Active != 1 || user.Sessions.Revoked != 1 || user.Sessions.Expired != 1 {
		t.Fatalf("sessions = %#v", user.Sessions)
	}
	if user.Audit.Total != 1 || user.Audit.LastEvent != "profile_updated" || user.Audit.LastIPAddress != "127.0.0.1" {
		t.Fatalf("audit = %#v", user.Audit)
	}

	if _, err := db.Exec(ctx, `
		UPDATE users
		SET status = 'deleted', deleted_at = $2
		WHERE id = $1
	`, userID, now); err != nil {
		t.Fatalf("soft delete user: %v", err)
	}
	if _, err := repo.FindUserDetail(ctx, userID, false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("FindUserDetail(deleted=false) error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := repo.FindUserDetail(ctx, userID, true); err != nil {
		t.Fatalf("FindUserDetail(deleted=true) error = %v", err)
	}
}

func TestUserRepositoryCreateUserIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	actorID := insertListUser(t, ctx, db, fmt.Sprintf("create-actor-%d@example.test", suffix), "Create Actor", model.UserStatusActive, nil)
	roleSlug := fmt.Sprintf("create_role_%d", suffix)
	var roleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, is_system)
		VALUES ('Create Role', $1, false)
		RETURNING id
	`, roleSlug).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}

	email := fmt.Sprintf("created-%d@example.test", suffix)
	username := fmt.Sprintf("created-%d", suffix)
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	userID, err := repo.CreateUser(ctx, service.NewUser{
		Name:           "Created User",
		Email:          email,
		Username:       username,
		Status:         model.UserStatusInvited,
		Profile:        service.UserProfileRecord{JobTitle: "Operator", Language: "id"},
		Roles:          []service.NewUserRole{{RoleID: roleID}},
		ActorUserID:    actorID,
		IPAddress:      "127.0.0.1",
		UserAgent:      "integration-test",
		InvitationSent: true,
		PasswordSetupToken: &service.NewPasswordResetToken{
			TokenHash: fmt.Sprintf("setup-token-%d", suffix),
			ExpiresAt: expiresAt,
		},
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userID, actorID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID); err != nil {
			t.Errorf("cleanup role: %v", err)
		}
	})

	var profileJobTitle string
	var identityProvider string
	var assignedRoleID string
	var setupTokenCount int
	var auditEvent string
	if err := db.QueryRow(ctx, `SELECT job_title FROM user_profiles WHERE user_id = $1`, userID).Scan(&profileJobTitle); err != nil {
		t.Fatalf("find profile: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT provider FROM auth_identities WHERE user_id = $1`, userID).Scan(&identityProvider); err != nil {
		t.Fatalf("find identity: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT role_id FROM user_roles WHERE user_id = $1`, userID).Scan(&assignedRoleID); err != nil {
		t.Fatalf("find role: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM password_reset_tokens WHERE user_id = $1`, userID).Scan(&setupTokenCount); err != nil {
		t.Fatalf("count setup tokens: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT event FROM audit_logs WHERE target_user_id = $1`, userID).Scan(&auditEvent); err != nil {
		t.Fatalf("find audit: %v", err)
	}
	if profileJobTitle != "Operator" || identityProvider != "local" || assignedRoleID != roleID {
		t.Fatalf("profile=%q identity=%q role=%q", profileJobTitle, identityProvider, assignedRoleID)
	}
	if setupTokenCount != 1 || auditEvent != "user_created" {
		t.Fatalf("setupTokenCount=%d auditEvent=%q", setupTokenCount, auditEvent)
	}
}

func TestUserRepositoryUpdateUserIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	actorID := insertListUser(t, ctx, db, fmt.Sprintf("update-actor-%d@example.test", suffix), "Update Actor", model.UserStatusActive, nil)
	userID := insertListUser(t, ctx, db, fmt.Sprintf("before-%d@example.test", suffix), "Before Update", model.UserStatusActive, nil)
	if _, err := db.Exec(ctx, `
		UPDATE users
		SET phone = '+628111111', email_verified_at = now(), phone_verified_at = now()
		WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("set verified identity: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_profiles (user_id, job_title)
		VALUES ($1, 'Editor')
	`, userID); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auth_identities (user_id, provider, provider_user_id, provider_email)
		VALUES ($1, 'local', $2, $3)
	`, userID, fmt.Sprintf("before-%d", suffix), fmt.Sprintf("before-%d@example.test", suffix)); err != nil {
		t.Fatalf("insert identity: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userID, actorID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	name := "After Update"
	email := fmt.Sprintf("after-%d@example.test", suffix)
	username := fmt.Sprintf("after-%d", suffix)
	phone := "+628222222"
	jobTitle := "Senior Editor"
	if err := repo.UpdateUser(ctx, service.UserUpdate{
		UserID:      userID,
		Name:        &name,
		Email:       &email,
		Username:    &username,
		Phone:       &phone,
		JobTitle:    &jobTitle,
		ActorUserID: actorID,
		IPAddress:   "127.0.0.1",
		UserAgent:   "integration-test",
	}); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	var actualName string
	var actualEmail string
	var actualUsername string
	var actualPhone string
	var emailVerifiedAt sql.NullTime
	var phoneVerifiedAt sql.NullTime
	if err := db.QueryRow(ctx, `
		SELECT name, email, username, phone, email_verified_at, phone_verified_at
		FROM users
		WHERE id = $1
	`, userID).Scan(
		&actualName, &actualEmail, &actualUsername, &actualPhone, &emailVerifiedAt, &phoneVerifiedAt,
	); err != nil {
		t.Fatalf("find updated user: %v", err)
	}
	var actualJobTitle string
	if err := db.QueryRow(ctx, `SELECT job_title FROM user_profiles WHERE user_id = $1`, userID).Scan(&actualJobTitle); err != nil {
		t.Fatalf("find updated profile: %v", err)
	}
	var providerUserID string
	var providerEmail string
	if err := db.QueryRow(ctx, `
		SELECT provider_user_id, provider_email
		FROM auth_identities
		WHERE user_id = $1 AND provider = 'local'
	`, userID).Scan(&providerUserID, &providerEmail); err != nil {
		t.Fatalf("find updated identity: %v", err)
	}
	var auditEvent string
	if err := db.QueryRow(ctx, `
		SELECT event
		FROM audit_logs
		WHERE target_user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&auditEvent); err != nil {
		t.Fatalf("find audit: %v", err)
	}

	if actualName != name || actualEmail != email || actualUsername != username || actualPhone != phone {
		t.Fatalf("identity = %q %q %q %q", actualName, actualEmail, actualUsername, actualPhone)
	}
	if emailVerifiedAt.Valid || phoneVerifiedAt.Valid {
		t.Fatalf("verification timestamps should be reset: email=%v phone=%v", emailVerifiedAt, phoneVerifiedAt)
	}
	if actualJobTitle != jobTitle || providerUserID != username || providerEmail != email {
		t.Fatalf("profile=%q providerUserID=%q providerEmail=%q", actualJobTitle, providerUserID, providerEmail)
	}
	if auditEvent != "user_updated" {
		t.Fatalf("auditEvent = %q", auditEvent)
	}
}

func TestUserRepositoryDeleteAndRestoreUserIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	actorID := insertListUser(t, ctx, db, fmt.Sprintf("lifecycle-actor-%d@example.test", suffix), "Lifecycle Actor", model.UserStatusActive, nil)
	userID := insertListUser(t, ctx, db, fmt.Sprintf("lifecycle-%d@example.test", suffix), "Lifecycle User", model.UserStatusActive, nil)
	var sessionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, now() + interval '1 hour')
		RETURNING id
	`, userID, fmt.Sprintf("lifecycle-session-%d", suffix)).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, now() + interval '1 hour')
	`, sessionID, userID, fmt.Sprintf("lifecycle-refresh-%d", suffix)); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userID, actorID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	deletedAt := time.Now().UTC()
	metadata := service.UserLifecycleMetadata{
		ActorUserID: actorID,
		IPAddress:   "127.0.0.1",
		UserAgent:   "integration-test",
	}
	if err := repo.DeleteUser(ctx, userID, metadata, deletedAt); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	var deletedStatus model.UserStatus
	var actualDeletedAt sql.NullTime
	var sessionRevoked bool
	var refreshRevoked bool
	if err := db.QueryRow(ctx, `
		SELECT
			u.status,
			u.deleted_at,
			s.revoked_at IS NOT NULL,
			rt.revoked_at IS NOT NULL
		FROM users u
		JOIN sessions s ON s.user_id = u.id
		JOIN refresh_tokens rt ON rt.session_id = s.id
		WHERE u.id = $1
	`, userID).Scan(&deletedStatus, &actualDeletedAt, &sessionRevoked, &refreshRevoked); err != nil {
		t.Fatalf("find deleted state: %v", err)
	}
	if deletedStatus != model.UserStatusDeleted || !actualDeletedAt.Valid || !sessionRevoked || !refreshRevoked {
		t.Fatalf("deleted state status=%q deletedAt=%v session=%v refresh=%v", deletedStatus, actualDeletedAt, sessionRevoked, refreshRevoked)
	}

	restoredAt := deletedAt.Add(time.Minute)
	if err := repo.RestoreUser(ctx, userID, metadata, restoredAt); err != nil {
		t.Fatalf("RestoreUser() error = %v", err)
	}

	var restoredStatus model.UserStatus
	var restoredDeletedAt sql.NullTime
	if err := db.QueryRow(ctx, `
		SELECT status, deleted_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&restoredStatus, &restoredDeletedAt); err != nil {
		t.Fatalf("find restored state: %v", err)
	}
	if restoredStatus != model.UserStatusActive || restoredDeletedAt.Valid {
		t.Fatalf("restored state status=%q deletedAt=%v", restoredStatus, restoredDeletedAt)
	}

	var events []string
	rows, err := db.Query(ctx, `
		SELECT event
		FROM audit_logs
		WHERE target_user_id = $1 AND event IN ('user_deleted', 'user_restored')
		ORDER BY created_at, id
	`, userID)
	if err != nil {
		t.Fatalf("find lifecycle audit: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var event string
		if err := rows.Scan(&event); err != nil {
			t.Fatalf("scan lifecycle audit: %v", err)
		}
		events = append(events, event)
	}
	if len(events) != 2 || events[0] != "user_deleted" || events[1] != "user_restored" {
		t.Fatalf("events = %#v", events)
	}
}

func TestUserRepositoryUpdateUserStatusIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	actorID := insertListUser(t, ctx, db, fmt.Sprintf("status-actor-%d@example.test", suffix), "Status Actor", model.UserStatusActive, nil)
	userID := insertListUser(t, ctx, db, fmt.Sprintf("status-%d@example.test", suffix), "Status User", model.UserStatusActive, nil)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userID, actorID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	if err := repo.UpdateUserStatus(ctx, service.UserStatusChange{
		UserID:      userID,
		Status:      model.UserStatusSuspended,
		Reason:      "Security review",
		ActorUserID: actorID,
		IPAddress:   "127.0.0.1",
		UserAgent:   "integration-test",
		ChangedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateUserStatus() error = %v", err)
	}

	var status model.UserStatus
	var event string
	var reason string
	var auditActorID string
	var previousStatus string
	var newStatus string
	if err := db.QueryRow(ctx, `
		SELECT
			u.status,
			a.event,
			a.metadata->>'reason',
			a.actor_user_id,
			a.metadata->>'previous_status',
			a.metadata->>'new_status'
		FROM users u
		JOIN audit_logs a ON a.target_user_id = u.id
		WHERE u.id = $1 AND a.event = 'user_status_changed'
		ORDER BY a.created_at DESC
		LIMIT 1
	`, userID).Scan(&status, &event, &reason, &auditActorID, &previousStatus, &newStatus); err != nil {
		t.Fatalf("find status audit: %v", err)
	}
	if status != model.UserStatusSuspended || event != "user_status_changed" || reason != "Security review" ||
		auditActorID != actorID || previousStatus != "active" || newStatus != "suspended" {
		t.Fatalf(
			"status=%q event=%q reason=%q actor=%q previous=%q new=%q",
			status, event, reason, auditActorID, previousStatus, newStatus,
		)
	}
}

func insertListUser(
	t *testing.T,
	ctx context.Context,
	db *database.Pool,
	email string,
	name string,
	status model.UserStatus,
	deletedAt *time.Time,
) string {
	t.Helper()

	var id string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, username, status, deleted_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, name, email, email, status, deletedAt).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestUserRepositoryListLoginHistoriesAndAuditLogsIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewUserRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	userID := insertListUser(t, ctx, db, fmt.Sprintf("audit-list-%d@example.test", suffix), "Audit List User", model.UserStatusActive, nil)

	// Insert test login histories
	if _, err := db.Exec(ctx, `
		INSERT INTO login_histories (user_id, identifier, event, success, ip_address, user_agent, device_name, reason, created_at)
		VALUES 
		($1, $2, 'login', true, '127.0.0.1', 'Mozilla/5.0', 'Desktop', '', now() - interval '2 days'),
		($1, $2, 'failed_login', false, '192.168.1.1', 'Chrome', 'Mobile', 'Invalid credentials', now() - interval '1 day')
	`, userID, fmt.Sprintf("audit-list-%d", suffix)); err != nil {
		t.Fatalf("insert login histories: %v", err)
	}

	// Insert test audit logs
	if _, err := db.Exec(ctx, `
		INSERT INTO audit_logs (module, event, actor_user_id, target_user_id, target_type, target_id, metadata, ip_address, user_agent, created_at)
		VALUES 
		('user', 'user_created', $1, $1, 'user', $1, '{"test": "create"}'::jsonb, '127.0.0.1', 'Mozilla/5.0', now() - interval '2 days'),
		('user', 'user_updated', $1, $1, 'user', $1, '{"test": "update"}'::jsonb, '192.168.1.1', 'Chrome', now() - interval '1 day')
	`, userID); err != nil {
		t.Fatalf("insert audit logs: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM login_histories WHERE user_id = $1`, userID); err != nil {
			t.Errorf("cleanup login histories: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE target_user_id = $1 OR actor_user_id = $1`, userID); err != nil {
			t.Errorf("cleanup audit logs: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	t.Run("ListLoginHistories global", func(t *testing.T) {
		histories, total, err := repo.ListLoginHistories(ctx, service.LoginHistoryFilter{
			Page:      1,
			PerPage:   10,
			Sort:      "created_at",
			Direction: "desc",
		})
		if err != nil {
			t.Fatalf("ListLoginHistories() error = %v", err)
		}
		if total < 2 || len(histories) < 2 {
			t.Fatalf("histories count = %d (want >= 2)", total)
		}
	})

	t.Run("ListLoginHistories filter user_id", func(t *testing.T) {
		histories, total, err := repo.ListLoginHistories(ctx, service.LoginHistoryFilter{
			Page:      1,
			PerPage:   10,
			UserID:    userID,
			Sort:      "created_at",
			Direction: "desc",
		})
		if err != nil {
			t.Fatalf("ListLoginHistories() error = %v", err)
		}
		if total != 2 || len(histories) != 2 {
			t.Fatalf("histories count = %d (want 2)", total)
		}
		if histories[0].Event != "failed_login" || histories[1].Event != "login" {
			t.Fatalf("unexpected order or events: %#v", histories)
		}
	})

	t.Run("ListAuditLogs global", func(t *testing.T) {
		logs, total, err := repo.ListAuditLogs(ctx, service.AuditLogFilter{
			Page:      1,
			PerPage:   10,
			Sort:      "created_at",
			Direction: "desc",
		})
		if err != nil {
			t.Fatalf("ListAuditLogs() error = %v", err)
		}
		if total < 2 || len(logs) < 2 {
			t.Fatalf("logs count = %d (want >= 2)", total)
		}
	})

	t.Run("ListAuditLogs filter target_user_id", func(t *testing.T) {
		logs, total, err := repo.ListAuditLogs(ctx, service.AuditLogFilter{
			Page:         1,
			PerPage:      10,
			TargetUserID: userID,
			Sort:         "created_at",
			Direction:    "desc",
		})
		if err != nil {
			t.Fatalf("ListAuditLogs() error = %v", err)
		}
		if total != 2 || len(logs) != 2 {
			t.Fatalf("logs count = %d (want 2)", total)
		}
		if logs[0].Event != "user_updated" || logs[1].Event != "user_created" {
			t.Fatalf("unexpected order or events: %#v", logs)
		}
	})
}

func insertDetailSession(
	t *testing.T,
	ctx context.Context,
	db *database.Pool,
	userID string,
	tokenHash string,
	expiresAt time.Time,
	revokedAt *time.Time,
) {
	t.Helper()

	if _, err := db.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, last_used_at, expires_at, revoked_at)
		VALUES ($1, $2, now(), $3, $4)
	`, userID, tokenHash, expiresAt, revokedAt); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}
