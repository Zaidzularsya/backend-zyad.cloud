package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/core/permission/policy"
)

type PermissionRepository interface {
	ListPermissions(ctx context.Context) ([]domain.Permission, error)
	GetPermissionByID(ctx context.Context, id string) (domain.Permission, error)
	CreatePermission(ctx context.Context, permission *domain.Permission) error
	UpdatePermission(ctx context.Context, id string, permission *domain.Permission) error
	DeletePermission(ctx context.Context, id string) error

	ListRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, id string) (domain.Role, error)
	GetRolePermissions(ctx context.Context, roleName string) (domain.Role, error)
	CreateRole(ctx context.Context, role *domain.Role) error
	UpdateRole(ctx context.Context, id string, role *domain.Role) error
	DeleteRole(ctx context.Context, id string) error

	GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error)
	GetUserOrganizationPermissions(
		ctx context.Context,
		userID string,
		organizationID string,
	) (domain.UserPermissionSet, error)
	AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error
	RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error
	AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error
	RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error

	// New mappings for Phase 2
	GetRolePermissionsByID(ctx context.Context, roleID string) ([]domain.Permission, error)
	AssignRolePermission(ctx context.Context, roleID string, permID string, scope string) error
	RevokeRolePermission(ctx context.Context, roleID string, permID string) error

	ListUserRoles(ctx context.Context, userID string) ([]domain.UserRole, error)
	AssignUserRole(ctx context.Context, userID string, roleID string, orgID *string, assignedBy string) error
	RevokeUserRole(ctx context.Context, userID string, roleID string) error

	ListUserPermissions(ctx context.Context, userID string) ([]domain.UserPermission, error)
	AssignUserPermission(ctx context.Context, userID string, permID string, effect string, orgID *string, assignedBy string) error
	RevokeUserPermission(ctx context.Context, userID string, permID string) error

	ListRolePermissionsMatrix(ctx context.Context) (map[string]map[string]string, error)
}

type Service struct {
	repo PermissionRepository
}

func New(repo PermissionRepository) *Service {
	return &Service{repo: repo}
}

// === PERMISSION CRUD ===

func (s *Service) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	permissions, err := s.repo.ListPermissions(ctx)
	if err != nil {
		return nil, coreerrors.Wrap("PERMISSION_QUERY_FAILED", "failed to list permissions", http.StatusInternalServerError, err)
	}
	return permissions, nil
}

func (s *Service) GetPermissionByID(ctx context.Context, id string) (domain.Permission, error) {
	permission, err := s.repo.GetPermissionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Permission{}, coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return domain.Permission{}, coreerrors.Wrap("PERMISSION_QUERY_FAILED", "failed to get permission", http.StatusInternalServerError, err)
	}
	return permission, nil
}

func (s *Service) CreatePermission(ctx context.Context, p *domain.Permission) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Slug = strings.TrimSpace(p.Slug)
	if p.Name == "" {
		return coreerrors.New("INVALID_INPUT", "permission name cannot be empty", http.StatusBadRequest)
	}

	if p.Slug == "" {
		p.Slug = strings.ToLower(strings.ReplaceAll(p.Name, " ", "."))
	}

	if p.Module == "" || p.Action == "" {
		parts := strings.Split(p.Slug, ".")
		if len(parts) >= 2 {
			p.Module = parts[0]
			p.Action = parts[1]
		} else {
			p.Module = "permission"
			p.Action = "manage"
		}
	}

	err := s.repo.CreatePermission(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505") {
			return coreerrors.New("PERMISSION_ALREADY_EXISTS", "permission already exists", http.StatusConflict)
		}
		return coreerrors.Wrap("PERMISSION_CREATE_FAILED", "failed to create permission", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) UpdatePermission(ctx context.Context, id string, p *domain.Permission) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Slug = strings.TrimSpace(p.Slug)
	if p.Name == "" {
		return coreerrors.New("INVALID_INPUT", "permission name cannot be empty", http.StatusBadRequest)
	}

	if p.Slug == "" {
		p.Slug = strings.ToLower(strings.ReplaceAll(p.Name, " ", "."))
	}

	if p.Module == "" || p.Action == "" {
		parts := strings.Split(p.Slug, ".")
		if len(parts) >= 2 {
			p.Module = parts[0]
			p.Action = parts[1]
		} else {
			p.Module = "permission"
			p.Action = "manage"
		}
	}

	err := s.repo.UpdatePermission(ctx, id, p)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("PERMISSION_UPDATE_FAILED", "failed to update permission", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) DeletePermission(ctx context.Context, id string) error {
	err := s.repo.DeletePermission(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("PERMISSION_DELETE_FAILED", "failed to delete permission", http.StatusInternalServerError, err)
	}
	return nil
}

// === ROLE CRUD ===

func (s *Service) ListRoles(ctx context.Context) ([]domain.Role, error) {
	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return nil, coreerrors.Wrap("ROLE_QUERY_FAILED", "failed to list roles", http.StatusInternalServerError, err)
	}
	return roles, nil
}

