package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/billing/dto"
	organizationdto "zyad.cloud/internal/modules/organization/dto"
	productdto "zyad.cloud/internal/modules/product/dto"
	subscriptiondto "zyad.cloud/internal/modules/subscription/dto"
)

type stubTenantBillingSubscriptionReader struct {
	subscription       subscriptiondto.SubscriptionResponse
	latestSubscription subscriptiondto.SubscriptionResponse
	usableErr          error
	changeResult       subscriptiondto.SubscriptionResponse
	organizationID     string
	changedID          string
	changedActorUserID string
	changedReason      string
}

func (s *stubTenantBillingSubscriptionReader) FindUsableByOrganization(
	context.Context,
	string,
) (subscriptiondto.SubscriptionResponse, error) {
	return s.subscription, s.usableErr
}

func (s *stubTenantBillingSubscriptionReader) FindLatestByOrganization(
	context.Context,
	string,
) (subscriptiondto.SubscriptionResponse, error) {
	if s.latestSubscription.ID != "" {
		return s.latestSubscription, nil
	}
	return s.subscription, nil
}

func (s *stubTenantBillingSubscriptionReader) ScheduleCancellation(
	_ context.Context,
	organizationID string,
	id string,
	actorUserID string,
	reason string,
) (subscriptiondto.SubscriptionResponse, error) {
	s.organizationID = organizationID
	s.changedID = id
	s.changedActorUserID = actorUserID
	s.changedReason = reason
	return s.changeResult, nil
}

type stubTenantBillingPlanReader struct {
	plan productdto.PlanResponse
}

func (s *stubTenantBillingPlanReader) FindByID(
	context.Context,
	string,
	bool,
) (productdto.PlanResponse, error) {
	return s.plan, nil
}

type stubTenantBillingInvoiceReader struct {
	result  dto.InvoiceListResponse
	query   dto.InvoiceListQuery
	created dto.CreateInvoiceRequest
}

func (s *stubTenantBillingInvoiceReader) List(
	_ context.Context,
	query dto.InvoiceListQuery,
) (dto.InvoiceListResponse, error) {
	s.query = query
	return s.result, nil
}

func (s *stubTenantBillingInvoiceReader) Create(
	_ context.Context,
	request dto.CreateInvoiceRequest,
) (dto.InvoiceResponse, error) {
	s.created = request
	return dto.InvoiceResponse{
		ID:             "invoice-1",
		OrganizationID: request.OrganizationID,
		SubscriptionID: stringPointer(request.SubscriptionID),
		Status:         "open",
		Currency:       request.Currency,
		Items: []dto.InvoiceItemResponse{
			{
				Description: request.Items[0].Description,
				UnitAmount:  request.Items[0].UnitAmount,
			},
		},
		Metadata: request.Metadata,
	}, nil
}

type stubTenantBillingUsageReader struct {
	result         organizationdto.UsageResponse
	results        map[string]organizationdto.UsageResponse
	errByFeature   map[string]error
	organizationID string
	query          organizationdto.UsageQuery
	queries        []organizationdto.UsageQuery
}

func (s *stubTenantBillingUsageReader) CheckUsage(
	_ context.Context,
	organizationID string,
	query organizationdto.UsageQuery,
) (organizationdto.UsageResponse, error) {
	s.organizationID = organizationID
	s.query = query
	s.queries = append(s.queries, query)
	if err := s.errByFeature[query.FeatureKey]; err != nil {
		return organizationdto.UsageResponse{}, err
	}
	if result, ok := s.results[query.FeatureKey]; ok {
		return result, nil
	}
	return s.result, nil
}

func TestTenantBillingServiceCheckUsageMapsOrganizationUsage(t *testing.T) {
	usageReader := &stubTenantBillingUsageReader{
		result: organizationdto.UsageResponse{
			FeatureKey:     "whatsapp.max_messages_per_month",
			MetricKey:      "messages",
			UsageValue:     12,
			PeriodStart:    "2026-06-01T00:00:00Z",
			PeriodEnd:      "2026-07-01T00:00:00Z",
			LimitValue:     int64Ptr(100),
			RemainingValue: int64Ptr(88),
		},
	}
	service := NewTenantBillingService(nil, nil, nil, usageReader)

	result, err := service.CheckUsage(context.Background(), "organization-1", dto.UsageQuery{
		FeatureKey:  "whatsapp.max_messages_per_month",
		MetricKey:   "messages",
		LimitKey:    "limit",
		PeriodStart: "2026-06-01T00:00:00Z",
		PeriodEnd:   "2026-07-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CheckUsage error = %v", err)
	}
	if usageReader.organizationID != "organization-1" || usageReader.query.MetricKey != "messages" {
		t.Fatalf("usage reader = %#v", usageReader)
	}
	if result.UsedValue != "12" || result.LimitValue == nil || *result.LimitValue != "100" {
		t.Fatalf("result = %#v", result)
	}
}

