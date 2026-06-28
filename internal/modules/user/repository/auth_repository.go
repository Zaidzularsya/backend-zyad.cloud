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

	"zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
)

type AuthRepository struct {
	db *database.Pool
}

func NewAuthRepository(db *database.Pool) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) FindLoginUserByIdentifier(ctx context.Context, identifier string) (service.LoginUser, error) {
	identifier = strings.TrimSpace(identifier)

	var user service.LoginUser
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(username, ''), COALESCE(password_hash, ''), status, deleted_at
		FROM users
		WHERE deleted_at IS NULL
			AND (lower(email) = lower($1) OR lower(username) = lower($1))
		LIMIT 1
	`, identifier).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.PasswordHash,
		&user.Status,
		&user.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.LoginUser{}, service.ErrLoginUserNotFound
		}
		return service.LoginUser{}, err
	}

	roles, err := r.roleSlugs(ctx, user.ID)
	if err != nil {
		return service.LoginUser{}, err
	}
	permissions, err := r.permissionSlugs(ctx, user.ID)
	if err != nil {
		return service.LoginUser{}, err
	}
	user.Roles = roles
	user.Permissions = permissions

	return user, nil
}

func (r *AuthRepository) FindGoogleUserBySubject(ctx context.Context, subject string) (service.GoogleAuthUser, error) {
	var user service.GoogleAuthUser
	var deletedAt sql.NullTime
	var emailVerifiedAt sql.NullTime

	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, COALESCE(u.username, ''), u.status, u.deleted_at, u.email_verified_at
		FROM auth_identities ai
		JOIN users u ON u.id = ai.user_id
		WHERE ai.provider = 'google'
			AND ai.provider_user_id = $1
		LIMIT 1
	`, strings.TrimSpace(subject)).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Status,
		&deletedAt,
		&emailVerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GoogleAuthUser{}, service.ErrGoogleIdentityNotFound
		}
		return service.GoogleAuthUser{}, err
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}
	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	}
	if err := r.fillGoogleUserAccess(ctx, &user); err != nil {
		return service.GoogleAuthUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) FindGoogleUserByEmail(ctx context.Context, email string) (service.GoogleAuthUser, error) {
	var user service.GoogleAuthUser
	var deletedAt sql.NullTime
	var emailVerifiedAt sql.NullTime

	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(username, ''), status, deleted_at, email_verified_at
		FROM users
		WHERE deleted_at IS NULL
			AND lower(email) = lower($1)
		LIMIT 1
	`, strings.TrimSpace(email)).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Status,
		&deletedAt,
		&emailVerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GoogleAuthUser{}, service.ErrGoogleEmailUserNotFound
		}
		return service.GoogleAuthUser{}, err
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}
	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	}
	if err := r.fillGoogleUserAccess(ctx, &user); err != nil {
		return service.GoogleAuthUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) LinkGoogleIdentity(ctx context.Context, link service.GoogleIdentityLink) (service.GoogleAuthUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return service.GoogleAuthUser{}, err
	}
	defer tx.Rollback(ctx)

	var user service.GoogleAuthUser
	var deletedAt sql.NullTime
	var emailVerifiedAt sql.NullTime
	if err := tx.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(username, ''), status, deleted_at, email_verified_at
		FROM users
		WHERE id = $1
			AND deleted_at IS NULL
		FOR UPDATE
	`, link.UserID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Status,
		&deletedAt,
		&emailVerifiedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GoogleAuthUser{}, service.ErrGoogleEmailUserNotFound
		}
		return service.GoogleAuthUser{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_identities (user_id, provider, provider_user_id, provider_email, created_at, updated_at)
		VALUES ($1, 'google', $2, $3, $4, $4)
		ON CONFLICT (provider, provider_user_id)
		DO UPDATE SET provider_email = EXCLUDED.provider_email, updated_at = EXCLUDED.updated_at
	`, link.UserID, link.ProviderUserID, link.ProviderEmail, link.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, $2),
			updated_at = $2
		WHERE id = $1
	`, link.UserID, link.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	} else {
		user.EmailVerifiedAt = &link.VerifiedAt
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	metadata, err := json.Marshal(map[string]any{
		"provider":       "google",
		"provider_email": link.ProviderEmail,
	})
	if err != nil {
		return service.GoogleAuthUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, target_user_id, target_type, target_id, metadata, ip_address, user_agent, created_at
		)
		VALUES (
			'user', 'google_identity_linked', $1, 'user', $1, $2, NULLIF($3, '')::inet, NULLIF($4, ''), $5
		)
	`, link.UserID, metadata, link.IPAddress, link.UserAgent, link.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if err := r.fillGoogleUserAccess(ctx, &user); err != nil {
		return service.GoogleAuthUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) CreateGoogleUser(ctx context.Context, registration service.GoogleUserRegistration) (service.GoogleAuthUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return service.GoogleAuthUser{}, err
	}
	defer tx.Rollback(ctx)

	var roleID string
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM roles
		WHERE lower(COALESCE(NULLIF(slug, ''), role_name)) = lower($1)
		LIMIT 1
	`, registration.RoleSlug).Scan(&roleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GoogleAuthUser{}, service.ErrGoogleDefaultRoleNotFound
		}
		return service.GoogleAuthUser{}, err
	}

	username, err := r.nextAvailableUsername(ctx, tx, registration.UsernameBase)
	if err != nil {
		return service.GoogleAuthUser{}, err
	}

	var user service.GoogleAuthUser
	var emailVerifiedAt sql.NullTime
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, username, status, email_verified_at, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $5, $5)
		RETURNING id, name, email, COALESCE(username, ''), status, email_verified_at
	`, registration.Name, registration.Email, username, registration.Status, registration.VerifiedAt).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Status,
		&emailVerifiedAt,
	); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_profiles (user_id, avatar_url, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $3)
	`, user.ID, registration.AvatarURL, registration.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_identities (user_id, provider, provider_user_id, provider_email, created_at, updated_at)
		VALUES ($1, 'google', $2, $3, $4, $4)
	`, user.ID, registration.Subject, registration.Email, registration.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
	`, user.ID, roleID); err != nil {
		return service.GoogleAuthUser{}, err
	}

	metadata, err := json.Marshal(map[string]any{
		"provider":       "google",
		"provider_email": registration.Email,
		"default_role":   registration.RoleSlug,
		"status":         registration.Status,
	})
	if err != nil {
		return service.GoogleAuthUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module, event, target_user_id, target_type, target_id, metadata, ip_address, user_agent, created_at
		)
		VALUES (
			'user', 'google_user_registered', $1, 'user', $1, $2, NULLIF($3, '')::inet, NULLIF($4, ''), $5
		)
	`, user.ID, metadata, registration.IPAddress, registration.UserAgent, registration.VerifiedAt); err != nil {
		return service.GoogleAuthUser{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return service.GoogleAuthUser{}, err
	}
	if err := r.fillGoogleUserAccess(ctx, &user); err != nil {
		return service.GoogleAuthUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) FindPasswordResetUserByEmail(ctx context.Context, email string) (service.PasswordResetUser, error) {
	email = strings.TrimSpace(email)

	var user service.PasswordResetUser
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, status
		FROM users
		WHERE deleted_at IS NULL
			AND lower(email) = lower($1)
		LIMIT 1
	`, email).Scan(&user.ID, &user.Name, &user.Email, &user.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PasswordResetUser{}, service.ErrPasswordResetUserNotFound
		}
		return service.PasswordResetUser{}, err
	}

	return user, nil
}

