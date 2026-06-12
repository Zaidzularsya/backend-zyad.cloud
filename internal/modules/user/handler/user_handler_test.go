package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"

	"github.com/gin-gonic/gin"
)

func TestUserHandlerListUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		users: []dto.UserListItem{{ID: "user-1", Email: "jane@example.test"}},
		meta:  dto.PaginationMeta{Page: 2, PerPage: 10, Total: 11, TotalPages: 2},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("admin-1"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?page=2&per_page=10&search=jane", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.query.Page != 2 || service.query.PerPage != 10 || service.query.Search != "jane" {
		t.Fatalf("query = %#v", service.query)
	}
	if len(checker.permissions) == 0 || checker.permissions[0] != "user.read" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerListUsersRequiresRestorePermissionForDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{}
	checker := &fakePermissionChecker{deniedPermission: "user.restore"}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("admin-1"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?include_deleted=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.called {
		t.Fatal("service should not be called")
	}
}

func TestUserHandlerGetUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		detail: dto.UserDetailResponse{
			ID:    "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
			Email: "jane@example.test",
		},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("admin-1"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.userID != "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b" {
		t.Fatalf("userID = %q", service.userID)
	}
}

func TestUserHandlerGetDeletedUserRequiresRestorePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{}
	checker := &fakePermissionChecker{deniedPermission: "user.restore"}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("admin-1"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b?include_deleted=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.getCalled {
		t.Fatal("service should not be called")
	}
}

func TestUserHandlerCreateUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		created: dto.UserDetailResponse{
			ID:    "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
			Email: "jane@example.test",
		},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewBufferString(`{
		"name":"Jane Doe",
		"email":"jane@example.test",
		"password":"temporary-secret"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.createCalled || service.createRequest.Email != "jane@example.test" {
		t.Fatalf("request = %#v", service.createRequest)
	}
	if service.createMetadata.ActorUserID == "" {
		t.Fatalf("metadata = %#v", service.createMetadata)
	}
	if len(checker.permissions) == 0 || checker.permissions[0] != "user.create" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerUpdateUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		updated: dto.UserDetailResponse{
			ID:    "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
			Email: "jane@example.test",
		},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPatch, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", bytes.NewBufferString(`{
		"name":"Jane Updated",
		"job_title":"Senior Editor"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.updateCalled || service.updateUserID == "" {
		t.Fatalf("userID = %q request = %#v", service.updateUserID, service.updateRequest)
	}
	if service.updateMetadata.ActorUserID == "" {
		t.Fatalf("metadata = %#v", service.updateMetadata)
	}
	if len(checker.permissions) == 0 || checker.permissions[0] != "user.update" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerDeleteUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.deleteCalled || service.deleteMetadata.ActorUserID == "" {
		t.Fatalf("deleteCalled=%v metadata=%#v", service.deleteCalled, service.deleteMetadata)
	}
	if checker.permissions[0] != "user.delete" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerRestoreUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		restored: dto.UserDetailResponse{ID: "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b/restore", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.restoreCalled || service.restoreMetadata.ActorUserID == "" {
		t.Fatalf("restoreCalled=%v metadata=%#v", service.restoreCalled, service.restoreMetadata)
	}
	if checker.permissions[0] != "user.restore" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerBulkActionUsesActionPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		bulkResult: dto.BulkUserActionResponse{Action: "update_status", Total: 1, Succeeded: 1},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/bulk-action", bytes.NewBufferString(`{
		"action":"update_status",
		"user_ids":["7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"],
		"payload":{"status":"inactive"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.bulkCalled || checker.permissions[0] != "user.update_status" {
		t.Fatalf("bulkCalled=%v permissions=%#v", service.bulkCalled, checker.permissions)
	}
}

func TestUserHandlerSuspendUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{
		statusUpdated: dto.UserDetailResponse{
			ID:     "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
			Status: "suspended",
		},
	}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b/suspend", bytes.NewBufferString(`{
		"reason":"Security review"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !service.statusCalled || service.statusRequest.Status != "suspended" || service.statusRequest.Reason != "Security review" {
		t.Fatalf("request = %#v", service.statusRequest)
	}
	if checker.permissions[0] != "user.update_status" {
		t.Fatalf("permissions = %#v", checker.permissions)
	}
}

func TestUserHandlerActivateUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeUserListService{statusUpdated: dto.UserDetailResponse{Status: "active"}}
	checker := &fakePermissionChecker{}
	handler := NewUserHandler(service, checker)
	router := gin.New()
	group := router.Group("")
	group.Use(seedUserContext("a73d7c06-96e9-4ab2-bb32-e025dde5660c"))
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/7ea7b1cb-04eb-456a-a19f-e41d590d2a3b/activate", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || service.statusRequest.Status != "active" {
		t.Fatalf("status=%d request=%#v body=%s", rec.Code, service.statusRequest, rec.Body.String())
	}
}

