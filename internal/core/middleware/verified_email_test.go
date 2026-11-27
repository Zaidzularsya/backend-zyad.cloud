package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireVerifiedEmailBlocksUnverified(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		user     *AuthenticatedUser
		wantCode int
		wantBody string
	}{
		{name: "unverified", user: &AuthenticatedUser{ID: "u-1", Email: "a@x.test"}, wantCode: http.StatusForbidden, wantBody: "EMAIL_NOT_VERIFIED"},
		{name: "verified", user: &AuthenticatedUser{ID: "u-1", Email: "a@x.test", EmailVerified: true}, wantCode: http.StatusOK},
		{name: "anonymous", user: nil, wantCode: http.StatusUnauthorized, wantBody: "UNAUTHORIZED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/claim",
				func(c *gin.Context) {
					if tc.user != nil {
						c.Set(AuthenticatedUserContextKey, *tc.user)
					}
					c.Next()
				},
				RequireVerifiedEmail(),
				func(c *gin.Context) { c.Status(http.StatusOK) },
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/claim", nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body = %s, want %s", rec.Body.String(), tc.wantBody)
			}
		})
	}
}
