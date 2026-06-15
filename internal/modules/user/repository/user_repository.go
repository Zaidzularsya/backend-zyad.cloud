package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/user/model"
	"zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
)

type UserRepository struct {
	db *database.Pool
}

func NewUserRepository(db *database.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user service.NewUser) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if len(user.Roles) > 0 {
		roleIDs := make([]string, 0, len(user.Roles))
		for _, role := range user.Roles {
			roleIDs = append(roleIDs, role.RoleID)
		}
		var roleCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM roles WHERE id = ANY($1::uuid[])`, roleIDs).Scan(&roleCount); err != nil {
			return "", err
		}
		if roleCount != len(roleIDs) {
			return "", service.ErrCreateUserRoleNotFound
		}
	}

	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, username, password_hash, phone, status)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6)
		RETURNING id
	`, user.Name, user.Email, user.Username, user.PasswordHash, user.Phone, user.Status).Scan(&userID); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_profiles (
			user_id, avatar_url, bio, job_title, department, company, address, timezone, language
		)
		VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, '')
		)
	`, userID, user.Profile.AvatarURL, user.Profile.Bio, user.Profile.JobTitle, user.Profile.Department,
		user.Profile.Company, user.Profile.Address, user.Profile.Timezone, user.Profile.Language); err != nil {
		return "", err
	}

	providerUserID := user.Username
	if providerUserID == "" {
		providerUserID = user.Email
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_identities (user_id, provider, provider_user_id, provider_email)
		VALUES ($1, 'local', $2, $3)
	`, userID, providerUserID, user.Email); err != nil {
		return "", err
	}

	for _, role := range user.Roles {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id, organization_id, assigned_by)
			VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid)
		`, userID, role.RoleID, role.OrganizationID, user.ActorUserID); err != nil {
			return "", err
		}
	}

	if user.PasswordSetupToken != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			VALUES ($1, $2, $3)
		`, userID, user.PasswordSetupToken.TokenHash, user.PasswordSetupToken.ExpiresAt); err != nil {
			return "", err
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"status":          user.Status,
		"role_count":      len(user.Roles),
		"invitation_sent": user.InvitationSent,
	})
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, actor_user_id, target_user_id, target_type, target_id,
			metadata, ip_address, user_agent
		)
		VALUES (
			'user', 'user_created', NULLIF($1, '')::uuid, $2, 'user', $2,
			$3, NULLIF($4, '')::inet, NULLIF($5, '')
		)
	`, user.ActorUserID, userID, metadata, user.IPAddress, user.UserAgent); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, update service.UserUpdate) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentEmail string
	var currentUsername sql.NullString
	var currentPhone sql.NullString
	if err := tx.QueryRow(ctx, `
		SELECT email, username, phone
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, update.UserID).Scan(&currentEmail, &currentUsername, &currentPhone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ErrUpdateUserNotFound
		}
		return err
	}

	email := currentEmail
	if update.Email != nil {
		email = *update.Email
	}
	username := currentUsername.String
	if update.Username != nil {
		username = *update.Username
	}
	phone := currentPhone.String
	if update.Phone != nil {
		phone = *update.Phone
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET
			name = CASE WHEN $2 THEN $3 ELSE name END,
			email = CASE WHEN $4 THEN $5 ELSE email END,
			username = CASE WHEN $6 THEN NULLIF($7, '') ELSE username END,
			phone = CASE WHEN $8 THEN NULLIF($9, '') ELSE phone END,
			email_verified_at = CASE
				WHEN $4 AND lower(email) IS DISTINCT FROM lower($5) THEN NULL
				ELSE email_verified_at
			END,
			phone_verified_at = CASE
				WHEN $8 AND phone IS DISTINCT FROM NULLIF($9, '') THEN NULL
				ELSE phone_verified_at
			END,
			updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, update.UserID,
		update.Name != nil, stringValue(update.Name),
		update.Email != nil, email,
		update.Username != nil, username,
		update.Phone != nil, phone,
	); err != nil {
		return err
	}

	if update.Email != nil || update.Username != nil {
		providerUserID := username
		if providerUserID == "" {
			providerUserID = email
		}
		if _, err := tx.Exec(ctx, `
			UPDATE auth_identities
			SET provider_user_id = $2, provider_email = $3, updated_at = now()
			WHERE user_id = $1 AND provider = 'local'
		`, update.UserID, providerUserID, email); err != nil {
			return err
		}
	}

	if hasProfileUpdate(update) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_profiles (user_id)
			VALUES ($1)
			ON CONFLICT (user_id) DO NOTHING
		`, update.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE user_profiles
			SET
				avatar_url = CASE WHEN $2 THEN NULLIF($3, '') ELSE avatar_url END,
				bio = CASE WHEN $4 THEN NULLIF($5, '') ELSE bio END,
				job_title = CASE WHEN $6 THEN NULLIF($7, '') ELSE job_title END,
				department = CASE WHEN $8 THEN NULLIF($9, '') ELSE department END,
				company = CASE WHEN $10 THEN NULLIF($11, '') ELSE company END,
				address = CASE WHEN $12 THEN NULLIF($13, '') ELSE address END,
				timezone = CASE WHEN $14 THEN NULLIF($15, '') ELSE timezone END,
				language = CASE WHEN $16 THEN NULLIF($17, '') ELSE language END,
				updated_at = now()
			WHERE user_id = $1
		`, update.UserID,
			update.AvatarURL != nil, stringValue(update.AvatarURL),
			update.Bio != nil, stringValue(update.Bio),
			update.JobTitle != nil, stringValue(update.JobTitle),
			update.Department != nil, stringValue(update.Department),
			update.Company != nil, stringValue(update.Company),
			update.Address != nil, stringValue(update.Address),
			update.Timezone != nil, stringValue(update.Timezone),
			update.Language != nil, stringValue(update.Language),
		); err != nil {
			return err
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"changed_fields": updateUserChangedFields(update),
		"email_changed":  update.Email != nil && !strings.EqualFold(currentEmail, email),
		"phone_changed":  update.Phone != nil && currentPhone.String != phone,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, actor_user_id, target_user_id, target_type, target_id,
			metadata, ip_address, user_agent
		)
		VALUES (
			'user', 'user_updated', NULLIF($1, '')::uuid, $2, 'user', $2,
			$3, NULLIF($4, '')::inet, NULLIF($5, '')
		)
	`, update.ActorUserID, update.UserID, metadata, update.IPAddress, update.UserAgent); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *UserRepository) DeleteUser(ctx context.Context, userID string, metadata service.UserLifecycleMetadata, deletedAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var previousStatus model.UserStatus
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&previousStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ErrDeleteUserNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET status = 'deleted', deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`, userID, deletedAt); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, userID, deletedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, userID, deletedAt); err != nil {
		return err
	}

	auditMetadata, err := json.Marshal(map[string]any{
		"previous_status":  previousStatus,
		"new_status":       model.UserStatusDeleted,
		"sessions_revoked": true,
	})
	if err != nil {
		return err
	}
	if err := insertUserLifecycleAudit(ctx, tx, "user_deleted", userID, metadata, auditMetadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepository) RestoreUser(ctx context.Context, userID string, metadata service.UserLifecycleMetadata, restoredAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentStatus model.UserStatus
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM users
		WHERE id = $1 AND deleted_at IS NOT NULL
		FOR UPDATE
	`, userID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ErrRestoreUserNotFound
		}
		return err
	}

	restoreStatus := model.UserStatusInactive
	var previousStatus string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(metadata->>'previous_status', '')
		FROM audit_logs
		WHERE target_user_id = $1 AND event = 'user_deleted'
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userID).Scan(&previousStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	candidate := model.UserStatus(previousStatus)
	if candidate.IsValid() && candidate != model.UserStatusDeleted {
		restoreStatus = candidate
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET status = $2, deleted_at = NULL, updated_at = $3
		WHERE id = $1 AND deleted_at IS NOT NULL
	`, userID, restoreStatus, restoredAt); err != nil {
		return err
	}

	auditMetadata, err := json.Marshal(map[string]any{
		"previous_status": currentStatus,
		"restored_status": restoreStatus,
	})
	if err != nil {
		return err
	}
	if err := insertUserLifecycleAudit(ctx, tx, "user_restored", userID, metadata, auditMetadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepository) UpdateUserStatus(ctx context.Context, change service.UserStatusChange) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var previousStatus model.UserStatus
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, change.UserID).Scan(&previousStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ErrUpdateUserNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET status = $2, updated_at = $3
		WHERE id = $1 AND deleted_at IS NULL
	`, change.UserID, change.Status, change.ChangedAt); err != nil {
		return err
	}

	auditMetadata, err := json.Marshal(map[string]any{
		"previous_status": previousStatus,
		"new_status":      change.Status,
		"reason":          change.Reason,
	})
	if err != nil {
		return err
	}
	if err := insertUserLifecycleAudit(ctx, tx, "user_status_changed", change.UserID, service.UserLifecycleMetadata{
		ActorUserID: change.ActorUserID,
		IPAddress:   change.IPAddress,
		UserAgent:   change.UserAgent,
	}, auditMetadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type lifecycleAuditExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertUserLifecycleAudit(ctx context.Context, tx lifecycleAuditExecer, event string, userID string, metadata service.UserLifecycleMetadata, auditMetadata []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, actor_user_id, target_user_id, target_type, target_id,
			metadata, ip_address, user_agent
		)
		VALUES (
			'user', $1, NULLIF($2, '')::uuid, $3, 'user', $3,
			$4, NULLIF($5, '')::inet, NULLIF($6, '')
		)
	`, event, metadata.ActorUserID, userID, auditMetadata, metadata.IPAddress, metadata.UserAgent)
	return err
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func hasProfileUpdate(update service.UserUpdate) bool {
	return update.AvatarURL != nil || update.Bio != nil || update.JobTitle != nil ||
		update.Department != nil || update.Company != nil || update.Address != nil ||
		update.Timezone != nil || update.Language != nil
}

