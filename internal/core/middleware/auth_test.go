package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"

	"github.com/gin-gonic/gin"
)

func TestAuthenticateSetsUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", Authenticate(fakeAuthenticator{
		user: AuthenticatedUser{ID: "user-1", SessionID: "session-1"},
	}), func(c *gin.Context) {
		if got := permissionmiddleware.UserID(c); got != "user-1" {
			t.Fatalf("permission user id = %q, want user-1", got)
		}
		if user, ok := AuthenticatedUserFromContext(c); !ok || user.ID != "user-1" {
			t.Fatalf("authenticated user = %#v, %v", user, ok)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestAuthenticateRejectsMissingBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", Authenticate(fakeAuthenticator{}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticateReturnsAuthenticatorError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", Authenticate(fakeAuthenticator{
		err: coreerrors.New("FORBIDDEN", "forbidden", http.StatusForbidden),
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireRoleAllowsUserWithRequiredRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", seedAuthenticatedUser(AuthenticatedUser{
		ID:    "user-1",
		Roles: []string{"super_admin", "manager"},
	}), RequireRole("super_admin"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireRoleRejectsUserWithoutRequiredRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", seedAuthenticatedUser(AuthenticatedUser{
		ID:    "user-1",
		Roles: []string{"manager"},
	}), RequireRole("super_admin"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAnyRoleAllowsOneMatchingRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", seedAuthenticatedUser(AuthenticatedUser{
		ID:    "user-1",
		Roles: []string{"manager"},
	}), RequireAnyRole("super_admin", "manager"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

type fakeAuthenticator struct {
	user AuthenticatedUser
	err  error
}

func (a fakeAuthenticator) AuthenticateAccessToken(context.Context, string) (AuthenticatedUser, error) {
	if a.err != nil {
		return AuthenticatedUser{}, a.err
	}
	return a.user, nil
}

func seedAuthenticatedUser(user AuthenticatedUser) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AuthenticatedUserContextKey, user)
		c.Next()
	}
}
