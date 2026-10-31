package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/platform/database"
)

const routerNameUniqueIndex = "idx_mikrotik_routers_org_name_unique"

// CreateRouterParams carries the values the service sets on a new router.
// PasswordEncrypted must already be ciphertext.
type CreateRouterParams struct {
	Name              string
	Host              string
	Port              int
	APIType           domain.APIType
	UseTLS            bool
	TLSFingerprint    string
	Username          string
	PasswordEncrypted string
	RouterOSVersion   string
	CreatedBy         string
}

// UpdateRouterParams is a partial update: a nil pointer means "leave the column
// unchanged" (e.g. PasswordEncrypted nil keeps the stored password). A non-nil
// pointer sets the column; for the string columns that are nullable in the table
// (TLSFingerprint, RouterOSVersion) an empty string stores NULL.
//
// When Host, Port, APIType, UseTLS or TLSFingerprint is provided and differs from
// the stored value, the repository resets isolir_ready/isolir_ready_at in the same
// statement, because a changed endpoint requires a new readiness check.
type UpdateRouterParams struct {
	Name              *string
	Host              *string
	Port              *int
	APIType           *domain.APIType
	UseTLS            *bool
	TLSFingerprint    *string
	Username          *string
	PasswordEncrypted *string
	RouterOSVersion   *string
	UpdatedBy         string
}

// AuditEntry is written to audit_logs with module 'mikrotik' and target_type
// 'mikrotik_router'. The organization comes from the scope. An empty ActorUserID
// stores NULL.
type AuditEntry struct {
	Event       string
	ActorUserID string
	TargetID    string
	Metadata    map[string]any
}

type RouterRepository interface {
	Create(ctx context.Context, scope coretenant.Scope, params CreateRouterParams) (domain.Router, error)
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error)
	List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Router, int64, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateRouterParams) (domain.Router, error)
	// RecordCheck stores the outcome of a connectivity check; an empty version keeps the stored one.
	RecordCheck(ctx context.Context, scope coretenant.Scope, id, status, version string) error
	// MarkSetup stores the isolir setup result; isolir_ready_at is set only when ready is true.
	MarkSetup(ctx context.Context, scope coretenant.Scope, id string, ready bool, profile, redirectURL string) error
	SoftDelete(ctx context.Context, scope coretenant.Scope, id string, by string) error
	// HasReadyRouter reports whether the organization has at least one
	// non-deleted router with isolir_ready = true.
	HasReadyRouter(ctx context.Context, scope coretenant.Scope) (bool, error)
	InsertAudit(ctx context.Context, scope coretenant.Scope, entry AuditEntry) error
}

type routerRepository struct {
	db *database.Pool
}

func NewRouterRepository(db *database.Pool) RouterRepository {
	return &routerRepository{db: db}
}

const routerColumns = `
	id, organization_id, name, host, port, api_type, use_tls, tls_fingerprint, username, password_encrypted,
	isolir_ready, isolir_ready_at, isolir_profile_name, isolir_redirect_url,
	last_check_at, last_check_status, routeros_version, created_at, updated_at
`

func scanRouter(row pgx.Row) (domain.Router, error) {
	var r domain.Router
	var apiType string
	var fingerprint, profile, redirectURL, checkStatus, version *string
	err := row.Scan(
		&r.ID, &r.OrganizationID, &r.Name, &r.Host, &r.Port, &apiType, &r.UseTLS, &fingerprint, &r.Username, &r.PasswordEncrypted,
		&r.IsolirReady, &r.IsolirReadyAt, &profile, &redirectURL,
		&r.LastCheckAt, &checkStatus, &version, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return domain.Router{}, err
	}
	r.APIType = domain.APIType(apiType)
	r.TLSFingerprint = derefString(fingerprint)
	r.IsolirProfileName = derefString(profile)
	r.IsolirRedirectURL = derefString(redirectURL)
	r.LastCheckStatus = derefString(checkStatus)
	r.RouterOSVersion = derefString(version)
	return r, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mapRouterError translates database errors into module errors.
func mapRouterError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return mikrotik.ErrRouterNotFound
	case isUniqueViolation(err, routerNameUniqueIndex):
		return mikrotik.ErrRouterNameTaken
	default:
		return err
	}
}

func (r *routerRepository) Create(ctx context.Context, scope coretenant.Scope, p CreateRouterParams) (domain.Router, error) {
	query := `
		INSERT INTO mikrotik_routers (
			organization_id, name, host, port, api_type, use_tls, tls_fingerprint, username, password_encrypted,
			routeros_version, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
		RETURNING ` + routerColumns

	var out domain.Router
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanRouter(tx.QueryRow(ctx, query,
			scope.OrganizationID(), p.Name, p.Host, p.Port, string(p.APIType), p.UseTLS,
			nullableString(p.TLSFingerprint), p.Username, p.PasswordEncrypted,
			nullableString(p.RouterOSVersion), nullableString(p.CreatedBy),
		))
		return scanErr
	})
	if err != nil {
		return domain.Router{}, mapRouterError(err)
	}
	return out, nil
}

