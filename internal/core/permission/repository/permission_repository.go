package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/platform/database"
)

type Repository struct {
	db *database.Pool
}

func New(db *database.Pool) *Repository {
	return &Repository{db: db}
}

// === PERMISSION CRUD ===

func (r *Repository) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, slug, module, action, COALESCE(description, ''), created_at, updated_at
		FROM permissions
		ORDER BY slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]domain.Permission, 0)
	for rows.Next() {
		var p domain.Permission
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Slug,
			&p.Module,
			&p.Action,
			&p.Description,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		p.CreatedAt = createdAt
		p.UpdatedAt = updatedAt
		p.ModuleID = "" // Deprecated field in schema normalization
		permissions = append(permissions, p)
	}

	return permissions, rows.Err()
}

func (r *Repository) GetPermissionByID(ctx context.Context, id string) (domain.Permission, error) {
	var p domain.Permission
	var createdAt, updatedAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, name, slug, module, action, COALESCE(description, ''), created_at, updated_at
		FROM permissions
		WHERE id = $1
	`, id).Scan(
		&p.ID,
		&p.Name,
		&p.Slug,
		&p.Module,
		&p.Action,
		&p.Description,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Permission{}, err
	}
	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	return p, nil
}

func (r *Repository) CreatePermission(ctx context.Context, p *domain.Permission) error {
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow(ctx, `
		INSERT INTO permissions (name, slug, module, action, permission_name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $2, $5, now(), now())
		RETURNING id, created_at, updated_at
	`, p.Name, p.Slug, p.Module, p.Action, p.Description).Scan(&p.ID, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	p.CreatedAt = &createdAt
	p.UpdatedAt = &updatedAt
	return nil
}

func (r *Repository) UpdatePermission(ctx context.Context, id string, p *domain.Permission) error {
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow(ctx, `
		UPDATE permissions
		SET name = $1, slug = $2, module = $3, action = $4, permission_name = $2, description = $5, updated_at = now()
		WHERE id = $6
		RETURNING created_at, updated_at
	`, p.Name, p.Slug, p.Module, p.Action, p.Description, id).Scan(&createdAt, &updatedAt)
	if err != nil {
		return err
	}
	p.CreatedAt = &createdAt
	p.UpdatedAt = &updatedAt
	return nil
}

func (r *Repository) DeletePermission(ctx context.Context, id string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM permissions WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// === ROLE CRUD ===

func (r *Repository) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, role_name, slug, COALESCE(description, ''), is_system, created_at, updated_at
		FROM roles
		ORDER BY slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]domain.Role, 0)
	for rows.Next() {
		var role domain.Role
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(
			&role.ID,
			&role.Name,
			&role.Slug,
			&role.Description,
			&role.IsSystem,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		role.CreatedAt = createdAt
		role.UpdatedAt = updatedAt
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

func (r *Repository) GetRoleByID(ctx context.Context, id string) (domain.Role, error) {
	var role domain.Role
	var createdAt, updatedAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, role_name, slug, COALESCE(description, ''), is_system, created_at, updated_at
		FROM roles
		WHERE id = $1
	`, id).Scan(
		&role.ID,
		&role.Name,
		&role.Slug,
		&role.Description,
		&role.IsSystem,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Role{}, err
	}
	role.CreatedAt = createdAt
	role.UpdatedAt = updatedAt

	// Load role permissions mapping
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.name, p.slug, p.module, p.action, COALESCE(p.description, ''), p.created_at, p.updated_at
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.slug
	`, id)
	if err != nil {
		return domain.Role{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Permission
		var pCreatedAt, pUpdatedAt *time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Module, &p.Action, &p.Description, &pCreatedAt, &pUpdatedAt); err != nil {
			return domain.Role{}, err
		}
		p.CreatedAt = pCreatedAt
		p.UpdatedAt = pUpdatedAt
		role.Permissions = append(role.Permissions, p)
	}

	return role, rows.Err()
}

func (r *Repository) GetRolePermissions(ctx context.Context, roleName string) (domain.Role, error) {
	var role domain.Role
	var createdAt, updatedAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, role_name, slug, COALESCE(description, ''), is_system, created_at, updated_at
		FROM roles
		WHERE role_name = $1 OR slug = $1
	`, roleName).Scan(
		&role.ID,
		&role.Name,
		&role.Slug,
		&role.Description,
		&role.IsSystem,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Role{}, err
	}
	role.CreatedAt = createdAt
	role.UpdatedAt = updatedAt

	rows, err := r.db.Query(ctx, `
		SELECT
			p.id,
			p.name,
			p.slug,
			p.module,
			p.action,
			COALESCE(p.description, ''),
			p.created_at,
			p.updated_at
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		JOIN roles r ON r.id = rp.role_id
		WHERE r.id = $1
		ORDER BY p.slug
	`, role.ID)
	if err != nil {
		return domain.Role{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Permission
		var pCreatedAt, pUpdatedAt *time.Time
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Slug,
			&p.Module,
			&p.Action,
			&p.Description,
			&pCreatedAt,
			&pUpdatedAt,
		); err != nil {
			return domain.Role{}, err
		}
		p.CreatedAt = pCreatedAt
		p.UpdatedAt = pUpdatedAt
		role.Permissions = append(role.Permissions, p)
	}

	if err := rows.Err(); err != nil {
		return domain.Role{}, err
	}

	return role, nil
}

