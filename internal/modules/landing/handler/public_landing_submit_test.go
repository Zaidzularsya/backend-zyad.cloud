package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"

	coremiddleware "zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

// submissionServiceStub records calls; embedding the interface makes any
// unexpected method call panic.
type submissionServiceStub struct {
	service.SubmissionService

	submitCalls  int
	submitParams repository.CreateSubmissionParams
	submitPlat   bool
	submitErr    error

	retryCalls int
	retryID    string
	retryScope coretenant.Scope
	retryPlat  bool
	retryErr   error
	retryRes   domain.LandingSubmission
}

func (s *submissionServiceStub) SubmitPublic(_ context.Context, _ coretenant.Scope, p repository.CreateSubmissionParams, isPlatformOrg bool) (service.PublicSubmitResult, error) {
	s.submitCalls++
	s.submitParams = p
	s.submitPlat = isPlatformOrg
	if s.submitErr != nil {
		return service.PublicSubmitResult{}, s.submitErr
	}
	return service.PublicSubmitResult{Reference: "REF-1", SuccessMessage: "Terima kasih", RedirectURL: "/thanks"}, nil
}

func (s *submissionServiceStub) RetryCRMSync(_ context.Context, scope coretenant.Scope, id string, isPlatformOrg bool) (domain.LandingSubmission, error) {
	s.retryCalls++
	s.retryID = id
	s.retryScope = scope
	s.retryPlat = isPlatformOrg
	return s.retryRes, s.retryErr
}

type rateCounterStub struct {
	count int64
	keys  []string
}

func (r *rateCounterStub) Incr(ctx context.Context, key string) *goredis.IntCmd {
	r.count++
	r.keys = append(r.keys, key)
	cmd := goredis.NewIntCmd(ctx)
	cmd.SetVal(r.count)
	return cmd
}

func (r *rateCounterStub) Expire(ctx context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	cmd := goredis.NewBoolCmd(ctx)
	cmd.SetVal(true)
	return cmd
}

var _ coremiddleware.RateCounter = (*rateCounterStub)(nil)

func tenantMiddleware(t *testing.T, orgID string, orgType coretenant.OrganizationType) gin.HandlerFunc {
	t.Helper()
	return func(c *gin.Context) {
		tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     orgID,
			OrganizationSlug:   "org",
			OrganizationType:   orgType,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			ResolutionSource:   coretenant.ResolutionSourceCustomDomain,
			DataPlacement:      coretenant.DataPlacementShared,
			RequestHost:        "org.app.zyad.test",
		})
		if err != nil {
			t.Fatalf("NewVerifiedContext() error = %v", err)
		}
		coremiddleware.SetTenantContext(c, tc)
		c.Next()
	}
}

const submitBody = `{"fields":{"email":"a@b.id"},"consent":true,"website":""}`

func newSubmitRouter(t *testing.T, svc service.SubmissionService, counter coremiddleware.RateCounter, orgType coretenant.OrganizationType) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(tenantMiddleware(t, "11111111-1111-1111-1111-111111111111", orgType))
	NewPublicLandingHandler(nil, nil, svc, nil, counter).RegisterRoutes(router.Group(""))
	return router
}

func postSubmit(router *gin.Engine, formKey, idemKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/public/landing/forms/"+formKey+"/submissions", strings.NewReader(submitBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPublicSubmit_RateLimited(t *testing.T) {
	svc := &submissionServiceStub{}
	counter := &rateCounterStub{}
	router := newSubmitRouter(t, svc, counter, coretenant.OrganizationTypeCustomer)

	for i := 1; i <= 5; i++ {
		if rec := postSubmit(router, "form-1", ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d body=%s", i, rec.Code, rec.Body.String())
		}
	}
	rec := postSubmit(router, "form-1", "")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("6th request status = %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.submitCalls != 5 {
		t.Fatalf("submitCalls = %d, want 5", svc.submitCalls)
	}
	if !strings.HasPrefix(counter.keys[0], "rl:lform:203.0.113.9:form-1:") {
		t.Fatalf("rate limit key = %q", counter.keys[0])
	}
}