func (r *AuthRepository) FindPasswordChangeUser(ctx context.Context, userID string) (service.PasswordChangeUser, error) {
	var user service.PasswordChangeUser
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(password_hash, '')
		FROM users
		WHERE id = $1
			AND deleted_at IS NULL
		LIMIT 1
	`, userID).Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PasswordChangeUser{}, service.ErrPasswordChangeUserNotFound
		}
		return service.PasswordChangeUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) CreateSession(ctx context.Context, session service.NewSession) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO sessions (
			user_id,
			refresh_token_hash,
			device_name,
			user_agent,
			ip_address,
			last_used_at,
			expires_at,
			created_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::inet, now(), $6, now())
		RETURNING id
	`, session.UserID, session.RefreshTokenHash, session.DeviceName, session.UserAgent, session.IPAddress, session.ExpiresAt).Scan(&id); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, now())
	`, id, session.UserID, session.RefreshTokenHash, session.ExpiresAt); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return id, nil
}

func (r *AuthRepository) CreatePasswordResetToken(ctx context.Context, token service.NewPasswordResetToken) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO password_reset_tokens (
			user_id,
			token_hash,
			expires_at,
			created_at
		)
		VALUES ($1, $2, $3, now())
	`, token.UserID, token.TokenHash, token.ExpiresAt)
	return err
}