func (r *Repository) CreateRole(ctx context.Context, role *domain.Role) error {
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, description, is_system, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		RETURNING id, created_at, updated_at
	`, role.Name, role.Slug, role.Description, role.IsSystem).Scan(&role.ID, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	role.CreatedAt = &createdAt
	role.UpdatedAt = &updatedAt
	return nil
}

func (r *Repository) UpdateRole(ctx context.Context, id string, role *domain.Role) error {
	var createdAt, updatedAt time.Time
	err := r.db.QueryRow(ctx, `
		UPDATE roles
		SET role_name = $1, slug = $2, description = $3, updated_at = now()
		WHERE id = $4
		RETURNING is_system, created_at, updated_at
	`, role.Name, role.Slug, role.Description, id).Scan(&role.IsSystem, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	role.CreatedAt = &createdAt
	role.UpdatedAt = &updatedAt
	return nil
}

func (r *Repository) DeleteRole(ctx context.Context, id string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM roles WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// === ROLE PERMISSION MAPPINGS ===

func (r *Repository) GetRolePermissionsByID(ctx context.Context, roleID string) ([]domain.Permission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.name, p.slug, p.module, p.action, COALESCE(p.description, ''), p.created_at, p.updated_at
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.slug
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]domain.Permission, 0)
	for rows.Next() {
		var p domain.Permission
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Module, &p.Action, &p.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.CreatedAt = createdAt
		p.UpdatedAt = updatedAt
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

func (r *Repository) AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, permID := range permissionIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
			VALUES ($1, $2, 'organization', now())
			ON CONFLICT (role_id, permission_id) DO NOTHING
		`, roleID, permID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) AssignRolePermission(ctx context.Context, roleID string, permID string, scope string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (role_id, permission_id) DO UPDATE
		SET scope = EXCLUDED.scope, granted_at = now()
	`, roleID, permID, scope)
	return err
}

func (r *Repository) RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, permID := range permissionIDs {
		_, err := tx.Exec(ctx, `
			DELETE FROM role_permissions
			WHERE role_id = $1 AND permission_id = $2
		`, roleID, permID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeRolePermission(ctx context.Context, roleID string, permID string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM role_permissions WHERE role_id = $1 AND permission_id = $2
	`, roleID, permID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// === USER ROLE MAPPINGS ===

func (r *Repository) ListUserRoles(ctx context.Context, userID string) ([]domain.UserRole, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ur.id, ur.user_id, ur.role_id, r.slug, ur.organization_id, ur.assigned_by, ur.assigned_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.slug
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]domain.UserRole, 0)
	for rows.Next() {
		var ur domain.UserRole
		var orgID, assignedBy *string
		if err := rows.Scan(
			&ur.ID,
			&ur.UserID,
			&ur.RoleID,
			&ur.RoleSlug,
			&orgID,
			&assignedBy,
			&ur.AssignedAt,
		); err != nil {
			return nil, err
		}
		ur.OrganizationID = orgID
		ur.AssignedBy = assignedBy
		roles = append(roles, ur)
	}
	return roles, rows.Err()
}

