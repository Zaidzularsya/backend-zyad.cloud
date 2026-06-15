package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/core/permission/service"
)

type mockRepo struct {
	permissions     map[string]domain.Permission
	roles           map[string]domain.Role
	userRoles       map[string][]domain.UserRole
	userPermissions map[string][]domain.UserPermission
	rolePermissions map[string]map[string]string // role_id -> perm_id -> scope
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		permissions:     make(map[string]domain.Permission),
		roles:           make(map[string]domain.Role),
		userRoles:       make(map[string][]domain.UserRole),
		userPermissions: make(map[string][]domain.UserPermission),
		rolePermissions: make(map[string]map[string]string),
	}
}

func (m *mockRepo) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	list := make([]domain.Permission, 0, len(m.permissions))
	for _, p := range m.permissions {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockRepo) GetPermissionByID(ctx context.Context, id string) (domain.Permission, error) {
	p, ok := m.permissions[id]
	if !ok {
		return domain.Permission{}, sqlErrNoRows()
	}
	return p, nil
}

func (m *mockRepo) CreatePermission(ctx context.Context, p *domain.Permission) error {
	p.ID = "perm-" + p.Slug
	now := time.Now()
	p.CreatedAt = &now
	p.UpdatedAt = &now
	m.permissions[p.ID] = *p
	return nil
}

func (m *mockRepo) UpdatePermission(ctx context.Context, id string, p *domain.Permission) error {
	return nil
}

func (m *mockRepo) DeletePermission(ctx context.Context, id string) error {
	delete(m.permissions, id)
	return nil
}

func (m *mockRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	list := make([]domain.Role, 0, len(m.roles))
	for _, r := range m.roles {
		list = append(list, r)
	}
	return list, nil
}

func (m *mockRepo) GetRoleByID(ctx context.Context, id string) (domain.Role, error) {
	r, ok := m.roles[id]
	if !ok {
		return domain.Role{}, sqlErrNoRows()
	}
	return r, nil
}

func (m *mockRepo) GetRolePermissions(ctx context.Context, roleName string) (domain.Role, error) {
	for _, r := range m.roles {
		if r.Name == roleName || r.Slug == roleName {
			return r, nil
		}
	}
	return domain.Role{}, sqlErrNoRows()
}

func (m *mockRepo) CreateRole(ctx context.Context, role *domain.Role) error {
	role.ID = "role-" + role.Slug
	now := time.Now()
	role.CreatedAt = &now
	role.UpdatedAt = &now
	m.roles[role.ID] = *role
	return nil
}

func (m *mockRepo) UpdateRole(ctx context.Context, id string, role *domain.Role) error {
	return nil
}

func (m *mockRepo) DeleteRole(ctx context.Context, id string) error {
	delete(m.roles, id)
	return nil
}

func (m *mockRepo) GetRolePermissionsByID(ctx context.Context, roleID string) ([]domain.Permission, error) {
	return nil, nil
}

func (m *mockRepo) AssignRolePermission(ctx context.Context, roleID string, permID string, scope string) error {
	if _, ok := m.rolePermissions[roleID]; !ok {
		m.rolePermissions[roleID] = make(map[string]string)
	}
	m.rolePermissions[roleID][permID] = scope
	return nil
}

func (m *mockRepo) RevokeRolePermission(ctx context.Context, roleID string, permID string) error {
	if _, ok := m.rolePermissions[roleID]; ok {
		delete(m.rolePermissions[roleID], permID)
	}
	return nil
}

func (m *mockRepo) AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return nil
}

func (m *mockRepo) RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return nil
}

func (m *mockRepo) ListUserRoles(ctx context.Context, userID string) ([]domain.UserRole, error) {
	return m.userRoles[userID], nil
}

func (m *mockRepo) AssignUserRole(ctx context.Context, userID string, roleID string, orgID *string, assignedBy string) error {
	r := m.roles[roleID]
	ur := domain.UserRole{
		ID:             "ur-" + userID + "-" + roleID,
		UserID:         userID,
		RoleID:         roleID,
		RoleSlug:       r.Slug,
		OrganizationID: orgID,
		AssignedAt:     time.Now(),
	}
	m.userRoles[userID] = append(m.userRoles[userID], ur)
	return nil
}

func (m *mockRepo) AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	return nil
}