func TestPublicSubmit_InactiveForm404(t *testing.T) {
	for name, err := range map[string]error{
		"inactive": service.ErrFormNotActive,
		"unknown":  pgx.ErrNoRows,
	} {
		t.Run(name, func(t *testing.T) {
			router := newSubmitRouter(t, &submissionServiceStub{submitErr: err}, nil, coretenant.OrganizationTypeCustomer)
			rec := postSubmit(router, "form-1", "")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "LANDING_FORM_NOT_FOUND") {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPublicSubmit_ResponseIsMinimal(t *testing.T) {
	router := newSubmitRouter(t, &submissionServiceStub{}, nil, coretenant.OrganizationTypeCustomer)
	rec := postSubmit(router, "form-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, k := range []string{"SubmittedData", "IPAddressHash", "UserAgent", "OrganizationID", "IdempotencyKey", "CRMLeadID", "CRMSyncError"} {
		if strings.Contains(body, k) {
			t.Fatalf("response leaks %s: %s", k, body)
		}
	}
	for _, k := range []string{`"reference":"REF-1"`, `"success_message":"Terima kasih"`, `"redirect_url":"/thanks"`} {
		if !strings.Contains(body, k) {
			t.Fatalf("response missing %s: %s", k, body)
		}
	}
}

func TestPublicSubmit_InvalidSubmissionIs422(t *testing.T) {
	err := fmt.Errorf("%w: too many fields", service.ErrInvalidSubmission)
	router := newSubmitRouter(t, &submissionServiceStub{submitErr: err}, nil, coretenant.OrganizationTypeCustomer)
	rec := postSubmit(router, "form-1", "")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublicSubmit_InternalErrorIsGeneric(t *testing.T) {
	router := newSubmitRouter(t, &submissionServiceStub{submitErr: errors.New("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505) a@b.id")}, nil, coretenant.OrganizationTypeCustomer)
	rec := postSubmit(router, "form-1", "")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "SQLSTATE") || strings.Contains(rec.Body.String(), "a@b.id") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestPublicSubmit_OversizedBodyRejected(t *testing.T) {
	router := newSubmitRouter(t, &submissionServiceStub{}, nil, coretenant.OrganizationTypeCustomer)
	big := `{"fields":{"message":"` + strings.Repeat("a", 70*1024) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/public/landing/forms/form-1/submissions", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublicSubmit_SpamIs422AndOtherErrorsAre500(t *testing.T) {
	router := newSubmitRouter(t, &submissionServiceStub{submitErr: service.ErrSpamDetected}, nil, coretenant.OrganizationTypeCustomer)
	rec := postSubmit(router, "form-1", "")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("spam status = %d body=%s", rec.Code, rec.Body.String())
	}
	router = newSubmitRouter(t, &submissionServiceStub{submitErr: errors.New("db down")}, nil, coretenant.OrganizationTypeCustomer)
	if rec := postSubmit(router, "form-1", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublicSubmit_PlatformFlagAndIdempotencyKeyForwarded(t *testing.T) {
	svc := &submissionServiceStub{}
	router := newSubmitRouter(t, svc, nil, coretenant.OrganizationTypePlatform)
	if rec := postSubmit(router, "form-1", "key-123"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !svc.submitPlat {
		t.Fatal("isPlatformOrg = false for platform tenant")
	}
	if svc.submitParams.IdempotencyKey != "key-123" {
		t.Fatalf("IdempotencyKey = %q", svc.submitParams.IdempotencyKey)
	}

	svc = &submissionServiceStub{}
	router = newSubmitRouter(t, svc, nil, coretenant.OrganizationTypeCustomer)
	postSubmit(router, "form-1", "")
	if svc.submitPlat {
		t.Fatal("isPlatformOrg = true for customer tenant")
	}
}