func updateUserChangedFields(update service.UserUpdate) []string {
	fields := make([]string, 0, 12)
	values := []struct {
		name  string
		value *string
	}{
		{"name", update.Name},
		{"email", update.Email},
		{"username", update.Username},
		{"phone", update.Phone},
		{"avatar_url", update.AvatarURL},
		{"bio", update.Bio},
		{"job_title", update.JobTitle},
		{"department", update.Department},
		{"company", update.Company},
		{"address", update.Address},
		{"timezone", update.Timezone},
		{"language", update.Language},
	}
	for _, field := range values {
		if field.value != nil {
			fields = append(fields, field.name)
		}
	}
	return fields
}

func (r *UserRepository) ListUsers(ctx context.Context, filter service.UserListFilter) ([]service.UserListRecord, int64, error) {
	where, args := userListWhere(filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM users u"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn := map[string]string{
		"name":       "u.name",
		"email":      "u.email",
		"status":     "u.status",
		"created_at": "u.created_at",
		"updated_at": "u.updated_at",
	}[filter.Sort]
	if sortColumn == "" {
		sortColumn = "u.created_at"
	}
	direction := "DESC"
	if filter.Direction == "asc" {
		direction = "ASC"
	}

	args = append(args, filter.PerPage, filter.Offset)
	query := `
		SELECT
			u.id,
			u.name,
			u.email,
			COALESCE(u.username, ''),
			COALESCE(u.phone, ''),
			u.status,
			u.email_verified_at,
			u.phone_verified_at,
			u.last_login_at,
			u.created_at,
			u.updated_at,
			u.deleted_at,
			COALESCE((
				SELECT array_agg(DISTINCT COALESCE(NULLIF(r.slug, ''), r.role_name) ORDER BY COALESCE(NULLIF(r.slug, ''), r.role_name))
				FROM user_roles ur
				JOIN roles r ON r.id = ur.role_id
				WHERE ur.user_id = u.id
			), ARRAY[]::varchar[])
		FROM users u` + where + `
		ORDER BY ` + sortColumn + ` ` + direction + `, u.id ASC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]service.UserListRecord, 0)
	for rows.Next() {
		var user service.UserListRecord
		if err := rows.Scan(
			&user.ID,
			&user.Name,
			&user.Email,
			&user.Username,
			&user.Phone,
			&user.Status,
			&user.EmailVerifiedAt,
			&user.PhoneVerifiedAt,
			&user.LastLoginAt,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.DeletedAt,
			&user.Roles,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *UserRepository) FindUserDetail(ctx context.Context, userID string, includeDeleted bool) (service.UserDetailRecord, error) {
	var user service.UserDetailRecord
	var emailVerifiedAt sql.NullTime
	var phoneVerifiedAt sql.NullTime
	var lastLoginAt sql.NullTime
	var deletedAt sql.NullTime

	query := `
		SELECT
			u.id,
			u.name,
			u.email,
			COALESCE(u.username, ''),
			COALESCE(u.phone, ''),
			u.status,
			u.email_verified_at,
			u.phone_verified_at,
			u.last_login_at,
			u.created_at,
			u.updated_at,
			u.deleted_at,
			COALESCE(p.avatar_url, ''),
			COALESCE(p.bio, ''),
			COALESCE(p.job_title, ''),
			COALESCE(p.department, ''),
			COALESCE(p.company, ''),
			COALESCE(p.address, ''),
			COALESCE(p.timezone, ''),
			COALESCE(p.language, '')
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = $1`
	if !includeDeleted {
		query += " AND u.deleted_at IS NULL"
	}
	query += " LIMIT 1"

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Phone,
		&user.Status,
		&emailVerifiedAt,
		&phoneVerifiedAt,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
		&user.Profile.AvatarURL,
		&user.Profile.Bio,
		&user.Profile.JobTitle,
		&user.Profile.Department,
		&user.Profile.Company,
		&user.Profile.Address,
		&user.Profile.Timezone,
		&user.Profile.Language,
	)
	if err != nil {
		return service.UserDetailRecord{}, err
	}
	user.EmailVerifiedAt = nullTimePtr(emailVerifiedAt)
	user.PhoneVerifiedAt = nullTimePtr(phoneVerifiedAt)
	user.LastLoginAt = nullTimePtr(lastLoginAt)
	user.DeletedAt = nullTimePtr(deletedAt)

	roles, err := r.userRoles(ctx, userID)
	if err != nil {
		return service.UserDetailRecord{}, err
	}
	permissions, err := r.userDirectPermissions(ctx, userID)
	if err != nil {
		return service.UserDetailRecord{}, err
	}
	sessions, err := r.userSessionSummary(ctx, userID)
	if err != nil {
		return service.UserDetailRecord{}, err
	}
	audit, err := r.userAuditSummary(ctx, userID)
	if err != nil {
		return service.UserDetailRecord{}, err
	}

	user.Roles = roles
	user.Permissions = permissions
	user.Sessions = sessions
	user.Audit = audit
	return user, nil
}

func (r *UserRepository) userRoles(ctx context.Context, userID string) ([]service.UserRoleRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			ur.id,
			r.role_name,
			COALESCE(NULLIF(r.slug, ''), r.role_name),
			COALESCE(ur.organization_id::text, ''),
			ur.assigned_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY 3, ur.assigned_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]service.UserRoleRecord, 0)
	for rows.Next() {
		var role service.UserRoleRecord
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &role.OrganizationID, &role.AssignedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *UserRepository) userDirectPermissions(ctx context.Context, userID string) ([]service.UserPermissionRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			up.id,
			p.id,
			COALESCE(NULLIF(p.slug, ''), p.permission_name),
			up.effect,
			COALESCE(up.organization_id::text, ''),
			up.assigned_at
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.user_id = $1
		ORDER BY 3, up.effect, up.assigned_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]service.UserPermissionRecord, 0)
	for rows.Next() {
		var permission service.UserPermissionRecord
		if err := rows.Scan(
			&permission.ID,
			&permission.PermissionID,
			&permission.Slug,
			&permission.Effect,
			&permission.OrganizationID,
			&permission.AssignedAt,
		); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (r *UserRepository) userSessionSummary(ctx context.Context, userID string) (service.UserSessionSummaryRecord, error) {
	var summary service.UserSessionSummaryRecord
	var lastActiveAt sql.NullTime
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE revoked_at IS NULL AND expires_at > (now() AT TIME ZONE 'UTC')),
			count(*) FILTER (WHERE revoked_at IS NOT NULL),
			count(*) FILTER (WHERE revoked_at IS NULL AND expires_at <= (now() AT TIME ZONE 'UTC')),
			max(COALESCE(last_used_at, created_at))
		FROM sessions
		WHERE user_id = $1
	`, userID).Scan(
		&summary.Total,
		&summary.Active,
		&summary.Revoked,
		&summary.Expired,
		&lastActiveAt,
	)
	if err != nil {
		return service.UserSessionSummaryRecord{}, err
	}
	summary.LastActiveAt = nullTimePtr(lastActiveAt)
	return summary, nil
}