func (m *mockRepo) RevokeUserRole(ctx context.Context, userID string, roleID string) error {
	list := m.userRoles[userID]
	for i, ur := range list {
		if ur.RoleID == roleID {
			m.userRoles[userID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return sqlErrNoRows()
}

func (m *mockRepo) RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	return nil
}

func (m *mockRepo) ListUserPermissions(ctx context.Context, userID string) ([]domain.UserPermission, error) {
	return m.userPermissions[userID], nil
}

func (m *mockRepo) AssignUserPermission(ctx context.Context, userID string, permID string, effect string, orgID *string, assignedBy string) error {
	p := m.permissions[permID]
	up := domain.UserPermission{
		ID:             "up-" + userID + "-" + permID,
		UserID:         userID,
		PermissionID:   permID,
		PermissionSlug: p.Slug,
		OrganizationID: orgID,
		Effect:         effect,
		AssignedAt:     time.Now(),
	}
	m.userPermissions[userID] = append(m.userPermissions[userID], up)
	return nil
}

func (m *mockRepo) RevokeUserPermission(ctx context.Context, userID string, permID string) error {
	list := m.userPermissions[userID]
	for i, up := range list {
		if up.PermissionID == permID {
			m.userPermissions[userID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return sqlErrNoRows()
}

func (m *mockRepo) ListRolePermissionsMatrix(ctx context.Context) (map[string]map[string]string, error) {
	return m.rolePermissions, nil
}

func (m *mockRepo) GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error) {
	return m.getUserPermissions(userID, nil)
}

func (m *mockRepo) GetUserOrganizationPermissions(
	_ context.Context,
	userID string,
	organizationID string,
) (domain.UserPermissionSet, error) {
	return m.getUserPermissions(userID, &organizationID)
}

func (m *mockRepo) getUserPermissions(
	userID string,
	organizationID *string,
) (domain.UserPermissionSet, error) {
	roleNamesMap := make(map[string]struct{})
	roleNames := make([]string, 0)
	effectiveMap := make(map[string]domain.Permission)

	// Roles
	for _, ur := range m.userRoles[userID] {
		if !sameOrganizationScope(ur.OrganizationID, organizationID) {
			continue
		}
		r := m.roles[ur.RoleID]
		if _, ok := roleNamesMap[r.Name]; !ok {
			roleNamesMap[r.Name] = struct{}{}
			roleNames = append(roleNames, r.Name)
		}

		// Role mappings
		if permsMap, ok := m.rolePermissions[ur.RoleID]; ok {
			for permID := range permsMap {
				p := m.permissions[permID]
				effectiveMap[p.Slug] = p
			}
		}
	}

	// Direct Overrides
	for _, up := range m.userPermissions[userID] {
		if !sameOrganizationScope(up.OrganizationID, organizationID) {
			continue
		}
		p := m.permissions[up.PermissionID]
		if up.Effect == "deny" {
			delete(effectiveMap, p.Slug)
		} else if up.Effect == "allow" {
			effectiveMap[p.Slug] = p
		}
	}

	permsList := make([]domain.Permission, 0, len(effectiveMap))
	for _, p := range effectiveMap {
		permsList = append(permsList, p)
	}

	return domain.UserPermissionSet{
		UserID:      userID,
		RoleNames:   roleNames,
		Permissions: permsList,
	}, nil
}

func sameOrganizationScope(actual *string, expected *string) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return *actual == *expected
}

func sqlErrNoRows() error {
	// Simple mock helper
	return errors.New("no rows in result set")
}

// === TEST CASES ===

func TestCan_RolePermission(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()

	// Seed permission
	p1 := domain.Permission{Name: "Lead Read", Slug: "lead.read"}
	_ = repo.CreatePermission(ctx, &p1)

	// Seed role
	r1 := domain.Role{Name: "Sales", Slug: "sales"}
	_ = repo.CreateRole(ctx, &r1)

	// Map permission to role
	_ = repo.AssignRolePermission(ctx, r1.ID, p1.ID, "organization")

	// Assign role to user
	_ = repo.AssignUserRole(ctx, "user-1", r1.ID, nil, "admin-1")

	// Check if user has permission
	err := svc.Can(ctx, "user-1", []string{"lead.read"})
	if err != nil {
		t.Fatalf("expected user-1 to have 'lead.read' permission, got error: %v", err)
	}

	// Check non-existent permission
	err = svc.Can(ctx, "user-1", []string{"customer.create"})
	if err == nil {
		t.Fatal("expected user-1 to lack 'customer.create' permission")
	}
}

func TestCan_DirectDenyOverride(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()

	// Seed permission
	p1 := domain.Permission{Name: "Lead Read", Slug: "lead.read"}
	_ = repo.CreatePermission(ctx, &p1)

	// Seed role
	r1 := domain.Role{Name: "Sales", Slug: "sales"}
	_ = repo.CreateRole(ctx, &r1)

	// Map permission to role
	_ = repo.AssignRolePermission(ctx, r1.ID, p1.ID, "organization")

	// Assign role to user
	_ = repo.AssignUserRole(ctx, "user-1", r1.ID, nil, "admin-1")

	// Override direct permission as 'deny'
	_ = repo.AssignUserPermission(ctx, "user-1", p1.ID, "deny", nil, "admin-1")

	// Check if user has permission (should be forbidden!)
	err := svc.Can(ctx, "user-1", []string{"lead.read"})
	if err == nil {
		t.Fatal("expected user-1 to lack 'lead.read' due to direct 'deny' override")
	}
}

func TestCan_DirectAllowOverride(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()

	// Seed permission
	p1 := domain.Permission{Name: "Customer Create", Slug: "customer.create"}
	_ = repo.CreatePermission(ctx, &p1)

	// Override direct permission as 'allow' without any role
	_ = repo.AssignUserPermission(ctx, "user-1", p1.ID, "allow", nil, "admin-1")

	// Check if user has permission (should be allowed!)
	err := svc.Can(ctx, "user-1", []string{"customer.create"})
	if err != nil {
		t.Fatalf("expected user-1 to have 'customer.create' due to direct 'allow' override, got error: %v", err)
	}
}

func TestCanOrganizationIsolatesAssignmentsByOrganization(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()
	organizationA := "11111111-1111-1111-1111-111111111111"
	organizationB := "22222222-2222-2222-2222-222222222222"

	permission := domain.Permission{Name: "Member Read", Slug: "organization.member.read"}
	_ = repo.CreatePermission(ctx, &permission)
	role := domain.Role{Name: "Organization Admin", Slug: "organization_admin"}
	_ = repo.CreateRole(ctx, &role)
	_ = repo.AssignRolePermission(ctx, role.ID, permission.ID, "organization")
	_ = repo.AssignUserRole(ctx, "user-1", role.ID, &organizationA, "admin-1")

	if err := svc.CanOrganization(
		ctx,
		"user-1",
		organizationA,
		[]string{"organization.member.read"},
	); err != nil {
		t.Fatalf("organization A permission error = %v", err)
	}
	if err := svc.CanOrganization(
		ctx,
		"user-1",
		organizationB,
		[]string{"organization.member.read"},
	); err == nil {
		t.Fatal("organization A permission authorized organization B")
	}
	if err := svc.Can(ctx, "user-1", []string{"organization.member.read"}); err == nil {
		t.Fatal("organization permission leaked into global evaluation")
	}
}

func TestCanOrganizationDirectDenyOnlyAppliesToSameOrganization(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()
	organizationA := "11111111-1111-1111-1111-111111111111"
	organizationB := "22222222-2222-2222-2222-222222222222"

	permission := domain.Permission{Name: "Member Read", Slug: "organization.member.read"}
	_ = repo.CreatePermission(ctx, &permission)
	role := domain.Role{Name: "Organization Admin", Slug: "organization_admin"}
	_ = repo.CreateRole(ctx, &role)
	_ = repo.AssignRolePermission(ctx, role.ID, permission.ID, "organization")
	_ = repo.AssignUserRole(ctx, "user-1", role.ID, &organizationA, "admin-1")
	_ = repo.AssignUserRole(ctx, "user-1", role.ID, &organizationB, "admin-1")
	_ = repo.AssignUserPermission(
		ctx,
		"user-1",
		permission.ID,
		"deny",
		&organizationA,
		"admin-1",
	)

	if err := svc.CanOrganization(
		ctx,
		"user-1",
		organizationA,
		[]string{"organization.member.read"},
	); err == nil {
		t.Fatal("organization A direct deny did not override role permission")
	}
	if err := svc.CanOrganization(
		ctx,
		"user-1",
		organizationB,
		[]string{"organization.member.read"},
	); err != nil {
		t.Fatalf("organization A deny affected organization B: %v", err)
	}
}

func TestDeleteRole_SystemRoleProtection(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo)
	ctx := context.Background()

	// Seed system role
	r1 := domain.Role{Name: "Super Admin", Slug: "super_admin", IsSystem: true}
	_ = repo.CreateRole(ctx, &r1)

	// Try deleting system role
	err := svc.DeleteRole(ctx, r1.ID)
	if err == nil {
		t.Fatal("expected error when deleting a system role")
	}

	if !strings.Contains(err.Error(), "system role") {
		t.Fatalf("expected system role protection error, got: %v", err)
	}
}
