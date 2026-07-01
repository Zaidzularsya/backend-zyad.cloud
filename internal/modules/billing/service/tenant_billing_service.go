package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	organizationdto "zyad.cloud/internal/modules/organization/dto"
)

type TenantBillingSubscriptionReader interface {
	FindUsableByOrganization(ctx context.Context, organizationID string) (dto.SubscriptionResponse, error)
	FindLatestByOrganization(ctx context.Context, organizationID string) (dto.SubscriptionResponse, error)
	ScheduleCancellation(
		ctx context.Context,
		organizationID string,
		id string,
		actorUserID string,
		reason string,
	) (dto.SubscriptionResponse, error)
}

type TenantBillingPlanReader interface {
	FindByID(ctx context.Context, id string, includePrices bool) (dto.PlanResponse, error)
}

type TenantBillingInvoiceReader interface {
	List(ctx context.Context, query dto.InvoiceListQuery) (dto.InvoiceListResponse, error)
	Create(ctx context.Context, request dto.CreateInvoiceRequest) (dto.InvoiceResponse, error)
}

type TenantBillingUsageReader interface {
	CheckUsage(
		ctx context.Context,
		organizationID string,
		query organizationdto.UsageQuery,
	) (organizationdto.UsageResponse, error)
}

type TenantBillingService struct {
	subscriptions TenantBillingSubscriptionReader
	plans         TenantBillingPlanReader
	invoices      TenantBillingInvoiceReader
	usage         TenantBillingUsageReader
	now           func() time.Time
}

type billingUsageSummarySpec struct {
	featureKey string
	metricKey  string
	limitKey   string
}

var tenantBillingUsageSummarySpecs = []billingUsageSummarySpec{
	{
		featureKey: "whatsapp.max_messages_per_month",
		metricKey:  "messages",
		limitKey:   "limit",
	},
	{
		featureKey: "automation.max_runs_per_month",
		metricKey:  "runs",
		limitKey:   "limit",
	},
}

func NewTenantBillingService(
	subscriptions TenantBillingSubscriptionReader,
	plans TenantBillingPlanReader,
	invoices TenantBillingInvoiceReader,
	usage TenantBillingUsageReader,
) *TenantBillingService {
	return &TenantBillingService{
		subscriptions: subscriptions,
		plans:         plans,
		invoices:      invoices,
		usage:         usage,
		now:           time.Now,
	}
}

func (s *TenantBillingService) CurrentPlan(
	ctx context.Context,
	organizationID string,
) (dto.CurrentPlanResponse, error) {
	subscription, err := s.subscriptions.FindUsableByOrganization(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && s.subscriptions != nil {
			subscription, err = s.subscriptions.FindLatestByOrganization(ctx, strings.TrimSpace(organizationID))
			if err != nil {
				return dto.CurrentPlanResponse{}, err
			}
		} else if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return dto.CurrentPlanResponse{}, mapSubscriptionError(err)
			}
			return dto.CurrentPlanResponse{}, err
		}
	}
	if s.plans != nil && strings.TrimSpace(subscription.PlanID) != "" {
		plan, planErr := s.plans.FindByID(ctx, subscription.PlanID, true)
		if planErr == nil {
			subscription.Plan = &plan
		}
	}
	return dto.CurrentPlanResponse{
		Subscription: subscription,
		Usage:        s.currentUsageSummary(ctx, strings.TrimSpace(organizationID)),
	}, nil
}

func (s *TenantBillingService) RequestUpgrade(
	ctx context.Context,
	organizationID string,
	actorUserID string,
	request dto.UpgradeSubscriptionRequest,
) (dto.InvoiceResponse, error) {
	if s.subscriptions == nil || s.plans == nil || s.invoices == nil {
		return dto.InvoiceResponse{}, validationError("tenant billing dependencies are incomplete")
	}

	subscription, err := s.subscriptions.FindLatestByOrganization(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.InvoiceResponse{}, mapSubscriptionError(err)
		}
		return dto.InvoiceResponse{}, err
	}

	targetPlan, err := s.plans.FindByID(ctx, strings.TrimSpace(request.PlanID), true)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}
	if !targetPlan.IsActive {
		return dto.InvoiceResponse{}, validationError("target plan is inactive")
	}
	if !targetPlan.IsPublic {
		return dto.InvoiceResponse{}, validationError("target plan is not available for self-service upgrade")
	}
	if subscription.PlanID == targetPlan.ID {
		return dto.InvoiceResponse{}, validationError("target plan must be different from current plan")
	}

	interval := strings.TrimSpace(subscription.BillingInterval)
	if request.BillingInterval != nil && strings.TrimSpace(*request.BillingInterval) != "" {
		interval = strings.TrimSpace(*request.BillingInterval)
	}
	price, err := matchingPlanPrice(targetPlan.Prices, interval)
	if err != nil {
		return dto.InvoiceResponse{}, err
	}

	dueDate := s.now().UTC().AddDate(0, 0, 7).Format(time.RFC3339)
	metadata := map[string]any{
		"billing_action":       "upgrade_request",
		"source_plan_id":       subscription.PlanID,
		"target_plan_id":       targetPlan.ID,
		"target_plan_code":     targetPlan.Code,
		"billing_interval":     price.BillingInterval,
		"requested_by_user_id": strings.TrimSpace(actorUserID),
	}
	if strings.TrimSpace(request.Reason) != "" {
		metadata["reason"] = strings.TrimSpace(request.Reason)
	}

	return s.invoices.Create(ctx, dto.CreateInvoiceRequest{
		OrganizationID: organizationID,
		SubscriptionID: subscription.ID,
		Currency:       price.Currency,
		DueDate:        &dueDate,
		Metadata:       metadata,
		Items: []dto.CreateInvoiceItemRequest{
			{
				Type:        string(model.InvoiceItemTypeSubscription),
				Description: upgradeInvoiceDescription(targetPlan.Name, price.BillingInterval),
				Quantity:    "1",
				UnitAmount:  price.Amount,
				Metadata: map[string]any{
					"source_plan_id":   subscription.PlanID,
					"target_plan_id":   targetPlan.ID,
					"billing_interval": price.BillingInterval,
				},
			},
		},
	})
}