func (r *AuthRepository) FindPasswordResetTokenByHash(ctx context.Context, tokenHash string) (service.PasswordResetToken, error) {
	var token service.PasswordResetToken
	var usedAt sql.NullTime
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, expires_at, used_at
		FROM password_reset_tokens
		WHERE token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(&token.ID, &token.UserID, &token.ExpiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PasswordResetToken{}, service.ErrPasswordResetTokenNotFound
		}
		return service.PasswordResetToken{}, err
	}
	if usedAt.Valid {
		token.UsedAt = &usedAt.Time
	}
	return token, nil
}

func (r *AuthRepository) CompletePasswordReset(
	ctx context.Context,
	tokenHash string,
	passwordHash string,
	usedAt time.Time,
) (service.PasswordResetUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return service.PasswordResetUser{}, err
	}
	defer tx.Rollback(ctx)

	var tokenID string
	var expiresAt time.Time
	var existingUsedAt sql.NullTime
	var user service.PasswordResetUser
	err = tx.QueryRow(ctx, `
		SELECT prt.id, prt.user_id, prt.expires_at, prt.used_at, u.name, u.email, u.status
		FROM password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		WHERE prt.token_hash = $1
			AND u.deleted_at IS NULL
		LIMIT 1
		FOR UPDATE OF prt, u
	`, tokenHash).Scan(
		&tokenID,
		&user.ID,
		&expiresAt,
		&existingUsedAt,
		&user.Name,
		&user.Email,
		&user.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PasswordResetUser{}, service.ErrPasswordResetTokenNotFound
		}
		return service.PasswordResetUser{}, err
	}
	if existingUsedAt.Valid || !expiresAt.After(usedAt) {
		return service.PasswordResetUser{}, service.ErrPasswordResetTokenNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = $3
		WHERE id = $1
	`, user.ID, passwordHash, usedAt); err != nil {
		return service.PasswordResetUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = $2
		WHERE id = $1 AND used_at IS NULL
	`, tokenID, usedAt); err != nil {
		return service.PasswordResetUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, user.ID, usedAt); err != nil {
		return service.PasswordResetUser{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, user.ID, usedAt); err != nil {
		return service.PasswordResetUser{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return service.PasswordResetUser{}, err
	}
	return user, nil
}

func (r *AuthRepository) ChangePassword(ctx context.Context, change service.PasswordChange) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = $3
		WHERE id = $1
			AND deleted_at IS NULL
	`, change.UserID, change.PasswordHash, change.ChangedAt); err != nil {
		return err
	}

	if change.LogoutOtherDevices {
		if _, err := tx.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = COALESCE(revoked_at, $3)
			WHERE user_id = $1
				AND id <> $2
		`, change.UserID, change.CurrentSessionID, change.ChangedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE refresh_tokens
			SET revoked_at = COALESCE(revoked_at, $3)
			WHERE user_id = $1
				AND session_id <> $2
		`, change.UserID, change.CurrentSessionID, change.ChangedAt); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module,
			event,
			actor_user_id,
			target_user_id,
			target_type,
			target_id,
			metadata,
			ip_address,
			user_agent,
			created_at
		)
		VALUES (
			'user',
			'password_changed',
			$1,
			$1,
			'user',
			$1,
			jsonb_build_object('logout_other_devices', $2::boolean),
			NULLIF($3, '')::inet,
			$4,
			$5
		)
	`, change.UserID, change.LogoutOtherDevices, change.IPAddress, change.UserAgent, change.ChangedAt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AuthRepository) FindRefreshSessionByTokenHash(ctx context.Context, tokenHash string) (service.RefreshSession, error) {
	var session service.RefreshSession
	var tokenRevokedAt sql.NullTime
	var tokenReplacedByID sql.NullString
	var sessionRevokedAt sql.NullTime

	err := r.db.QueryRow(ctx, `
		SELECT
			rt.id,
			s.id,
			u.id,
			u.name,
			u.email,
			COALESCE(u.username, ''),
			u.status,
			rt.revoked_at,
			rt.replaced_by_token_id::text,
			s.revoked_at,
			s.expires_at
		FROM refresh_tokens rt
		JOIN sessions s ON s.id = rt.session_id
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&session.TokenID,
		&session.SessionID,
		&session.UserID,
		&session.Name,
		&session.Email,
		&session.Username,
		&session.Status,
		&tokenRevokedAt,
		&tokenReplacedByID,
		&sessionRevokedAt,
		&session.SessionExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.RefreshSession{}, service.ErrRefreshTokenNotFound
		}
		return service.RefreshSession{}, err
	}

	if tokenRevokedAt.Valid {
		session.TokenRevokedAt = &tokenRevokedAt.Time
	}
	if tokenReplacedByID.Valid {
		session.TokenReplacedByID = tokenReplacedByID.String
	}
	if sessionRevokedAt.Valid {
		session.SessionRevokedAt = &sessionRevokedAt.Time
	}

	roles, err := r.roleSlugs(ctx, session.UserID)
	if err != nil {
		return service.RefreshSession{}, err
	}
	permissions, err := r.permissionSlugs(ctx, session.UserID)
	if err != nil {
		return service.RefreshSession{}, err
	}
	session.Roles = roles
	session.Permissions = permissions

	return session, nil
}

func (r *AuthRepository) FindCurrentUser(ctx context.Context, userID string, sessionID string) (service.CurrentUser, error) {
	var user service.CurrentUser
	var emailVerifiedAt sql.NullTime
	var phoneVerifiedAt sql.NullTime
	var avatarURL sql.NullString
	var bio sql.NullString
	var jobTitle sql.NullString
	var department sql.NullString
	var company sql.NullString
	var address sql.NullString
	var timezoneValue sql.NullString
	var language sql.NullString
	var ipAddress sql.NullString
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime

	err := r.db.QueryRow(ctx, `
		SELECT
			u.id,
			u.name,
			u.email,
			COALESCE(u.username, ''),
			COALESCE(u.phone, ''),
			u.status,
			u.email_verified_at,
			u.phone_verified_at,
			p.avatar_url,
			p.bio,
			p.job_title,
			p.department,
			p.company,
			p.address,
			p.timezone,
			p.language,
			s.id,
			COALESCE(s.device_name, ''),
			s.ip_address::text,
			s.last_used_at,
			s.expires_at,
			s.revoked_at
		FROM users u
		JOIN sessions s ON s.user_id = u.id
		LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = $1
			AND s.id = $2
			AND u.deleted_at IS NULL
		LIMIT 1
	`, userID, sessionID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Username,
		&user.Phone,
		&user.Status,
		&emailVerifiedAt,
		&phoneVerifiedAt,
		&avatarURL,
		&bio,
		&jobTitle,
		&department,
		&company,
		&address,
		&timezoneValue,
		&language,
		&user.Session.ID,
		&user.Session.DeviceName,
		&ipAddress,
		&lastUsedAt,
		&user.Session.ExpiresAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.CurrentUser{}, service.ErrCurrentUserNotFound
		}
		return service.CurrentUser{}, err
	}

	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	}
	if phoneVerifiedAt.Valid {
		user.PhoneVerifiedAt = &phoneVerifiedAt.Time
	}
	if avatarURL.Valid {
		user.Profile.AvatarURL = avatarURL.String
	}
	if bio.Valid {
		user.Profile.Bio = bio.String
	}
	if jobTitle.Valid {
		user.Profile.JobTitle = jobTitle.String
	}
	if department.Valid {
		user.Profile.Department = department.String
	}
	if company.Valid {
		user.Profile.Company = company.String
	}
	if address.Valid {
		user.Profile.Address = address.String
	}
	if timezoneValue.Valid {
		user.Profile.Timezone = timezoneValue.String
	}
	if language.Valid {
		user.Profile.Language = language.String
	}
	if ipAddress.Valid {
		user.Session.IPAddress = ipAddress.String
	}
	if lastUsedAt.Valid {
		user.Session.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		user.Session.RevokedAt = &revokedAt.Time
	}

	roles, err := r.roleSlugs(ctx, user.ID)
	if err != nil {
		return service.CurrentUser{}, err
	}
	permissions, err := r.permissionSlugs(ctx, user.ID)
	if err != nil {
		return service.CurrentUser{}, err
	}
	user.Roles = roles
	user.Permissions = permissions

	return user, nil
}

func (r *AuthRepository) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE id = $1
	`, sessionID, revokedAt)
	return err
}

