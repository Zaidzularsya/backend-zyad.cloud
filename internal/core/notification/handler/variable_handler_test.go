package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"

	"github.com/gin-gonic/gin"
)

func TestVariableHandlerListVariables(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := &fakePermissionChecker{}
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(seedPermissionUser("user-1"))
	NewVariableHandler(nil, checker).RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notification-template-variables", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if checker.userID != "user-1" {
		t.Fatalf("checker userID = %q, want user-1", checker.userID)
	}
	if len(checker.permissions) != 1 || checker.permissions[0] != "notification_variable.read" {
		t.Fatalf("checker permissions = %#v", checker.permissions)
	}
	if !strings.Contains(rec.Body.String(), "auth.password_reset") {
		t.Fatalf("response body does not contain auth.password_reset: %s", rec.Body.String())
	}
}

func TestVariableHandlerGetVariables(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(seedPermissionUser("user-1"))
	NewVariableHandler(nil, &fakePermissionChecker{}).RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notification-template-variables/auth.password_reset", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "reset_url") {
		t.Fatalf("response body does not contain reset_url: %s", rec.Body.String())
	}
}

func TestVariableHandlerGetVariablesNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(seedPermissionUser("user-1"))
	NewVariableHandler(nil, &fakePermissionChecker{}).RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notification-template-variables/unknown.template", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

type fakePermissionChecker struct {
	userID      string
	permissions []string
}

func (c *fakePermissionChecker) Can(_ context.Context, userID string, requiredPermissions []string) error {
	c.userID = userID
	c.permissions = requiredPermissions
	return nil
}

func seedPermissionUser(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissionmiddleware.SetUserID(c, userID)
		c.Next()
	}
}
