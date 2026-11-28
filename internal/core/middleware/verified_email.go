package middleware

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"

	"github.com/gin-gonic/gin"
)

// RequireVerifiedEmail rejects accounts whose email is not verified yet. Such
// accounts may sign in and read, but endpoints that act on the email (for
// example claiming a CRM contact by email) need proof of ownership first.
// It must run after Authenticate.
func RequireVerifiedEmail() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := AuthenticatedUserFromContext(c)
		if !ok || user.ID == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
			c.Abort()
			return
		}
		if !user.EmailVerified {
			corehttp.Fail(c, coreerrors.New("EMAIL_NOT_VERIFIED", "Verifikasi email Anda terlebih dahulu.", http.StatusForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}
