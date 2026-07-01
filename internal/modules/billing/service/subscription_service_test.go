package service

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type stubSubscriptionStore struct {
	current     model.Subscription
	created     model.Subscription
	updated     model.Subscription
	events      []repository.SubscriptionEventParams
	lastUpdate  repository.UpdateSubscriptionParams
	updateCalls int
}

func (s *stubSubscriptionStore) Create(
	context.Context,
	repository.CreateSubscriptionParams,
) (model.Subscription, error) {
	return s.created, nil
}

func (s *stubSubscriptionStore) FindByID(
	context.Context,
	string,
	string,
) (model.Subscription, error) {
	return s.current, nil
}

func (s *stubSubscriptionStore) FindByIDUnscoped(
	context.Context,
	string,
) (model.Subscription, error) {
	return s.current, nil
}

func (s *stubSubscriptionStore) FindUsableByOrganization(
	context.Context,
	string,
) (model.Subscription, error) {
	return s.current, nil
}

func (s *stubSubscriptionStore) List(
	context.Context,
	repository.SubscriptionListFilter,
) ([]model.Subscription, int64, error) {
	return []model.Subscription{s.current}, 1, nil
}

func (s *stubSubscriptionStore) Update(
	_ context.Context,
	params repository.UpdateSubscriptionParams,
) (model.Subscription, error) {
	s.updateCalls++
	s.lastUpdate = params
	return s.updated, nil
}

func (s *stubSubscriptionStore) CreateEvent(
	_ context.Context,
	params repository.SubscriptionEventParams,
) (model.SubscriptionEvent, error) {
	s.events = append(s.events, params)
	return model.SubscriptionEvent{}, nil
}

type stubSubscriptionEntitlementStore struct {
	entitlements []model.PlanEntitlement
}

func (s stubSubscriptionEntitlementStore) ListByPlanID(
	context.Context,
	string,
) ([]model.PlanEntitlement, error) {
	return s.entitlements, nil
}

type stubSubscriptionEntitlementSink struct {
	syncCalls        int
	expireCalls      int
	lastSync         repository.SyncPlanEntitlementsParams
	lastExpireActor  string
	lastExpireReason string
}

func (s *stubSubscriptionEntitlementSink) SyncPlanEntitlements(
	_ context.Context,
	params repository.SyncPlanEntitlementsParams,
) ([]organizationmodel.Entitlement, error) {
	s.syncCalls++
	s.lastSync = params
	return nil, nil
}

func (s *stubSubscriptionEntitlementSink) ExpirePlanEntitlements(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
	actorUserID string,
	reason string,
) (int64, error) {
	s.expireCalls++
	s.lastExpireActor = actorUserID
	s.lastExpireReason = reason
	return 1, nil
}

func TestSubscriptionServiceCreateActiveSyncsEntitlements(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &stubSubscriptionStore{
		created: model.Subscription{
			ID:              "subscription-1",
			OrganizationID:  "organization-1",
			PlanID:          "plan-1",
			Status:          model.SubscriptionStatusActive,
			BillingInterval: model.BillingIntervalMonthly,
		},
	}
	sink := &stubSubscriptionEntitlementSink{}
	service := NewSubscriptionService(store, stubSubscriptionEntitlementStore{
		entitlements: []model.PlanEntitlement{
			{PlanID: "plan-1", FeatureID: "feature-1", FeatureKey: "landing.enabled"},
		},
	}, sink)
	service.now = func() time.Time { return now }

	_, err := service.Create(context.Background(), dto.CreateSubscriptionRequest{
		OrganizationID:  "organization-1",
		PlanID:          "plan-1",
		BillingInterval: "monthly",
		Status:          "active",
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if len(store.events) != 1 || store.events[0].Type != "subscription_created" {
		t.Fatalf("events = %#v, want subscription_created", store.events)
	}
	if sink.syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1", sink.syncCalls)
	}
	if sink.lastSync.SubscriptionID != "subscription-1" || sink.lastSync.OrganizationID != "organization-1" {
		t.Fatalf("lastSync = %#v", sink.lastSync)
	}
}

func TestSubscriptionServiceRejectsInvalidStatusTransition(t *testing.T) {
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-1",
			Status:         model.SubscriptionStatusCanceled,
		},
	}
	service := NewSubscriptionService(store, nil, nil)
	status := string(model.SubscriptionStatusActive)

	_, err := service.Update(context.Background(), "organization-1", "subscription-1", dto.UpdateSubscriptionRequest{
		Status: &status,
	})
	if err == nil {
		t.Fatal("Update error = nil, want validation error")
	}
	if store.updateCalls != 0 {
		t.Fatalf("updateCalls = %d, want 0", store.updateCalls)
	}
}