func (s *TenantBillingService) ListInvoices(
	ctx context.Context,
	organizationID string,
	query dto.InvoiceListQuery,
) (dto.InvoiceListResponse, error) {
	query.OrganizationID = strings.TrimSpace(organizationID)
	return s.invoices.List(ctx, query)
}

func (s *TenantBillingService) CheckUsage(
	ctx context.Context,
	organizationID string,
	query dto.UsageQuery,
) (dto.UsageItemResponse, error) {
	usage, err := s.usage.CheckUsage(ctx, strings.TrimSpace(organizationID), organizationdto.UsageQuery{
		FeatureKey:  query.FeatureKey,
		MetricKey:   query.MetricKey,
		LimitKey:    query.LimitKey,
		PeriodStart: query.PeriodStart,
		PeriodEnd:   query.PeriodEnd,
	})
	if err != nil {
		return dto.UsageItemResponse{}, err
	}
	return dto.UsageItemResponse{
		FeatureKey:     usage.FeatureKey,
		MetricKey:      usage.MetricKey,
		UsedValue:      strconv.FormatInt(usage.UsageValue, 10),
		LimitValue:     int64PointerString(usage.LimitValue),
		RemainingValue: int64PointerString(usage.RemainingValue),
		PeriodStart:    stringPointer(usage.PeriodStart),
		PeriodEnd:      stringPointer(usage.PeriodEnd),
	}, nil
}

func (s *TenantBillingService) CancelCurrentSubscription(
	ctx context.Context,
	organizationID string,
	actorUserID string,
	reason string,
) (dto.SubscriptionResponse, error) {
	subscription, err := s.subscriptions.FindUsableByOrganization(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.SubscriptionResponse{}, mapSubscriptionError(err)
		}
		return dto.SubscriptionResponse{}, err
	}
	return s.subscriptions.ScheduleCancellation(
		ctx,
		strings.TrimSpace(organizationID),
		subscription.ID,
		strings.TrimSpace(actorUserID),
		strings.TrimSpace(reason),
	)
}

func int64PointerString(value *int64) *string {
	if value == nil {
		return nil
	}
	formatted := strconv.FormatInt(*value, 10)
	return &formatted
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func matchingPlanPrice(prices []dto.PlanPriceResponse, billingInterval string) (dto.PlanPriceResponse, error) {
	billingInterval = strings.TrimSpace(billingInterval)
	if billingInterval == "" {
		return dto.PlanPriceResponse{}, validationError("billing interval is required")
	}
	for _, price := range prices {
		if strings.EqualFold(strings.TrimSpace(price.BillingInterval), billingInterval) && price.IsActive {
			return price, nil
		}
	}
	return dto.PlanPriceResponse{}, validationError("target plan price for billing interval is not available")
}

func upgradeInvoiceDescription(planName string, billingInterval string) string {
	name := strings.TrimSpace(planName)
	interval := strings.ReplaceAll(strings.TrimSpace(billingInterval), "_", " ")
	if name == "" {
		name = "selected plan"
	}
	if interval == "" {
		return "Upgrade to " + name
	}
	return "Upgrade to " + name + " (" + interval + ")"
}

func (s *TenantBillingService) currentUsageSummary(
	ctx context.Context,
	organizationID string,
) []dto.UsageItemResponse {
	if s == nil || s.usage == nil {
		return nil
	}
	periodStart, periodEnd := currentMonthPeriod(s.now().UTC())
	items := make([]dto.UsageItemResponse, 0, len(tenantBillingUsageSummarySpecs))
	for _, spec := range tenantBillingUsageSummarySpecs {
		usage, err := s.usage.CheckUsage(ctx, organizationID, organizationdto.UsageQuery{
			FeatureKey:  spec.featureKey,
			MetricKey:   spec.metricKey,
			LimitKey:    spec.limitKey,
			PeriodStart: periodStart.Format(time.RFC3339),
			PeriodEnd:   periodEnd.Format(time.RFC3339),
		})
		if err != nil {
			continue
		}
		items = append(items, dto.UsageItemResponse{
			FeatureKey:     usage.FeatureKey,
			MetricKey:      usage.MetricKey,
			UsedValue:      strconv.FormatInt(usage.UsageValue, 10),
			LimitValue:     int64PointerString(usage.LimitValue),
			RemainingValue: int64PointerString(usage.RemainingValue),
			PeriodStart:    stringPointer(usage.PeriodStart),
			PeriodEnd:      stringPointer(usage.PeriodEnd),
		})
	}
	return items
}

func currentMonthPeriod(now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return periodStart, periodStart.AddDate(0, 1, 0)
}
