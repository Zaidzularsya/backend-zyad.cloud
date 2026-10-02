package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
)

type allowAllChecker struct{}

func (allowAllChecker) Can(context.Context, string, []string) error { return nil }
func (allowAllChecker) CanOrganization(context.Context, string, string, []string) error {
	return nil
}

func TestQuotationV2RoutesRequirePermission(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewQuotationHandler(nil).RegisterRoutes(r.Group(""), denyAllChecker{})

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/quotations/x/revise"},
		{http.MethodGet, "/quotations/x/pdf"},
	} {
		if w := performJSON(r, tc.method, tc.path, ""); w.Code != http.StatusForbidden {
			t.Errorf("%s %s → %d, want 403", tc.method, tc.path, w.Code)
		}
	}
}

func TestQuotationSendRejectsUnknownChannel(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewQuotationHandler(nil).RegisterRoutes(r.Group(""), allowAllChecker{})

	if w := performJSON(r, http.MethodPost, "/quotations/x/send", `{"channel":"fax"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("send fax → %d, want 422", w.Code)
	}
}
