package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
)

func TestCompanyLookupRouteRequiresPermissionAndDoesNotClashWithID(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewCompanyHandler(nil).RegisterRoutes(r.Group(""), denyAllChecker{})

	w := performJSON(r, http.MethodGet, "/companies/lookup?name=maju", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("GET /companies/lookup without permission → %d, want 403", w.Code)
	}
}