func (r *AuthRepository) RotateRefreshToken(ctx context.Context, rotation service.RefreshTokenRotation) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var newTokenID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, now())
		RETURNING id
	`, rotation.SessionID, rotation.UserID, rotation.NewTokenHash, rotation.NewTokenExpiresAt).Scan(&newTokenID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2),
			replaced_by_token_id = $3
		WHERE id = $1
	`, rotation.OldTokenID, rotation.RotatedAt, newTokenID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET refresh_token_hash = $2,
			last_used_at = $3,
			expires_at = $4
		WHERE id = $1
	`, rotation.SessionID, rotation.NewTokenHash, rotation.RotatedAt, rotation.NewTokenExpiresAt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AuthRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users
		SET last_login_at = $2, updated_at = now()
		WHERE id = $1
	`, userID, at)
	return err
}

func (r *AuthRepository) RecordLoginHistory(ctx context.Context, history service.LoginHistoryRecord) error {
	var userID any
	if strings.TrimSpace(history.UserID) != "" {
		userID = history.UserID
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO login_histories (
			user_id,
			identifier,
			event,
			success,
			ip_address,
			user_agent,
			device_name,
			reason,
			created_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::inet, $6, $7, $8, now())
	`, userID, history.Identifier, history.Event, history.Success, history.IPAddress, history.UserAgent, history.DeviceName, history.Reason)
	return err
}

func (r *AuthRepository) roleSlugs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT COALESCE(NULLIF(r.slug, ''), r.role_name)
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY 1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

func (r *AuthRepository) permissionSlugs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT permission_slug
		FROM (
			SELECT COALESCE(NULLIF(p.slug, ''), p.permission_name) AS permission_slug
			FROM user_roles ur
			JOIN role_permissions rp ON rp.role_id = ur.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE ur.user_id = $1
				AND ur.organization_id IS NULL
			UNION
			SELECT COALESCE(NULLIF(p.slug, ''), p.permission_name) AS permission_slug
			FROM user_permissions up
			JOIN permissions p ON p.id = up.permission_id
			WHERE up.user_id = $1
				AND up.organization_id IS NULL
				AND up.effect = 'allow'
			EXCEPT
			SELECT COALESCE(NULLIF(p.slug, ''), p.permission_name) AS permission_slug
			FROM user_permissions up
			JOIN permissions p ON p.id = up.permission_id
			WHERE up.user_id = $1
				AND up.organization_id IS NULL
				AND up.effect = 'deny'
		) permissions
		ORDER BY permission_slug
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

func (r *AuthRepository) fillGoogleUserAccess(ctx context.Context, user *service.GoogleAuthUser) error {
	roles, err := r.roleSlugs(ctx, user.ID)
	if err != nil {
		return err
	}
	permissions, err := r.permissionSlugs(ctx, user.ID)
	if err != nil {
		return err
	}
	user.Roles = roles
	user.Permissions = permissions
	return nil
}

func (r *AuthRepository) nextAvailableUsername(ctx context.Context, tx pgx.Tx, base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "google_user"
	}
	if len(base) > 80 {
		base = base[:80]
	}

	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			suffix := fmt.Sprintf("_%d", i+1)
			trimmedBase := base
			if len(trimmedBase)+len(suffix) > 100 {
				trimmedBase = trimmedBase[:100-len(suffix)]
			}
			candidate = trimmedBase + suffix
		}

		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM users
				WHERE lower(username) = lower($1)
					AND deleted_at IS NULL
			)
		`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}

	return "", errors.New("failed to generate unique username")
}

