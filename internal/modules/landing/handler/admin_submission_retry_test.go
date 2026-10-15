package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/service"
)

// retryRouter mounts only the retry route; the permission middleware is
// covered by RegisterRoutes wiring and is not under test here.
func retryRouter(t *testing.T, svc service.SubmissionService, orgID string, orgType coretenant.OrganizationType) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(tenantMiddleware(t, orgID, orgType))
	router.POST("/admin/landing-submissions/:id/crm-sync", NewAdminSubmissionHandler(svc).RetryCRMSync)
	return router
}

func TestAdminRetry_OtherTenant404(t *testing.T) {
	// The service (scoped repo) reports "no rows" for another tenant's id.
	svc := &submissionServiceStub{retryErr: pgx.ErrNoRows}
	router := retryRouter(t, svc, "22222222-2222-2222-2222-222222222222", coretenant.OrganizationTypeCustomer)

	req := httptest.NewRequest(http.MethodPost, "/admin/landing-submissions/sub-of-tenant-a/crm-sync", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "SUBMISSION_NOT_FOUND") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.retryScope.OrganizationID() != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("retry scope org = %q", svc.retryScope.OrganizationID())
	}
}

func TestAdminRetry_ReturnsSubmissionAndPlatformFlag(t *testing.T) {
	svc := &submissionServiceStub{retryRes: domain.LandingSubmission{ID: "sub-1", CRMLeadID: "lead-1", CRMSyncStatus: domain.CRMSyncCreated}}
	router := retryRouter(t, svc, "11111111-1111-1111-1111-111111111111", coretenant.OrganizationTypePlatform)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/landing-submissions/sub-1/crm-sync", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data domain.LandingSubmission `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.CRMLeadID != "lead-1" || body.Data.CRMSyncStatus != domain.CRMSyncCreated {
		t.Fatalf("data = %+v", body.Data)
	}
	if svc.retryID != "sub-1" || !svc.retryPlat {
		t.Fatalf("retryID=%q platform=%v", svc.retryID, svc.retryPlat)
	}
}