func (r *Repository) AssignUserRole(ctx context.Context, userID string, roleID string, orgID *string, assignedBy string) error {
	var assignedByPtr *string
	if assignedBy != "" {
		assignedByPtr = &assignedBy
	}

	query := `
		INSERT INTO user_roles (id, user_id, role_id, organization_id, assigned_by, assigned_at)
		VALUES (gen_random_uuid(), $1, $2, NULL, $3, now())
		ON CONFLICT (user_id, role_id) WHERE organization_id IS NULL
		DO UPDATE SET assigned_by = EXCLUDED.assigned_by, assigned_at = now()
	`
	args := []any{userID, roleID, assignedByPtr}
	if orgID != nil {
		query = `
			INSERT INTO user_roles (id, user_id, role_id, organization_id, assigned_by, assigned_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, now())
			ON CONFLICT (user_id, role_id, organization_id) WHERE organization_id IS NOT NULL
			DO UPDATE SET assigned_by = EXCLUDED.assigned_by, assigned_at = now()
		`
		args = []any{userID, roleID, orgID, assignedByPtr}
	}

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *Repository) AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, roleID := range roleIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO user_roles (id, user_id, role_id, assigned_at)
			VALUES (gen_random_uuid(), $1, $2, now())
			ON CONFLICT (user_id, role_id) WHERE organization_id IS NULL DO NOTHING
		`, userID, roleID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeUserRole(ctx context.Context, userID string, roleID string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2
	`, userID, roleID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, roleID := range roleIDs {
		_, err := tx.Exec(ctx, `
			DELETE FROM user_roles
			WHERE user_id = $1 AND role_id = $2
		`, userID, roleID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// === USER DIRECT PERMISSION MAPPINGS ===

func (r *Repository) ListUserPermissions(ctx context.Context, userID string) ([]domain.UserPermission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT up.id, up.user_id, up.permission_id, p.slug, up.organization_id, up.effect, up.assigned_by, up.assigned_at, up.created_at
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.user_id = $1
		ORDER BY p.slug
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]domain.UserPermission, 0)
	for rows.Next() {
		var up domain.UserPermission
		var orgID, assignedBy *string
		if err := rows.Scan(
			&up.ID,
			&up.UserID,
			&up.PermissionID,
			&up.PermissionSlug,
			&orgID,
			&up.Effect,
			&assignedBy,
			&up.AssignedAt,
			&up.CreatedAt,
		); err != nil {
			return nil, err
		}
		up.OrganizationID = orgID
		up.AssignedBy = assignedBy
		permissions = append(permissions, up)
	}
	return permissions, rows.Err()
}