func TestTenantBillingServiceCancelCurrentSubscriptionUsesCurrentOrganization(t *testing.T) {
	subscriptionReader := &stubTenantBillingSubscriptionReader{
		subscription: subscriptiondto.SubscriptionResponse{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			Status:         "active",
		},
		changeResult: subscriptiondto.SubscriptionResponse{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			Status:            "active",
			CancelAtPeriodEnd: true,
		},
	}
	service := NewTenantBillingService(subscriptionReader, nil, nil, nil)

	result, err := service.CancelCurrentSubscription(
		context.Background(),
		"organization-1",
		"user-1",
		"customer requested cancellation",
	)
	if err != nil {
		t.Fatalf("CancelCurrentSubscription error = %v", err)
	}
	if result.Status != "active" || !result.CancelAtPeriodEnd {
		t.Fatalf("result = %#v", result)
	}
	if subscriptionReader.organizationID != "organization-1" ||
		subscriptionReader.changedID != "subscription-1" ||
		subscriptionReader.changedActorUserID != "user-1" ||
		subscriptionReader.changedReason != "customer requested cancellation" {
		t.Fatalf("subscription reader = %#v", subscriptionReader)
	}
}

func TestTenantBillingServiceCurrentPlanFallsBackToLatestSubscription(t *testing.T) {
	subscriptionReader := &stubTenantBillingSubscriptionReader{
		usableErr: pgx.ErrNoRows,
		latestSubscription: subscriptiondto.SubscriptionResponse{
			ID:             "subscription-2",
			OrganizationID: "organization-1",
			PlanID:         "plan-2",
			Status:         "suspended",
		},
	}
	planReader := &stubTenantBillingPlanReader{
		plan: productdto.PlanResponse{ID: "plan-2", Name: "Growth"},
	}
	service := NewTenantBillingService(subscriptionReader, planReader, nil, nil)

	result, err := service.CurrentPlan(context.Background(), "organization-1")
	if err != nil {
		t.Fatalf("CurrentPlan error = %v", err)
	}
	if result.Subscription.Status != "suspended" ||
		result.Subscription.Plan == nil ||
		result.Subscription.Plan.Name != "Growth" {
		t.Fatalf("result = %#v", result)
	}
}

func TestTenantBillingServiceCurrentPlanIncludesUsageSummary(t *testing.T) {
	subscriptionReader := &stubTenantBillingSubscriptionReader{
		subscription: subscriptiondto.SubscriptionResponse{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-growth",
			Status:         "active",
		},
	}
	planReader := &stubTenantBillingPlanReader{
		plan: productdto.PlanResponse{ID: "plan-growth", Name: "Growth"},
	}
	usageReader := &stubTenantBillingUsageReader{
		results: map[string]organizationdto.UsageResponse{
			"whatsapp.max_messages_per_month": {
				FeatureKey:     "whatsapp.max_messages_per_month",
				MetricKey:      "messages",
				UsageValue:     12,
				PeriodStart:    "2026-06-01T00:00:00Z",
				PeriodEnd:      "2026-07-01T00:00:00Z",
				LimitValue:     int64Ptr(1000),
				RemainingValue: int64Ptr(988),
			},
			"automation.max_runs_per_month": {
				FeatureKey:     "automation.max_runs_per_month",
				MetricKey:      "runs",
				UsageValue:     4,
				PeriodStart:    "2026-06-01T00:00:00Z",
				PeriodEnd:      "2026-07-01T00:00:00Z",
				LimitValue:     int64Ptr(50),
				RemainingValue: int64Ptr(46),
			},
		},
	}
	service := NewTenantBillingService(subscriptionReader, planReader, nil, usageReader)
	service.now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }

	result, err := service.CurrentPlan(context.Background(), "organization-1")
	if err != nil {
		t.Fatalf("CurrentPlan error = %v", err)
	}
	if len(result.Usage) != 2 {
		t.Fatalf("usage len = %d, want 2", len(result.Usage))
	}
	if usageReader.organizationID != "organization-1" || len(usageReader.queries) != 2 {
		t.Fatalf("usage reader = %#v", usageReader)
	}
	if usageReader.queries[0].PeriodStart != "2026-06-01T00:00:00Z" ||
		usageReader.queries[0].PeriodEnd != "2026-07-01T00:00:00Z" {
		t.Fatalf("usage query period = %#v", usageReader.queries[0])
	}
}

