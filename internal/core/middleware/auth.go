package middleware

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"

	"github.com/gin-gonic/gin"
)

type AuthenticatedUser struct {
	ID          string
	SessionID   string
	Status      string
	Roles       []string
	Permissions []string
}

type AccessTokenAuthenticator interface {
	AuthenticateAccessToken(ctx context.Context, accessToken string) (AuthenticatedUser, error)
}

const AuthenticatedUserContextKey = "authenticated_user"

func Authenticate(authenticator AccessTokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator == nil {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticator is required", http.StatusUnauthorized))
			c.Abort()
			return
		}

		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authorization bearer token is required", http.StatusUnauthorized))
			c.Abort()
			return
		}

		user, err := authenticator.AuthenticateAccessToken(c.Request.Context(), token)
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		if user.ID == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
			c.Abort()
			return
		}

		c.Set(AuthenticatedUserContextKey, user)
		permissionmiddleware.SetUserID(c, user.ID)
		c.Next()
	}
}

func AuthenticatedUserFromContext(c *gin.Context) (AuthenticatedUser, bool) {
	value, ok := c.Get(AuthenticatedUserContextKey)
	if !ok {
		return AuthenticatedUser{}, false
	}
	user, ok := value.(AuthenticatedUser)
	return user, ok
}

func RequireRole(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := AuthenticatedUserFromContext(c)
		if !ok || user.ID == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
			c.Abort()
			return
		}
		if !hasAllRoles(user.Roles, requiredRoles) {
			corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient role", http.StatusForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireAnyRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := AuthenticatedUserFromContext(c)
		if !ok || user.ID == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
			c.Abort()
			return
		}
		if !hasAnyRole(user.Roles, allowedRoles) {
			corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient role", http.StatusForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}

func hasAllRoles(userRoles []string, requiredRoles []string) bool {
	if len(requiredRoles) == 0 {
		return true
	}
	roles := roleSet(userRoles)
	for _, role := range requiredRoles {
		if !roles[role] {
			return false
		}
	}
	return true
}

func hasAnyRole(userRoles []string, allowedRoles []string) bool {
	if len(allowedRoles) == 0 {
		return true
	}
	roles := roleSet(userRoles)
	for _, role := range allowedRoles {
		if roles[role] {
			return true
		}
	}
	return false
}

func roleSet(roles []string) map[string]bool {
	set := make(map[string]bool, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		set[role] = true
	}
	return set
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