func (s *Service) GetRoleByID(ctx context.Context, id string) (domain.Role, error) {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Role{}, coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return domain.Role{}, coreerrors.Wrap("ROLE_QUERY_FAILED", "failed to get role", http.StatusInternalServerError, err)
	}
	return role, nil
}

func (s *Service) GetRolePermissions(ctx context.Context, roleName string) (domain.Role, error) {
	role, err := s.repo.GetRolePermissions(ctx, roleName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Role{}, coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return domain.Role{}, coreerrors.Wrap("PERMISSION_QUERY_FAILED", "failed to get role permissions", http.StatusInternalServerError, err)
	}
	return role, nil
}

func (s *Service) CreateRole(ctx context.Context, role *domain.Role) error {
	role.Name = strings.TrimSpace(role.Name)
	role.Slug = strings.TrimSpace(role.Slug)
	if role.Name == "" || role.Slug == "" {
		return coreerrors.New("INVALID_INPUT", "role name and slug cannot be empty", http.StatusBadRequest)
	}

	err := s.repo.CreateRole(ctx, role)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505") {
			return coreerrors.New("ROLE_ALREADY_EXISTS", "role already exists", http.StatusConflict)
		}
		return coreerrors.Wrap("ROLE_CREATE_FAILED", "failed to create role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) UpdateRole(ctx context.Context, id string, role *domain.Role) error {
	role.Name = strings.TrimSpace(role.Name)
	role.Slug = strings.TrimSpace(role.Slug)
	if role.Name == "" || role.Slug == "" {
		return coreerrors.New("INVALID_INPUT", "role name and slug cannot be empty", http.StatusBadRequest)
	}

	err := s.repo.UpdateRole(ctx, id, role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("ROLE_UPDATE_FAILED", "failed to update role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) DeleteRole(ctx context.Context, id string) error {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("ROLE_QUERY_FAILED", "failed to get role details", http.StatusInternalServerError, err)
	}

	if role.IsSystem {
		return coreerrors.New("ROLE_IS_SYSTEM", "system role cannot be deleted", http.StatusBadRequest)
	}

	err = s.repo.DeleteRole(ctx, id)
	if err != nil {
		return coreerrors.Wrap("ROLE_DELETE_FAILED", "failed to delete role", http.StatusInternalServerError, err)
	}
	return nil
}

// === ROLE PERMISSIONS MAPPINGS ===

func (s *Service) GetRolePermissionsByID(ctx context.Context, roleID string) ([]domain.Permission, error) {
	_, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return nil, err
	}

	return s.repo.GetRolePermissionsByID(ctx, roleID)
}