func scanStrings(rows pgx.Rows) ([]string, error) {
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan strings: %w", err)
	}
	return values, nil
}

func (r *AuthRepository) ListSessions(ctx context.Context, userID string) ([]service.CurrentSession, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, device_name, COALESCE(ip_address::text, ''), COALESCE(user_agent, ''), last_used_at, expires_at, revoked_at
		FROM sessions
		WHERE user_id = $1
		ORDER BY last_used_at DESC NULLS LAST, created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]service.CurrentSession, 0)
	for rows.Next() {
		var s service.CurrentSession
		var ipAddress, userAgent string
		var lastUsedAt, revokedAt sql.NullTime
		if err := rows.Scan(
			&s.ID,
			&s.DeviceName,
			&ipAddress,
			&userAgent,
			&lastUsedAt,
			&s.ExpiresAt,
			&revokedAt,
		); err != nil {
			return nil, err
		}
		s.IPAddress = ipAddress
		s.UserAgent = userAgent
		if lastUsedAt.Valid {
			s.LastUsedAt = &lastUsedAt.Time
		}
		if revokedAt.Valid {
			s.RevokedAt = &revokedAt.Time
		}
		sessions = append(sessions, s)
	}

	return sessions, rows.Err()
}

func (r *AuthRepository) RevokeUserSession(ctx context.Context, sessionID string, userID string, revokedAt time.Time) error {
	res, err := r.db.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $1
		WHERE id = $2 AND user_id = $3 AND revoked_at IS NULL
	`, revokedAt, sessionID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return service.ErrSessionNotFound
	}
	return nil
}

func (r *AuthRepository) RevokeAllSessions(ctx context.Context, userID string, excludeSessionID string, revokedAt time.Time) error {
	var err error
	if excludeSessionID != "" {
		_, err = r.db.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = $1
			WHERE user_id = $2 AND id <> $3 AND revoked_at IS NULL AND expires_at > $1
		`, revokedAt, userID, excludeSessionID)
	} else {
		_, err = r.db.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = $1
			WHERE user_id = $2 AND revoked_at IS NULL AND expires_at > $1
		`, revokedAt, userID)
	}
	return err
}

func (r *AuthRepository) CreateEmailVerificationToken(ctx context.Context, token service.NewEmailVerificationToken) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_verification_tokens (
			user_id,
			email,
			token_hash,
			expires_at,
			created_at
		)
		VALUES ($1, $2, $3, $4, now())
	`, token.UserID, token.Email, token.TokenHash, token.ExpiresAt)
	return err
}