type fakeUserListService struct {
	called          bool
	query           dto.UserListQuery
	users           []dto.UserListItem
	meta            dto.PaginationMeta
	err             error
	getCalled       bool
	userID          string
	includeDeleted  bool
	detail          dto.UserDetailResponse
	detailErr       error
	createCalled    bool
	createRequest   dto.CreateUserRequest
	createMetadata  service.CreateUserMetadata
	created         dto.UserDetailResponse
	createErr       error
	updateCalled    bool
	updateUserID    string
	updateRequest   dto.UpdateUserRequest
	updateMetadata  service.UpdateUserMetadata
	updated         dto.UserDetailResponse
	updateErr       error
	deleteCalled    bool
	deleteUserID    string
	deleteMetadata  service.UserLifecycleMetadata
	deleteErr       error
	restoreCalled   bool
	restoreUserID   string
	restoreMetadata service.UserLifecycleMetadata
	restored        dto.UserDetailResponse
	restoreErr      error
	bulkCalled      bool
	bulkRequest     dto.BulkUserActionRequest
	bulkMetadata    service.UserLifecycleMetadata
	bulkResult      dto.BulkUserActionResponse
	bulkErr         error
	statusCalled    bool
	statusUserID    string
	statusRequest   dto.UpdateUserStatusRequest
	statusMetadata  service.UserLifecycleMetadata
	statusUpdated   dto.UserDetailResponse
	statusErr       error
}

func (s *fakeUserListService) ChangeUserStatus(_ context.Context, userID string, req dto.UpdateUserStatusRequest, metadata service.UserLifecycleMetadata) (dto.UserDetailResponse, error) {
	s.statusCalled = true
	s.statusUserID = userID
	s.statusRequest = req
	s.statusMetadata = metadata
	return s.statusUpdated, s.statusErr
}

func (s *fakeUserListService) BulkAction(_ context.Context, req dto.BulkUserActionRequest, metadata service.UserLifecycleMetadata) (dto.BulkUserActionResponse, error) {
	s.bulkCalled = true
	s.bulkRequest = req
	s.bulkMetadata = metadata
	return s.bulkResult, s.bulkErr
}

func (s *fakeUserListService) DeleteUser(_ context.Context, userID string, metadata service.UserLifecycleMetadata) error {
	s.deleteCalled = true
	s.deleteUserID = userID
	s.deleteMetadata = metadata
	return s.deleteErr
}

func (s *fakeUserListService) RestoreUser(_ context.Context, userID string, metadata service.UserLifecycleMetadata) (dto.UserDetailResponse, error) {
	s.restoreCalled = true
	s.restoreUserID = userID
	s.restoreMetadata = metadata
	return s.restored, s.restoreErr
}

func (s *fakeUserListService) UpdateUser(_ context.Context, userID string, req dto.UpdateUserRequest, metadata service.UpdateUserMetadata) (dto.UserDetailResponse, error) {
	s.updateCalled = true
	s.updateUserID = userID
	s.updateRequest = req
	s.updateMetadata = metadata
	return s.updated, s.updateErr
}

func (s *fakeUserListService) CreateUser(_ context.Context, req dto.CreateUserRequest, metadata service.CreateUserMetadata) (dto.UserDetailResponse, error) {
	s.createCalled = true
	s.createRequest = req
	s.createMetadata = metadata
	return s.created, s.createErr
}

func (s *fakeUserListService) ListUsers(_ context.Context, query dto.UserListQuery) ([]dto.UserListItem, dto.PaginationMeta, error) {
	s.called = true
	s.query = query
	return s.users, s.meta, s.err
}

func (s *fakeUserListService) GetUser(_ context.Context, userID string, includeDeleted bool) (dto.UserDetailResponse, error) {
	s.getCalled = true
	s.userID = userID
	s.includeDeleted = includeDeleted
	return s.detail, s.detailErr
}

type fakePermissionChecker struct {
	permissions      []string
	deniedPermission string
}

func (c *fakePermissionChecker) Can(_ context.Context, _ string, permissions []string) error {
	c.permissions = append(c.permissions, permissions...)
	for _, permission := range permissions {
		if permission == c.deniedPermission {
			return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
		}
	}
	return nil
}

func seedUserContext(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissionmiddleware.SetUserID(c, userID)
		c.Next()
	}
}