func (r *UserRepository) userAuditSummary(ctx context.Context, userID string) (service.UserAuditSummaryRecord, error) {
	var summary service.UserAuditSummaryRecord
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE target_user_id = $1
	`, userID).Scan(&summary.Total); err != nil {
		return service.UserAuditSummaryRecord{}, err
	}
	if summary.Total == 0 {
		return summary, nil
	}

	var actorID sql.NullString
	var ipAddress sql.NullString
	var createdAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT event, actor_user_id::text, host(ip_address), created_at
		FROM audit_logs
		WHERE target_user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userID).Scan(&summary.LastEvent, &actorID, &ipAddress, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return summary, nil
		}
		return service.UserAuditSummaryRecord{}, err
	}
	summary.LastEventAt = &createdAt
	if actorID.Valid {
		summary.LastActorID = actorID.String
	}
	if ipAddress.Valid {
		summary.LastIPAddress = ipAddress.String
	}
	return summary, nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func userListWhere(filter service.UserListFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if !filter.IncludeDeleted {
		query.WriteString(" AND u.deleted_at IS NULL")
	}
	if filter.Search != "" {
		arg := addArg("%" + filter.Search + "%")
		query.WriteString(" AND (u.name ILIKE " + arg + " OR u.email ILIKE " + arg + " OR u.username ILIKE " + arg + ")")
	}
	if filter.Status != "" {
		query.WriteString(" AND u.status = " + addArg(string(filter.Status)))
	}
	if filter.Role != "" {
		arg := addArg(filter.Role)
		query.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM user_roles ur
				JOIN roles r ON r.id = ur.role_id
				WHERE ur.user_id = u.id
					AND COALESCE(NULLIF(r.slug, ''), r.role_name) = ` + arg + `
			)`)
	}
	if filter.OrganizationID != "" {
		arg := addArg(filter.OrganizationID)
		query.WriteString(`
			AND EXISTS (
				SELECT 1
				FROM user_roles ur
				WHERE ur.user_id = u.id
					AND ur.organization_id = ` + arg + `::uuid
			)`)
	}
	if filter.CreatedFrom != nil {
		query.WriteString(" AND u.created_at >= " + addArg(*filter.CreatedFrom))
	}
	if filter.CreatedTo != nil {
		query.WriteString(" AND u.created_at < " + addArg(filter.CreatedTo.Add(24*time.Hour)))
	}

	return query.String(), args
}