func TestSubscriptionServiceUpdateByIDUsesCurrentOrganization(t *testing.T) {
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-1",
			Status:         model.SubscriptionStatusActive,
		},
		updated: model.Subscription{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-1",
			Status:         model.SubscriptionStatusSuspended,
		},
	}
	service := NewSubscriptionService(store, nil, nil)
	status := string(model.SubscriptionStatusSuspended)

	response, err := service.UpdateByID(context.Background(), "subscription-1", dto.UpdateSubscriptionRequest{
		Status: &status,
		Reason: "manual hold",
	})
	if err != nil {
		t.Fatalf("UpdateByID error = %v", err)
	}
	if response.OrganizationID != "organization-1" || response.Status != "suspended" {
		t.Fatalf("response = %#v", response)
	}
	if store.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", store.updateCalls)
	}
}

func TestSubscriptionServiceScheduleCancellationUsesPeriodEndFlag(t *testing.T) {
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			PlanID:            "plan-1",
			Status:            model.SubscriptionStatusActive,
			BillingInterval:   model.BillingIntervalMonthly,
			CancelAtPeriodEnd: false,
		},
		updated: model.Subscription{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			PlanID:            "plan-1",
			Status:            model.SubscriptionStatusActive,
			BillingInterval:   model.BillingIntervalMonthly,
			CancelAtPeriodEnd: true,
		},
	}
	service := NewSubscriptionService(store, nil, nil)

	response, err := service.ScheduleCancellation(
		context.Background(),
		"organization-1",
		"subscription-1",
		"user-1",
		"customer requested cancellation",
	)
	if err != nil {
		t.Fatalf("ScheduleCancellation error = %v", err)
	}
	if !response.CancelAtPeriodEnd || response.Status != "active" {
		t.Fatalf("response = %#v", response)
	}
	if store.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", store.updateCalls)
	}
	if store.lastUpdate.CancelAtPeriodEnd == nil || !*store.lastUpdate.CancelAtPeriodEnd {
		t.Fatalf("lastUpdate.CancelAtPeriodEnd = %#v", store.lastUpdate.CancelAtPeriodEnd)
	}
	if len(store.events) != 1 || store.events[0].Type != "subscription_updated" {
		t.Fatalf("events = %#v", store.events)
	}
	if store.events[0].ActorUserID == nil || *store.events[0].ActorUserID != "user-1" {
		t.Fatalf("event actor = %#v", store.events[0].ActorUserID)
	}
}

func TestSubscriptionServiceScheduleCancellationReturnsCurrentWhenAlreadyScheduled(t *testing.T) {
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			PlanID:            "plan-1",
			Status:            model.SubscriptionStatusActive,
			BillingInterval:   model.BillingIntervalMonthly,
			CancelAtPeriodEnd: true,
		},
	}
	service := NewSubscriptionService(store, nil, nil)

	response, err := service.ScheduleCancellation(
		context.Background(),
		"organization-1",
		"subscription-1",
		"user-1",
		"",
	)
	if err != nil {
		t.Fatalf("ScheduleCancellation error = %v", err)
	}
	if !response.CancelAtPeriodEnd {
		t.Fatalf("response = %#v", response)
	}
	if store.updateCalls != 0 {
		t.Fatalf("updateCalls = %d, want 0", store.updateCalls)
	}
	if len(store.events) != 0 {
		t.Fatalf("events = %#v", store.events)
	}
}

