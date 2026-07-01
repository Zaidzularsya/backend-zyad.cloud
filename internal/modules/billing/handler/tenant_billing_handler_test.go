package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/billing/dto"

	"github.com/gin-gonic/gin"
)

const (
	tenantBillingOrganizationID = "11111111-1111-1111-1111-111111111111"
	tenantBillingMembershipID   = "22222222-2222-2222-2222-222222222222"
	tenantBillingUserID         = "33333333-3333-3333-3333-333333333333"
)

type tenantBillingHandlerServiceStub struct {
	organizationID string
	usageQuery     dto.UsageQuery
	upgradeRequest dto.UpgradeSubscriptionRequest
	upgradeActorID string
	cancelReason   string
	cancelActorID  string
}

func (s *tenantBillingHandlerServiceStub) CurrentPlan(
	_ context.Context,
	organizationID string,
) (dto.CurrentPlanResponse, error) {
	s.organizationID = organizationID
	return dto.CurrentPlanResponse{}, nil
}

func (s *tenantBillingHandlerServiceStub) CheckUsage(
	_ context.Context,
	organizationID string,
	query dto.UsageQuery,
) (dto.UsageItemResponse, error) {
	s.organizationID = organizationID
	s.usageQuery = query
	return dto.UsageItemResponse{FeatureKey: query.FeatureKey}, nil
}

func (s *tenantBillingHandlerServiceStub) ListInvoices(
	_ context.Context,
	organizationID string,
	_ dto.InvoiceListQuery,
) (dto.InvoiceListResponse, error) {
	s.organizationID = organizationID
	return dto.InvoiceListResponse{}, nil
}

func (s *tenantBillingHandlerServiceStub) RequestUpgrade(
	_ context.Context,
	organizationID string,
	actorUserID string,
	request dto.UpgradeSubscriptionRequest,
) (dto.InvoiceResponse, error) {
	s.organizationID = organizationID
	s.upgradeActorID = actorUserID
	s.upgradeRequest = request
	return dto.InvoiceResponse{OrganizationID: organizationID, Status: "open"}, nil
}

func (s *tenantBillingHandlerServiceStub) CancelCurrentSubscription(
	_ context.Context,
	organizationID string,
	actorUserID string,
	reason string,
) (dto.SubscriptionResponse, error) {
	s.organizationID = organizationID
	s.cancelActorID = actorUserID
	s.cancelReason = reason
	return dto.SubscriptionResponse{
		OrganizationID:    organizationID,
		Status:            "active",
		CancelAtPeriodEnd: true,
	}, nil
}

type tenantBillingOrganizationCheckerStub struct {
	organizationID string
	permissions    []string
}

func (s *tenantBillingOrganizationCheckerStub) CanOrganization(
	_ context.Context,
	_ string,
	organizationID string,
	permissions []string,
) error {
	s.organizationID = organizationID
	s.permissions = append(s.permissions, permissions...)
	return nil
}

func TestTenantBillingHandlerCheckUsageUsesVerifiedOrganization(t *testing.T) {
	service := &tenantBillingHandlerServiceStub{}
	checker := &tenantBillingOrganizationCheckerStub{}
	router := tenantBillingRouter(t, service, checker)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodGet,
			"/app/billing/usage?feature_key=whatsapp.max_messages_per_month&metric_key=messages&limit_key=limit&period_start=2026-06-01T00:00:00Z&period_end=2026-07-01T00:00:00Z",
			nil,
		),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != tenantBillingOrganizationID ||
		service.usageQuery.MetricKey != "messages" ||
		checker.organizationID != tenantBillingOrganizationID ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "organization.billing.read" {
		t.Fatalf("service=%#v checker=%#v", service, checker)
	}
}

func TestTenantBillingHandlerCancelSubscriptionUsesActor(t *testing.T) {
	service := &tenantBillingHandlerServiceStub{}
	checker := &tenantBillingOrganizationCheckerStub{}
	router := tenantBillingRouter(t, service, checker)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/app/billing/cancel",
		bytes.NewBufferString(`{"reason":"customer requested cancellation"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("scheduled for cancellation")) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if service.organizationID != tenantBillingOrganizationID ||
		service.cancelActorID != tenantBillingUserID ||
		service.cancelReason != "customer requested cancellation" ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "organization.billing.manage" {
		t.Fatalf("service=%#v checker=%#v", service, checker)
	}
}

func TestTenantBillingHandlerUpgradeUsesActor(t *testing.T) {
	service := &tenantBillingHandlerServiceStub{}
	checker := &tenantBillingOrganizationCheckerStub{}
	router := tenantBillingRouter(t, service, checker)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/app/billing/upgrade",
		bytes.NewBufferString(`{"plan_id":"plan-growth","billing_interval":"yearly","reason":"need more seats"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != tenantBillingOrganizationID ||
		service.upgradeActorID != tenantBillingUserID ||
		service.upgradeRequest.PlanID != "plan-growth" ||
		service.upgradeRequest.BillingInterval == nil ||
		*service.upgradeRequest.BillingInterval != "yearly" ||
		service.upgradeRequest.Reason != "need more seats" ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "organization.billing.manage" {
		t.Fatalf("service=%#v checker=%#v", service, checker)
	}
}

func tenantBillingRouter(
	t *testing.T,
	service TenantBillingService,
	checker permissionmiddleware.OrganizationPermissionChecker,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     tenantBillingOrganizationID,
			OrganizationSlug:   "acme",
			OrganizationType:   coretenant.OrganizationTypeCustomer,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			MembershipID:       tenantBillingMembershipID,
			MembershipStatus:   "active",
			MembershipVersion:  1,
			ResolutionSource:   coretenant.ResolutionSourceSession,
			DataPlacement:      coretenant.DataPlacementShared,
		})
		if err != nil {
			t.Fatalf("create tenant context: %v", err)
		}
		middleware.SetTenantContext(c, tenantContext)
		permissionmiddleware.SetUserID(c, tenantBillingUserID)
		c.Next()
	})
	group := router.Group("")
	NewTenantBillingHandler(service, checker).RegisterRoutes(group)
	return router
}