func (r *UserRepository) ListLoginHistories(ctx context.Context, filter service.LoginHistoryFilter) ([]model.LoginHistory, int64, error) {
	where, args := loginHistoryWhere(filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM login_histories lh"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn := map[string]string{
		"created_at": "lh.created_at",
	}[filter.Sort]
	if sortColumn == "" {
		sortColumn = "lh.created_at"
	}
	direction := "DESC"
	if filter.Direction == "asc" {
		direction = "ASC"
	}

	args = append(args, filter.PerPage, filter.Offset)
	query := `
		SELECT
			lh.id,
			COALESCE(lh.user_id::text, ''),
			COALESCE(lh.identifier, ''),
			lh.event,
			lh.success,
			COALESCE(host(lh.ip_address), ''),
			COALESCE(lh.user_agent, ''),
			COALESCE(lh.device_name, ''),
			COALESCE(lh.reason, ''),
			lh.created_at
		FROM login_histories lh` + where + `
		ORDER BY ` + sortColumn + ` ` + direction + `, lh.id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	histories := make([]model.LoginHistory, 0)
	for rows.Next() {
		var h model.LoginHistory
		var userIDStr string
		var ipStr string
		if err := rows.Scan(
			&h.ID,
			&userIDStr,
			&h.Identifier,
			&h.Event,
			&h.Success,
			&ipStr,
			&h.UserAgent,
			&h.DeviceName,
			&h.Reason,
			&h.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		h.UserID = userIDStr
		h.IPAddress = ipStr
		histories = append(histories, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return histories, total, nil
}

func loginHistoryWhere(filter service.LoginHistoryFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.UserID != "" {
		query.WriteString(" AND lh.user_id = " + addArg(filter.UserID) + "::uuid")
	}
	if filter.Event != "" {
		query.WriteString(" AND lh.event = " + addArg(filter.Event))
	}
	if filter.Success != nil {
		query.WriteString(" AND lh.success = " + addArg(*filter.Success))
	}
	if filter.IPAddress != "" {
		query.WriteString(" AND host(lh.ip_address) = " + addArg(filter.IPAddress))
	}
	if filter.Search != "" {
		arg := addArg("%" + filter.Search + "%")
		query.WriteString(" AND (lh.identifier ILIKE " + arg + " OR lh.reason ILIKE " + arg + " OR lh.device_name ILIKE " + arg + ")")
	}
	if filter.CreatedFrom != nil {
		query.WriteString(" AND lh.created_at >= " + addArg(*filter.CreatedFrom))
	}
	if filter.CreatedTo != nil {
		query.WriteString(" AND lh.created_at < " + addArg(filter.CreatedTo.Add(24*time.Hour)))
	}

	return query.String(), args
}

func (r *UserRepository) ListAuditLogs(ctx context.Context, filter service.AuditLogFilter) ([]model.AuditLog, int64, error) {
	where, args := auditLogWhere(filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs al"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn := map[string]string{
		"created_at": "al.created_at",
	}[filter.Sort]
	if sortColumn == "" {
		sortColumn = "al.created_at"
	}
	direction := "DESC"
	if filter.Direction == "asc" {
		direction = "ASC"
	}

	args = append(args, filter.PerPage, filter.Offset)
	query := `
		SELECT
			al.id,
			al.module,
			al.event,
			COALESCE(al.organization_id::text, ''),
			COALESCE(al.membership_id::text, ''),
			COALESCE(al.session_id::text, ''),
			COALESCE(al.actor_user_id::text, ''),
			COALESCE(al.operator_user_id::text, ''),
			COALESCE(al.effective_user_id::text, ''),
			COALESCE(al.impersonation_session_id::text, ''),
			COALESCE(al.resolution_source, ''),
			COALESCE(al.request_id, ''),
			COALESCE(al.target_user_id::text, ''),
			COALESCE(al.target_type, ''),
			COALESCE(al.target_id::text, ''),
			al.metadata,
			COALESCE(host(al.ip_address), ''),
			COALESCE(al.user_agent, ''),
			al.created_at
		FROM audit_logs al` + where + `
		ORDER BY ` + sortColumn + ` ` + direction + `, al.id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]model.AuditLog, 0)
	for rows.Next() {
		var l model.AuditLog
		var metadataBytes []byte
		if err := rows.Scan(
			&l.ID,
			&l.Module,
			&l.Event,
			&l.OrganizationID,
			&l.MembershipID,
			&l.SessionID,
			&l.ActorUserID,
			&l.OperatorUserID,
			&l.EffectiveUserID,
			&l.ImpersonationSessionID,
			&l.ResolutionSource,
			&l.RequestID,
			&l.TargetUserID,
			&l.TargetType,
			&l.TargetID,
			&metadataBytes,
			&l.IPAddress,
			&l.UserAgent,
			&l.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if len(metadataBytes) > 0 {
			_ = json.Unmarshal(metadataBytes, &l.Metadata)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func auditLogWhere(filter service.AuditLogFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.Module != "" {
		query.WriteString(" AND al.module = " + addArg(filter.Module))
	}
	if filter.OrganizationID != "" {
		query.WriteString(" AND al.organization_id = " + addArg(filter.OrganizationID) + "::uuid")
	}
	if filter.Event != "" {
		query.WriteString(" AND al.event = " + addArg(filter.Event))
	}
	if filter.ActorUserID != "" {
		query.WriteString(" AND al.actor_user_id = " + addArg(filter.ActorUserID) + "::uuid")
	}
	if filter.TargetUserID != "" {
		query.WriteString(" AND al.target_user_id = " + addArg(filter.TargetUserID) + "::uuid")
	}
	if filter.Search != "" {
		arg := addArg("%" + filter.Search + "%")
		query.WriteString(" AND (al.event ILIKE " + arg + " OR al.target_type ILIKE " + arg + ")")
	}
	if filter.CreatedFrom != nil {
		query.WriteString(" AND al.created_at >= " + addArg(*filter.CreatedFrom))
	}
	if filter.CreatedTo != nil {
		query.WriteString(" AND al.created_at < " + addArg(filter.CreatedTo.Add(24*time.Hour)))
	}

	return query.String(), args
}