func TestSubscriptionServiceActivateUpgradeByInvoiceUpdatesPlanAndEntitlements(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 30, 0, 0, time.UTC)
	paidAt := now.Add(15 * time.Minute)
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			PlanID:            "plan-basic",
			Status:            model.SubscriptionStatusSuspended,
			BillingInterval:   model.BillingIntervalMonthly,
			CancelAtPeriodEnd: true,
		},
		updated: model.Subscription{
			ID:                "subscription-1",
			OrganizationID:    "organization-1",
			PlanID:            "plan-pro",
			Status:            model.SubscriptionStatusActive,
			BillingInterval:   model.BillingIntervalYearly,
			CurrentPeriodEnd:  defaultPeriodEnd(paidAt.UTC(), "yearly"),
			CancelAtPeriodEnd: false,
		},
	}
	sink := &stubSubscriptionEntitlementSink{}
	service := NewSubscriptionService(store, stubSubscriptionEntitlementStore{
		entitlements: []model.PlanEntitlement{
			{PlanID: "plan-pro", FeatureID: "feature-1", FeatureKey: "landing.enabled"},
		},
	}, sink)
	service.now = func() time.Time { return now }
	invoice := model.Invoice{
		ID:             "invoice-upgrade-1",
		OrganizationID: "organization-1",
		SubscriptionID: stringPointer("subscription-1"),
		Metadata: map[string]any{
			"billing_action":       "upgrade_request",
			"target_plan_id":       "plan-pro",
			"billing_interval":     "yearly",
			"reason":               "customer requested annual billing",
			"requested_by_user_id": "user-42",
		},
	}

	if err := service.ActivateUpgradeByInvoice(context.Background(), invoice, paidAt); err != nil {
		t.Fatalf("ActivateUpgradeByInvoice error = %v", err)
	}
	if store.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", store.updateCalls)
	}
	if store.lastUpdate.PlanID == nil || *store.lastUpdate.PlanID != "plan-pro" {
		t.Fatalf("lastUpdate.PlanID = %#v", store.lastUpdate.PlanID)
	}
	if store.lastUpdate.Status == nil || *store.lastUpdate.Status != model.SubscriptionStatusActive {
		t.Fatalf("lastUpdate.Status = %#v", store.lastUpdate.Status)
	}
	if store.lastUpdate.BillingInterval == nil || *store.lastUpdate.BillingInterval != model.BillingIntervalYearly {
		t.Fatalf("lastUpdate.BillingInterval = %#v", store.lastUpdate.BillingInterval)
	}
	if store.lastUpdate.CancelAtPeriodEnd == nil || *store.lastUpdate.CancelAtPeriodEnd {
		t.Fatalf("lastUpdate.CancelAtPeriodEnd = %#v", store.lastUpdate.CancelAtPeriodEnd)
	}
	if len(store.events) != 1 || store.events[0].Type != "subscription_plan_changed" {
		t.Fatalf("events = %#v", store.events)
	}
	if store.events[0].ActorUserID == nil || *store.events[0].ActorUserID != "user-42" {
		t.Fatalf("event actor = %#v", store.events[0].ActorUserID)
	}
	if sink.syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1", sink.syncCalls)
	}
	if sink.lastSync.SubscriptionID != "subscription-1" || sink.lastSync.OrganizationID != "organization-1" {
		t.Fatalf("lastSync = %#v", sink.lastSync)
	}
	if sink.lastSync.ActorUserID != "user-42" {
		t.Fatalf("lastSync actor = %s, want user-42", sink.lastSync.ActorUserID)
	}
}

func TestSubscriptionServiceChangeStatusPropagatesActorToEventAndExpire(t *testing.T) {
	store := &stubSubscriptionStore{
		current: model.Subscription{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-1",
			Status:         model.SubscriptionStatusActive,
		},
		updated: model.Subscription{
			ID:             "subscription-1",
			OrganizationID: "organization-1",
			PlanID:         "plan-1",
			Status:         model.SubscriptionStatusSuspended,
		},
	}
	sink := &stubSubscriptionEntitlementSink{}
	service := NewSubscriptionService(store, nil, sink)

	response, err := service.ChangeStatus(
		context.Background(),
		"organization-1",
		"subscription-1",
		model.SubscriptionStatusSuspended,
		"user-9",
		"manual hold",
	)
	if err != nil {
		t.Fatalf("ChangeStatus error = %v", err)
	}
	if response.Status != "suspended" {
		t.Fatalf("response = %#v", response)
	}
	if len(store.events) != 1 || store.events[0].Type != "subscription_status_changed" {
		t.Fatalf("events = %#v", store.events)
	}
	if store.events[0].ActorUserID == nil || *store.events[0].ActorUserID != "user-9" {
		t.Fatalf("event actor = %#v", store.events[0].ActorUserID)
	}
	if sink.expireCalls != 1 || sink.lastExpireActor != "user-9" {
		t.Fatalf("expire sink = %#v", sink)
	}
}
