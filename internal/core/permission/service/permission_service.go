package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/core/permission/policy"
	"github.com/jackc/pgx/v5"
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
	AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error
	RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error
	AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error
	RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error
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
	if p.Name == "" {
		return coreerrors.New("INVALID_INPUT", "permission name cannot be empty", http.StatusBadRequest)
	}

	err := s.repo.CreatePermission(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505") {
			return coreerrors.New("PERMISSION_ALREADY_EXISTS", "permission name already exists", http.StatusConflict)
		}
		return coreerrors.Wrap("PERMISSION_CREATE_FAILED", "failed to create permission", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) UpdatePermission(ctx context.Context, id string, p *domain.Permission) error {
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
	if role.ID == "" {
		return domain.Role{}, coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
	}
	return role, nil
}

func (s *Service) CreateRole(ctx context.Context, role *domain.Role) error {
	role.Name = strings.TrimSpace(role.Name)
	if role.Name == "" {
		return coreerrors.New("INVALID_INPUT", "role name cannot be empty", http.StatusBadRequest)
	}

	err := s.repo.CreateRole(ctx, role)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505") {
			return coreerrors.New("ROLE_ALREADY_EXISTS", "role name already exists", http.StatusConflict)
		}
		return coreerrors.Wrap("ROLE_CREATE_FAILED", "failed to create role", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *Service) UpdateRole(ctx context.Context, id string, role *domain.Role) error {
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
	err := s.repo.DeleteRole(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("ROLE_NOT_FOUND", "role not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("ROLE_DELETE_FAILED", "failed to delete role", http.StatusInternalServerError, err)
	}
	return nil
}

// === MAPPINGS / RELATIONSHIPS ===

func (s *Service) GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error) {
	result, err := s.repo.GetUserPermissions(ctx, userID)
	if err != nil {
		return domain.UserPermissionSet{}, coreerrors.Wrap("PERMISSION_QUERY_FAILED", "failed to get user permissions", http.StatusInternalServerError, err)
	}
	return result, nil
}

func (s *Service) AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	// Verifikasi role ada
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

// === EVALUATION ===

func (s *Service) Can(ctx context.Context, userID string, requiredPermissions []string) error {
	result, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(result.Permissions))
	for _, permission := range result.Permissions {
		names = append(names, permission.Name)
	}

	if !policy.HasAll(names, requiredPermissions) {
		return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
	}

	return nil
}