func (r *routerRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error) {
	query := `SELECT ` + routerColumns + ` FROM mikrotik_routers WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var out domain.Router
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanRouter(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Router{}, mapRouterError(err)
	}
	return out, nil
}

func (r *routerRepository) List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Router, int64, error) {
	limit = clampLimit(limit)
	if offset < 0 {
		offset = 0
	}
	orgID := scope.OrganizationID()

	items := []domain.Router{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM mikrotik_routers WHERE organization_id = $1 AND deleted_at IS NULL`, orgID,
		).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+routerColumns+` FROM mikrotik_routers
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY lower(name), id LIMIT $2 OFFSET $3`, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, scanErr := scanRouter(rows)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *routerRepository) Update(ctx context.Context, scope coretenant.Scope, id string, p UpdateRouterParams) (domain.Router, error) {
	if !scope.IsValid() {
		return domain.Router{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = now()"}
	var endpointChanged []string // conditions comparing the stored value with the new one
	var args []interface{}
	addArg := func(value interface{}) int {
		args = append(args, value)
		return len(args)
	}
	set := func(column string, value interface{}, endpoint bool, pgType string) {
		n := addArg(value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d::%s", column, n, pgType))
		if endpoint {
			endpointChanged = append(endpointChanged, fmt.Sprintf("%s IS DISTINCT FROM $%d::%s", column, n, pgType))
		}
	}

	if p.Name != nil {
		set("name", *p.Name, false, "text")
	}
	if p.Host != nil {
		set("host", *p.Host, true, "text")
	}
	if p.Port != nil {
		set("port", *p.Port, true, "integer")
	}
	if p.APIType != nil {
		set("api_type", string(*p.APIType), true, "text")
	}
	if p.UseTLS != nil {
		set("use_tls", *p.UseTLS, true, "boolean")
	}
	if p.TLSFingerprint != nil {
		set("tls_fingerprint", nullableString(*p.TLSFingerprint), true, "text")
	}
	if p.Username != nil {
		set("username", *p.Username, false, "text")
	}
	if p.PasswordEncrypted != nil {
		set("password_encrypted", *p.PasswordEncrypted, false, "text")
	}
	if p.RouterOSVersion != nil {
		set("routeros_version", nullableString(*p.RouterOSVersion), false, "text")
	}
	if p.UpdatedBy != "" {
		set("updated_by", p.UpdatedBy, false, "uuid")
	}
	if len(endpointChanged) > 0 {
		// SET expressions read the pre-update row, so this compares old vs new values.
		cond := strings.Join(endpointChanged, " OR ")
		setClauses = append(setClauses,
			fmt.Sprintf("isolir_ready = CASE WHEN %s THEN false ELSE isolir_ready END", cond),
			fmt.Sprintf("isolir_ready_at = CASE WHEN %s THEN NULL ELSE isolir_ready_at END", cond),
		)
	}

	idArg := addArg(id)
	orgArg := addArg(scope.OrganizationID())
	query := fmt.Sprintf("UPDATE mikrotik_routers SET %s WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING %s",
		strings.Join(setClauses, ", "), idArg, orgArg, routerColumns)

	var out domain.Router
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanRouter(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Router{}, mapRouterError(err)
	}
	return out, nil
}

func (r *routerRepository) RecordCheck(ctx context.Context, scope coretenant.Scope, id, status, version string) error {
	return r.execOne(ctx, scope, `
		UPDATE mikrotik_routers
		SET last_check_at = now(), last_check_status = $1, routeros_version = COALESCE($2, routeros_version), updated_at = now()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL`,
		status, nullableString(version), id, scope.OrganizationID())
}

func (r *routerRepository) MarkSetup(ctx context.Context, scope coretenant.Scope, id string, ready bool, profile, redirectURL string) error {
	return r.execOne(ctx, scope, `
		UPDATE mikrotik_routers
		SET isolir_ready = $1,
			isolir_ready_at = CASE WHEN $1::boolean THEN now() ELSE NULL END,
			isolir_profile_name = $2, isolir_redirect_url = $3, updated_at = now()
		WHERE id = $4 AND organization_id = $5 AND deleted_at IS NULL`,
		ready, nullableString(profile), nullableString(redirectURL), id, scope.OrganizationID())
}

func (r *routerRepository) SoftDelete(ctx context.Context, scope coretenant.Scope, id string, by string) error {
	return r.execOne(ctx, scope, `
		UPDATE mikrotik_routers
		SET deleted_at = now(), updated_at = now(), updated_by = COALESCE($1, updated_by)
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL`,
		nullableString(by), id, scope.OrganizationID())
}

func (r *routerRepository) HasReadyRouter(ctx context.Context, scope coretenant.Scope) (bool, error) {
	var ok bool
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM mikrotik_routers
			WHERE organization_id = $1 AND isolir_ready AND deleted_at IS NULL)`, scope.OrganizationID()).Scan(&ok)
	})
	return ok, err
}

// execOne runs a single-row UPDATE in a scoped transaction and returns ErrRouterNotFound when no row matched.
func (r *routerRepository) execOne(ctx context.Context, scope coretenant.Scope, query string, args ...interface{}) error {
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
	return mapRouterError(err)
}

func (r *routerRepository) InsertAudit(ctx context.Context, scope coretenant.Scope, entry AuditEntry) error {
	// organization_id is stored both as a column (tenant filtering) and inside metadata.
	metadata := make(map[string]any, len(entry.Metadata)+1)
	for k, v := range entry.Metadata {
		metadata[k] = v
	}
	metadata["organization_id"] = scope.OrganizationID()

	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_logs (module, event, actor_user_id, target_type, target_id, metadata, organization_id)
			VALUES ('mikrotik', $1, $2, 'mikrotik_router', $3, $4, $5)`,
			entry.Event, nullableString(entry.ActorUserID), nullableString(entry.TargetID), metadata, scope.OrganizationID())
		return err
	})
}