func (r *AuthRepository) FindEmailVerificationTokenByHash(ctx context.Context, tokenHash string) (service.EmailVerificationToken, error) {
	var token service.EmailVerificationToken
	var usedAt sql.NullTime
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, email, token_hash, expires_at, used_at
		FROM email_verification_tokens
		WHERE token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(&token.ID, &token.UserID, &token.Email, &token.TokenHash, &token.ExpiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.EmailVerificationToken{}, service.ErrEmailVerificationTokenNotFound
		}
		return service.EmailVerificationToken{}, err
	}
	if usedAt.Valid {
		token.UsedAt = &usedAt.Time
	}
	return token, nil
}

func (r *AuthRepository) CompleteEmailVerification(
	ctx context.Context,
	tokenHash string,
	verifiedAt time.Time,
) (service.EmailVerificationUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return service.EmailVerificationUser{}, err
	}
	defer tx.Rollback(ctx)

	var tokenID string
	var userID string
	var email string
	var expiresAt time.Time
	var existingUsedAt sql.NullTime
	var user service.EmailVerificationUser

	err = tx.QueryRow(ctx, `
		SELECT evt.id, evt.user_id, evt.email, evt.expires_at, evt.used_at, u.name
		FROM email_verification_tokens evt
		JOIN users u ON u.id = evt.user_id
		WHERE evt.token_hash = $1
			AND u.deleted_at IS NULL
		LIMIT 1
		FOR UPDATE OF evt, u
	`, tokenHash).Scan(
		&tokenID,
		&userID,
		&email,
		&expiresAt,
		&existingUsedAt,
		&user.Name,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.EmailVerificationUser{}, service.ErrEmailVerificationTokenNotFound
		}
		return service.EmailVerificationUser{}, err
	}

	user.ID = userID
	user.Email = email

	if existingUsedAt.Valid || !expiresAt.After(verifiedAt) {
		return service.EmailVerificationUser{}, service.ErrEmailVerificationTokenNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET email_verified_at = $2,
			status = CASE WHEN status = 'pending' THEN 'active' ELSE status END,
			updated_at = $2
		WHERE id = $1
	`, userID, verifiedAt); err != nil {
		return service.EmailVerificationUser{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE email_verification_tokens
		SET used_at = $2
		WHERE id = $1
	`, tokenID, verifiedAt); err != nil {
		return service.EmailVerificationUser{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE email_verification_tokens
		SET used_at = $2
		WHERE user_id = $1 AND id <> $3 AND used_at IS NULL
	`, userID, verifiedAt, tokenID); err != nil {
		return service.EmailVerificationUser{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return service.EmailVerificationUser{}, err
	}

	return user, nil
}

func (r *AuthRepository) FindEmailVerificationUserByEmail(ctx context.Context, email string) (service.EmailVerificationTargetUser, error) {
	email = strings.TrimSpace(email)

	var user service.EmailVerificationTargetUser
	var emailVerifiedAt sql.NullTime

	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, status, email_verified_at
		FROM users
		WHERE deleted_at IS NULL
			AND lower(email) = lower($1)
		LIMIT 1
	`, email).Scan(&user.ID, &user.Name, &user.Email, &user.Status, &emailVerifiedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.EmailVerificationTargetUser{}, service.ErrLoginUserNotFound
		}
		return service.EmailVerificationTargetUser{}, err
	}

	if emailVerifiedAt.Valid {
		user.EmailVerifiedAt = &emailVerifiedAt.Time
	}

	return user, nil
}