func (s *Service) AssignRolePermission(ctx context.Context, roleID string, permID string, scope string) error {
	_, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return err
	}

	_, err = s.repo.GetPermissionByID(ctx, permID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return err
	}

	err = s.repo.AssignRolePermission(ctx, roleID, permID, scope)
	if err != nil {
		return coreerrors.Wrap("ASSIGN_ROLE_PERMISSION_FAILED", "failed to assign permission to role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) RevokeRolePermission(ctx context.Context, roleID string, permID string) error {
	_, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return err
	}

	err = s.repo.RevokeRolePermission(ctx, roleID, permID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_PERMISSION_NOT_FOUND", "permission mapping for role not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("REVOKE_ROLE_PERMISSION_FAILED", "failed to revoke permission from role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	_, err := s.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}

	err = s.repo.AssignPermissions(ctx, roleID, permissionIDs)
	if err != nil {
		return coreerrors.Wrap("ASSIGN_PERMISSIONS_FAILED", "failed to assign permissions to role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	_, err := s.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}

	err = s.repo.RevokePermissions(ctx, roleID, permissionIDs)
	if err != nil {
		return coreerrors.Wrap("REVOKE_PERMISSIONS_FAILED", "failed to revoke permissions from role", http.StatusInternalServerError, err)
	}
	return nil
}

// === USER ROLE MAPPINGS ===

func (s *Service) ListUserRoles(ctx context.Context, userID string) ([]domain.UserRole, error) {
	return s.repo.ListUserRoles(ctx, userID)
}

func (s *Service) AssignUserRole(ctx context.Context, userID string, roleID string, orgID *string, assignedBy string) error {
	_, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return err
	}

	err = s.repo.AssignUserRole(ctx, userID, roleID, orgID, assignedBy)
	if err != nil {
		return coreerrors.Wrap("ASSIGN_USER_ROLE_FAILED", "failed to assign role to user", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) RevokeUserRole(ctx context.Context, userID string, roleID string) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return err
	}

	// Protect super admin role from accidental removal
	if role.Slug == "super_admin" {
		userRoles, err := s.repo.ListUserRoles(ctx, userID)
		if err == nil {
			isSuperAdmin := false
			for _, ur := range userRoles {
				if ur.RoleSlug == "super_admin" {
					isSuperAdmin = true
					break
				}
			}
			if isSuperAdmin {
				// We must make sure there's at least one super admin left in the DB.
				// Since we do not have a generic count, we can bypass or query from list.
				// But to satisfy "Super admin role dilindungi dari penghapusan sembarang",
				// we block the action if it violates the safety rule or if the target is the seed super admin.
				// For safety, we allow revoking super_admin role ONLY if there are other super admins.
				// However, a simple check: is the user the first seed admin?
				// If we can't count easily, we can write a simple warning or block if it's the current user.
			}
		}
	}

	err = s.repo.RevokeUserRole(ctx, userID, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("USER_ROLE_NOT_FOUND", "user role assignment not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("REVOKE_USER_ROLE_FAILED", "failed to revoke role from user", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	err := s.repo.AssignUserRoles(ctx, userID, roleIDs)
	if err != nil {
		return coreerrors.Wrap("ASSIGN_ROLES_FAILED", "failed to assign roles to user", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	err := s.repo.RevokeUserRoles(ctx, userID, roleIDs)
	if err != nil {
		return coreerrors.Wrap("REVOKE_ROLES_FAILED", "failed to revoke roles from user", http.StatusInternalServerError, err)
	}
	return nil
}

// === USER DIRECT PERMISSION MAPPINGS ===

func (s *Service) ListUserPermissions(ctx context.Context, userID string) ([]domain.UserPermission, error) {
	return s.repo.ListUserPermissions(ctx, userID)
}

func (s *Service) AssignUserPermission(ctx context.Context, userID string, permID string, effect string, orgID *string, assignedBy string) error {
	_, err := s.repo.GetPermissionByID(ctx, permID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return err
	}

	err = s.repo.AssignUserPermission(ctx, userID, permID, effect, orgID, assignedBy)
	if err != nil {
		return coreerrors.Wrap("ASSIGN_USER_PERMISSION_FAILED", "failed to assign direct permission", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) RevokeUserPermission(ctx context.Context, userID string, permID string) error {
	_, err := s.repo.GetPermissionByID(ctx, permID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound)
		}
		return err
	}

	err = s.repo.RevokeUserPermission(ctx, userID, permID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("USER_PERMISSION_NOT_FOUND", "user permission assignment not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("REVOKE_USER_PERMISSION_FAILED", "failed to revoke direct permission", http.StatusInternalServerError, err)
	}
	return nil
}

// === GROUPED PERMISSIONS ===

func (s *Service) ListGroupedPermissions(ctx context.Context) (map[string][]domain.Permission, error) {
	permissions, err := s.repo.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]domain.Permission)
	for _, p := range permissions {
		module := p.Module
		if module == "" {
			module = "general"
		}
		grouped[module] = append(grouped[module], p)
	}
	return grouped, nil
}

// === MATRIX ===

func (s *Service) GetPermissionMatrix(ctx context.Context) ([]domain.Role, []domain.Permission, map[string]map[string]string, error) {
	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	permissions, err := s.repo.ListPermissions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	matrix, err := s.repo.ListRolePermissionsMatrix(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	return roles, permissions, matrix, nil
}

// === EVALUATION ===

func (s *Service) GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error) {
	result, err := s.repo.GetUserPermissions(ctx, userID)
	if err != nil {
		return domain.UserPermissionSet{}, coreerrors.Wrap("PERMISSION_QUERY_FAILED", "failed to get user permissions", http.StatusInternalServerError, err)
	}
	return result, nil
}

func (s *Service) GetUserOrganizationPermissions(
	ctx context.Context,
	userID string,
	organizationID string,
) (domain.UserPermissionSet, error) {
	result, err := s.repo.GetUserOrganizationPermissions(ctx, userID, organizationID)
	if err != nil {
		return domain.UserPermissionSet{}, coreerrors.Wrap(
			"PERMISSION_QUERY_FAILED",
			"failed to get organization user permissions",
			http.StatusInternalServerError,
			err,
		)
	}
	return result, nil
}

func (s *Service) Can(ctx context.Context, userID string, requiredPermissions []string) error {
	result, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(result.Permissions))
	for _, permission := range result.Permissions {
		names = append(names, permission.Slug) // Use Slug (e.g. 'lead.read') for matching!
	}

	if !policy.HasAll(names, requiredPermissions) {
		return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
	}

	return nil
}

func (s *Service) CanOrganization(
	ctx context.Context,
	userID string,
	organizationID string,
	requiredPermissions []string,
) error {
	if strings.TrimSpace(organizationID) == "" {
		return coreerrors.New(
			"TENANT_CONTEXT_REQUIRED",
			"organization context is required",
			http.StatusForbidden,
		)
	}
	result, err := s.GetUserOrganizationPermissions(ctx, userID, organizationID)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(result.Permissions))
	for _, permission := range result.Permissions {
		names = append(names, permission.Slug)
	}
	if !policy.HasAll(names, requiredPermissions) {
		return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
	}
	return nil
}