func (r *Repository) AssignUserPermission(ctx context.Context, userID string, permID string, effect string, orgID *string, assignedBy string) error {
	var assignedByPtr *string
	if assignedBy != "" {
		assignedByPtr = &assignedBy
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Check if record exists under global or organization-specific context
	var existingID string
	var queryErr error
	if orgID == nil {
		queryErr = tx.QueryRow(ctx, `
			SELECT id FROM user_permissions
			WHERE user_id = $1 AND permission_id = $2 AND organization_id IS NULL
		`, userID, permID).Scan(&existingID)
	} else {
		queryErr = tx.QueryRow(ctx, `
			SELECT id FROM user_permissions
			WHERE user_id = $1 AND permission_id = $2 AND organization_id = $3
		`, userID, permID, *orgID).Scan(&existingID)
	}

	if queryErr != nil && !errors.Is(queryErr, pgx.ErrNoRows) {
		return queryErr
	}

	if errors.Is(queryErr, pgx.ErrNoRows) {
		// INSERT
		_, err = tx.Exec(ctx, `
			INSERT INTO user_permissions (id, user_id, permission_id, organization_id, effect, assigned_by, assigned_at, created_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
		`, userID, permID, orgID, effect, assignedByPtr)
	} else {
		// UPDATE
		_, err = tx.Exec(ctx, `
			UPDATE user_permissions
			SET effect = $1, assigned_by = $2, assigned_at = now()
			WHERE id = $3
		`, effect, assignedByPtr, existingID)
	}

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeUserPermission(ctx context.Context, userID string, permID string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM user_permissions WHERE user_id = $1 AND permission_id = $2
	`, userID, permID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// === EFFECTIVE PERMISSIONS EVALUATION ===

func (r *Repository) GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error) {
	return r.getUserPermissions(ctx, userID, "")
}

func (r *Repository) GetUserOrganizationPermissions(
	ctx context.Context,
	userID string,
	organizationID string,
) (domain.UserPermissionSet, error) {
	return r.getUserPermissions(ctx, userID, organizationID)
}

func (r *Repository) getUserPermissions(
	ctx context.Context,
	userID string,
	organizationID string,
) (domain.UserPermissionSet, error) {
	// Query 1: Get permissions granted via roles
	roleRows, err := r.db.Query(ctx, `
		SELECT DISTINCT
			r.role_name,
			p.id,
			p.name,
			p.slug,
			p.module,
			p.action,
			COALESCE(p.description, ''),
			p.created_at,
			p.updated_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN role_permissions rp ON rp.role_id = r.id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
			AND (
				(NULLIF($2, '')::uuid IS NULL AND ur.organization_id IS NULL)
				OR ur.organization_id = NULLIF($2, '')::uuid
			)
		ORDER BY r.role_name, p.slug
	`, userID, organizationID)
	if err != nil {
		return domain.UserPermissionSet{}, err
	}
	defer roleRows.Close()

	roleNames := make([]string, 0)
	roleNamesMap := make(map[string]struct{})
	effectivePerms := make(map[string]domain.Permission)

	for roleRows.Next() {
		var roleName string
		var p domain.Permission
		var createdAt, updatedAt *time.Time
		if err := roleRows.Scan(
			&roleName,
			&p.ID,
			&p.Name,
			&p.Slug,
			&p.Module,
			&p.Action,
			&p.Description,
			&createdAt,
			&updatedAt,
		); err != nil {
			return domain.UserPermissionSet{}, err
		}
		p.CreatedAt = createdAt
		p.UpdatedAt = updatedAt

		if _, exists := roleNamesMap[roleName]; !exists {
			roleNamesMap[roleName] = struct{}{}
			roleNames = append(roleNames, roleName)
		}
		// Role permissions are implicitly "allow"
		effectivePerms[p.Slug] = p
	}

	if err := roleRows.Err(); err != nil {
		return domain.UserPermissionSet{}, err
	}

	// Query 2: Get direct permissions with overrides
	directRows, err := r.db.Query(ctx, `
		SELECT
			p.id,
			p.name,
			p.slug,
			p.module,
			p.action,
			COALESCE(p.description, ''),
			up.effect,
			p.created_at,
			p.updated_at
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.user_id = $1
			AND (
				(NULLIF($2, '')::uuid IS NULL AND up.organization_id IS NULL)
				OR up.organization_id = NULLIF($2, '')::uuid
			)
		ORDER BY p.slug
	`, userID, organizationID)
	if err != nil {
		return domain.UserPermissionSet{}, err
	}
	defer directRows.Close()

	for directRows.Next() {
		var p domain.Permission
		var effect string
		var createdAt, updatedAt *time.Time
		if err := directRows.Scan(
			&p.ID,
			&p.Name,
			&p.Slug,
			&p.Module,
			&p.Action,
			&p.Description,
			&effect,
			&createdAt,
			&updatedAt,
		); err != nil {
			return domain.UserPermissionSet{}, err
		}
		p.CreatedAt = createdAt
		p.UpdatedAt = updatedAt

		if effect == "deny" {
			delete(effectivePerms, p.Slug)
		} else if effect == "allow" {
			effectivePerms[p.Slug] = p
		}
	}

	if err := directRows.Err(); err != nil {
		return domain.UserPermissionSet{}, err
	}

	// Assemble final list of permissions
	result := domain.UserPermissionSet{
		UserID:      userID,
		RoleNames:   roleNames,
		Permissions: make([]domain.Permission, 0, len(effectivePerms)),
	}
	for _, p := range effectivePerms {
		result.Permissions = append(result.Permissions, p)
	}

	return result, nil
}

// === MATRIX SUPPORT ===

func (r *Repository) ListRolePermissionsMatrix(ctx context.Context) (map[string]map[string]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT role_id, permission_id, scope
		FROM role_permissions
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matrix := make(map[string]map[string]string)
	for rows.Next() {
		var roleID, permID, scope string
		if err := rows.Scan(&roleID, &permID, &scope); err != nil {
			return nil, err
		}
		if _, ok := matrix[roleID]; !ok {
			matrix[roleID] = make(map[string]string)
		}
		matrix[roleID][permID] = scope
	}
	return matrix, rows.Err()
}