func TestTenantBillingServiceCurrentPlanSkipsUsageSummaryErrors(t *testing.T) {
	subscriptionReader := &stubTenantBillingSubscriptionReader{
		subscription: subscriptiondto.SubscriptionResponse{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			Status:         "active",
		},
	}
	usageReader := &stubTenantBillingUsageReader{
		results: map[string]organizationdto.UsageResponse{
			"automation.max_runs_per_month": {
				FeatureKey:     "automation.max_runs_per_month",
				MetricKey:      "runs",
				UsageValue:     4,
				PeriodStart:    "2026-06-01T00:00:00Z",
				PeriodEnd:      "2026-07-01T00:00:00Z",
				LimitValue:     int64Ptr(50),
				RemainingValue: int64Ptr(46),
			},
		},
		errByFeature: map[string]error{
			"whatsapp.max_messages_per_month": errors.New("entitlement unavailable"),
		},
	}
	service := NewTenantBillingService(subscriptionReader, nil, nil, usageReader)
	service.now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }

	result, err := service.CurrentPlan(context.Background(), "organization-1")
	if err != nil {
		t.Fatalf("CurrentPlan error = %v", err)
	}
	if len(result.Usage) != 1 || result.Usage[0].FeatureKey != "automation.max_runs_per_month" {
		t.Fatalf("usage = %#v", result.Usage)
	}
}

func TestTenantBillingServiceRequestUpgradeCreatesInvoice(t *testing.T) {
	interval := "yearly"
	subscriptionReader := &stubTenantBillingSubscriptionReader{
		latestSubscription: subscriptiondto.SubscriptionResponse{
			ID:              "subscription-1",
			OrganizationID:  "organization-1",
			PlanID:          "plan-starter",
			Status:          "active",
			BillingInterval: "monthly",
		},
	}
	planReader := &stubTenantBillingPlanReader{
		plan: productdto.PlanResponse{
			ID:       "plan-growth",
			Code:     "growth",
			Name:     "Growth",
			IsPublic: true,
			IsActive: true,
			Prices: []productdto.PlanPriceResponse{
				{BillingInterval: "monthly", Currency: "IDR", Amount: "199000.00", IsActive: true},
				{BillingInterval: "yearly", Currency: "IDR", Amount: "1999000.00", IsActive: true},
			},
		},
	}
	invoiceReader := &stubTenantBillingInvoiceReader{}
	service := NewTenantBillingService(subscriptionReader, planReader, invoiceReader, nil)

	result, err := service.RequestUpgrade(context.Background(), "organization-1", "user-1", dto.UpgradeSubscriptionRequest{
		PlanID:          "plan-growth",
		BillingInterval: &interval,
		Reason:          "need analytics",
	})
	if err != nil {
		t.Fatalf("RequestUpgrade error = %v", err)
	}
	if result.Status != "open" || result.Currency != "IDR" {
		t.Fatalf("result = %#v", result)
	}
	if invoiceReader.created.OrganizationID != "organization-1" ||
		invoiceReader.created.SubscriptionID != "subscription-1" ||
		len(invoiceReader.created.Items) != 1 ||
		invoiceReader.created.Items[0].UnitAmount != "1999000.00" {
		t.Fatalf("created invoice = %#v", invoiceReader.created)
	}
	if invoiceReader.created.Metadata["billing_action"] != "upgrade_request" ||
		invoiceReader.created.Metadata["target_plan_id"] != "plan-growth" {
		t.Fatalf("metadata = %#v", invoiceReader.created.Metadata)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
