package service

import (
	"context"
	"strings"
	"time"

	organizationmodel "zyad.cloud/internal/modules/organization/model"
	productmodel "zyad.cloud/internal/modules/product/model"
	subscription "zyad.cloud/internal/modules/subscription"
	"zyad.cloud/internal/modules/subscription/dto"
	"zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/modules/subscription/repository"
)

// SubscriptionInvoice is the minimal invoice shape the subscription service
// needs in order to activate a plan upgrade after payment. Billing supplies
// the concrete invoice model that structurally satisfies this shape via its
// own metadata and identifiers.
type SubscriptionInvoice struct {
	OrganizationID string
	SubscriptionID *string
	Metadata       map[string]any
}

type SubscriptionStore interface {
	Create(ctx context.Context, params repository.CreateSubscriptionParams) (model.Subscription, error)
	FindByID(ctx context.Context, organizationID string, id string) (model.Subscription, error)
	FindByIDUnscoped(ctx context.Context, id string) (model.Subscription, error)
	FindUsableByOrganization(ctx context.Context, organizationID string) (model.Subscription, error)
	List(ctx context.Context, filter repository.SubscriptionListFilter) ([]model.Subscription, int64, error)
	ListAllOrganizations(
		ctx context.Context,
		filter repository.SubscriptionListFilter,
	) ([]model.Subscription, int64, error)
	Update(ctx context.Context, params repository.UpdateSubscriptionParams) (model.Subscription, error)
	CreateEvent(ctx context.Context, params repository.SubscriptionEventParams) (model.SubscriptionEvent, error)
}

type SubscriptionEntitlementStore interface {
	ListByPlanID(ctx context.Context, planID string) ([]productmodel.PlanEntitlement, error)
}

type SubscriptionService struct {
	store            SubscriptionStore
	entitlementStore SubscriptionEntitlementStore
	entitlementSink  subscriptionEntitlementSink
	now              func() time.Time
}

type subscriptionEntitlementSink interface {
	SyncPlanEntitlements(ctx context.Context, params repository.SyncPlanEntitlementsParams) ([]organizationmodel.Entitlement, error)
	ExpirePlanEntitlements(
		ctx context.Context,
		organizationID string,
		subscriptionID string,
		effectiveUntil time.Time,
		actorUserID string,
		reason string,
	) (int64, error)
}

func NewSubscriptionService(
	store SubscriptionStore,
	entitlementStore SubscriptionEntitlementStore,
	entitlementSink subscriptionEntitlementSink,
) *SubscriptionService {
	return &SubscriptionService{
		store:            store,
		entitlementStore: entitlementStore,
		entitlementSink:  entitlementSink,
		now:              time.Now,
	}
}

// List returns subscriptions for a single tenant. query.OrganizationID must
// be set by the caller (e.g. from a verified tenant context) — this method
// refuses to run an unscoped, cross-tenant query.
func (s *SubscriptionService) List(ctx context.Context, query dto.SubscriptionListQuery) (dto.SubscriptionListResponse, error) {
	filter, page, perPage, err := subscriptionListFilter(query)
	if err != nil {
		return dto.SubscriptionListResponse{}, err
	}
	subscriptions, total, err := s.store.List(ctx, filter)
	if err != nil {
		return dto.SubscriptionListResponse{}, err
	}
	return dto.SubscriptionListResponse{
		Items: subscriptionResponses(subscriptions),
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

// ListAllOrganizations lists subscriptions across every tenant. Intended for
// platform-admin endpoints only — callers must gate access themselves.
func (s *SubscriptionService) ListAllOrganizations(
	ctx context.Context,
	query dto.SubscriptionListQuery,
) (dto.SubscriptionListResponse, error) {
	filter, page, perPage, err := subscriptionListFilter(query)
	if err != nil {
		return dto.SubscriptionListResponse{}, err
	}
	subscriptions, total, err := s.store.ListAllOrganizations(ctx, filter)
	if err != nil {
		return dto.SubscriptionListResponse{}, err
	}
	return dto.SubscriptionListResponse{
		Items: subscriptionResponses(subscriptions),
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

func (s *SubscriptionService) FindByID(ctx context.Context, organizationID string, id string) (dto.SubscriptionResponse, error) {
	subscriptionRecord, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(id))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	return subscriptionResponse(subscriptionRecord), nil
}

func (s *SubscriptionService) FindUsableByOrganization(ctx context.Context, organizationID string) (dto.SubscriptionResponse, error) {
	subscriptionRecord, err := s.store.FindUsableByOrganization(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	return subscriptionResponse(subscriptionRecord), nil
}

func (s *SubscriptionService) FindLatestByOrganization(ctx context.Context, organizationID string) (dto.SubscriptionResponse, error) {
	subscriptions, _, err := s.store.List(ctx, repository.SubscriptionListFilter{
		OrganizationID: strings.TrimSpace(organizationID),
		Limit:          1,
	})
	if err != nil {
		return dto.SubscriptionResponse{}, err
	}
	if len(subscriptions) == 0 {
		return dto.SubscriptionResponse{}, subscription.SubscriptionNotFoundError()
	}
	return subscriptionResponse(subscriptions[0]), nil
}

func (s *SubscriptionService) Create(ctx context.Context, request dto.CreateSubscriptionRequest) (dto.SubscriptionResponse, error) {
	params, err := s.createParams(request)
	if err != nil {
		return dto.SubscriptionResponse{}, err
	}
	subscriptionRecord, err := s.store.Create(ctx, params)
	if err != nil {
		return dto.SubscriptionResponse{}, err
	}
	if _, err := s.store.CreateEvent(ctx, repository.SubscriptionEventParams{
		SubscriptionID: subscriptionRecord.ID,
		OrganizationID: subscriptionRecord.OrganizationID,
		Type:           "subscription_created",
		NewStatus:      &subscriptionRecord.Status,
		Metadata:       map[string]any{"plan_id": subscriptionRecord.PlanID},
	}); err != nil {
		return dto.SubscriptionResponse{}, err
	}
	if subscriptionRecord.IsUsable() {
		if err := s.syncEntitlements(ctx, subscriptionRecord, "", "subscription created"); err != nil {
			return dto.SubscriptionResponse{}, err
		}
	}
	return subscriptionResponse(subscriptionRecord), nil
}

func (s *SubscriptionService) Update(
	ctx context.Context,
	organizationID string,
	id string,
	request dto.UpdateSubscriptionRequest,
) (dto.SubscriptionResponse, error) {
	return s.updateWithActor(ctx, organizationID, id, request, "")
}

func (s *SubscriptionService) updateWithActor(
	ctx context.Context,
	organizationID string,
	id string,
	request dto.UpdateSubscriptionRequest,
	actorUserID string,
) (dto.SubscriptionResponse, error) {
	current, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(id))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	params, err := s.updateParams(current, request)
	if err != nil {
		return dto.SubscriptionResponse{}, err
	}
	updated, err := s.store.Update(ctx, params)
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	if err := s.recordUpdateEvent(ctx, current, updated, strings.TrimSpace(actorUserID), request.Reason); err != nil {
		return dto.SubscriptionResponse{}, err
	}
	if shouldSyncSubscriptionEntitlements(current, updated) {
		if err := s.syncEntitlements(ctx, updated, strings.TrimSpace(actorUserID), "subscription updated"); err != nil {
			return dto.SubscriptionResponse{}, err
		}
	}
	if shouldExpireSubscriptionEntitlements(current, updated) {
		if err := s.expireEntitlements(ctx, updated, strings.TrimSpace(actorUserID), "subscription ended"); err != nil {
			return dto.SubscriptionResponse{}, err
		}
	}
	return subscriptionResponse(updated), nil
}

func (s *SubscriptionService) ChangeStatus(
	ctx context.Context,
	organizationID string,
	id string,
	status model.SubscriptionStatus,
	actorUserID string,
	reason string,
) (dto.SubscriptionResponse, error) {
	if !status.IsValid() {
		return dto.SubscriptionResponse{}, validationError("subscription status is invalid")
	}
	statusValue := string(status)
	return s.updateWithActor(ctx, organizationID, id, dto.UpdateSubscriptionRequest{
		Status:   &statusValue,
		Reason:   reason,
		Metadata: nil,
	}, actorUserID)
}

func (s *SubscriptionService) UpdateByID(
	ctx context.Context,
	id string,
	request dto.UpdateSubscriptionRequest,
) (dto.SubscriptionResponse, error) {
	subscriptionRecord, err := s.store.FindByIDUnscoped(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	return s.Update(ctx, subscriptionRecord.OrganizationID, subscriptionRecord.ID, request)
}

func (s *SubscriptionService) ChangeStatusByID(
	ctx context.Context,
	id string,
	status model.SubscriptionStatus,
	actorUserID string,
	reason string,
) (dto.SubscriptionResponse, error) {
	subscriptionRecord, err := s.store.FindByIDUnscoped(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	return s.ChangeStatus(ctx, subscriptionRecord.OrganizationID, subscriptionRecord.ID, status, actorUserID, reason)
}

func (s *SubscriptionService) ScheduleCancellation(
	ctx context.Context,
	organizationID string,
	id string,
	actorUserID string,
	reason string,
) (dto.SubscriptionResponse, error) {
	current, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(id))
	if err != nil {
		return dto.SubscriptionResponse{}, mapSubscriptionError(err)
	}
	if !current.IsUsable() {
		return dto.SubscriptionResponse{}, validationError("subscription is not eligible for scheduled cancellation")
	}
	if current.CancelAtPeriodEnd {
		return subscriptionResponse(current), nil
	}

	cancelAtPeriodEnd := true
	updated, err := s.updateWithActor(ctx, current.OrganizationID, current.ID, dto.UpdateSubscriptionRequest{
		CancelAtPeriodEnd: &cancelAtPeriodEnd,
		Reason:            scheduledCancellationReason(reason),
	}, actorUserID)
	if err != nil {
		return dto.SubscriptionResponse{}, err
	}
	return updated, nil
}

func (s *SubscriptionService) ActivateUpgradeByInvoice(
	ctx context.Context,
	invoice SubscriptionInvoice,
	paidAt time.Time,
) error {
	if !isUpgradeRequestMetadata(invoice.Metadata) {
		return nil
	}
	if invoice.SubscriptionID == nil || strings.TrimSpace(*invoice.SubscriptionID) == "" {
		return validationError("upgrade invoice subscription is required")
	}

	current, err := s.store.FindByID(
		ctx,
		strings.TrimSpace(invoice.OrganizationID),
		strings.TrimSpace(*invoice.SubscriptionID),
	)
	if err != nil {
		return mapSubscriptionError(err)
	}

	targetPlanID, ok := metadataStringValue(invoice.Metadata, "target_plan_id")
	if !ok {
		return validationError("upgrade invoice target plan is required")
	}
	interval := strings.TrimSpace(string(current.BillingInterval))
	if metadataInterval, ok := metadataStringValue(invoice.Metadata, "billing_interval"); ok {
		interval = metadataInterval
	}
	if interval == "" {
		return validationError("upgrade invoice billing interval is required")
	}

	shouldActivate := current.PlanID != targetPlanID ||
		!strings.EqualFold(string(current.BillingInterval), interval) ||
		current.Status != model.SubscriptionStatusActive ||
		current.CancelAtPeriodEnd
	if !shouldActivate {
		return nil
	}

	status := string(model.SubscriptionStatusActive)
	cancelAtPeriodEnd := false
	request := dto.UpdateSubscriptionRequest{
		PlanID:            &targetPlanID,
		Status:            &status,
		BillingInterval:   &interval,
		CancelAtPeriodEnd: &cancelAtPeriodEnd,
		Reason:            upgradeActivationReason(invoice),
	}
	if current.CurrentPeriodStart == nil {
		start := paidAt.UTC().Format(time.RFC3339)
		request.CurrentPeriodStart = &start
	}
	if current.CurrentPeriodEnd == nil {
		end := defaultPeriodEnd(paidAt.UTC(), interval).Format(time.RFC3339)
		request.CurrentPeriodEnd = &end
	}
	actorUserID, _ := metadataStringValue(invoice.Metadata, "requested_by_user_id")
	_, err = s.updateWithActor(ctx, current.OrganizationID, current.ID, request, actorUserID)
	return err
}

func (s *SubscriptionService) createParams(request dto.CreateSubscriptionRequest) (repository.CreateSubscriptionParams, error) {
	status := model.SubscriptionStatus(strings.TrimSpace(request.Status))
	if status == "" {
		status = model.SubscriptionStatusActive
	}
	if !status.IsValid() {
		return repository.CreateSubscriptionParams{}, validationError("subscription status is invalid")
	}
	interval := model.BillingInterval(strings.TrimSpace(request.BillingInterval))
	if !interval.IsValidSubscriptionInterval() {
		return repository.CreateSubscriptionParams{}, validationError("billing interval is invalid")
	}
	if strings.TrimSpace(request.OrganizationID) == "" {
		return repository.CreateSubscriptionParams{}, validationError("organization id is required")
	}
	if strings.TrimSpace(request.PlanID) == "" {
		return repository.CreateSubscriptionParams{}, validationError("plan id is required")
	}
	currentPeriodStart, err := parseOptionalTime(request.CurrentPeriodStart)
	if err != nil {
		return repository.CreateSubscriptionParams{}, err
	}
	currentPeriodEnd, err := parseOptionalTime(request.CurrentPeriodEnd)
	if err != nil {
		return repository.CreateSubscriptionParams{}, err
	}
	trialStart, err := parseOptionalTime(request.TrialStart)
	if err != nil {
		return repository.CreateSubscriptionParams{}, err
	}
	trialEnd, err := parseOptionalTime(request.TrialEnd)
	if err != nil {
		return repository.CreateSubscriptionParams{}, err
	}
	if status.IsUsable() && currentPeriodStart == nil {
		now := s.now().UTC()
		currentPeriodStart = &now
	}
	if status.IsUsable() && currentPeriodEnd == nil && currentPeriodStart != nil {
		currentPeriodEnd = defaultPeriodEnd(*currentPeriodStart, string(interval))
	}
	return repository.CreateSubscriptionParams{
		OrganizationID:     request.OrganizationID,
		PlanID:             request.PlanID,
		Status:             status,
		BillingInterval:    interval,
		CurrentPeriodStart: currentPeriodStart,
		CurrentPeriodEnd:   currentPeriodEnd,
		TrialStart:         trialStart,
		TrialEnd:           trialEnd,
		CancelAtPeriodEnd:  request.CancelAtPeriodEnd,
		Metadata:           request.Metadata,
	}, nil
}

func (s *SubscriptionService) updateParams(
	current model.Subscription,
	request dto.UpdateSubscriptionRequest,
) (repository.UpdateSubscriptionParams, error) {
	params := repository.UpdateSubscriptionParams{
		ID:                 current.ID,
		OrganizationID:     current.OrganizationID,
		PlanID:             trimOptionalString(request.PlanID),
		CancelAtPeriodEnd:  request.CancelAtPeriodEnd,
		Metadata:           request.Metadata,
		CurrentPeriodStart: current.CurrentPeriodStart,
		CurrentPeriodEnd:   current.CurrentPeriodEnd,
		TrialStart:         current.TrialStart,
		TrialEnd:           current.TrialEnd,
	}
	if request.Status != nil {
		status := model.SubscriptionStatus(strings.TrimSpace(*request.Status))
		if !status.IsValid() {
			return repository.UpdateSubscriptionParams{}, validationError("subscription status is invalid")
		}
		if !canTransitionSubscriptionStatus(current.Status, status) {
			return repository.UpdateSubscriptionParams{}, validationError("subscription status transition is invalid")
		}
		params.Status = &status
		now := s.now().UTC()
		switch status {
		case model.SubscriptionStatusCanceled, model.SubscriptionStatusExpired:
			params.CanceledAt = &now
		case model.SubscriptionStatusSuspended:
			params.SuspendedAt = &now
		case model.SubscriptionStatusActive, model.SubscriptionStatusTrialing, model.SubscriptionStatusPastDue, model.SubscriptionStatusGracePeriod:
			params.SuspendedAt = nil
		}
	}
	if request.BillingInterval != nil {
		interval := model.BillingInterval(strings.TrimSpace(*request.BillingInterval))
		if !interval.IsValidSubscriptionInterval() {
			return repository.UpdateSubscriptionParams{}, validationError("billing interval is invalid")
		}
		params.BillingInterval = &interval
	}
	var err error
	if request.CurrentPeriodStart != nil {
		params.CurrentPeriodStart, err = parseOptionalTime(request.CurrentPeriodStart)
		if err != nil {
			return repository.UpdateSubscriptionParams{}, err
		}
	}
	if request.CurrentPeriodEnd != nil {
		params.CurrentPeriodEnd, err = parseOptionalTime(request.CurrentPeriodEnd)
		if err != nil {
			return repository.UpdateSubscriptionParams{}, err
		}
	}
	if request.TrialStart != nil {
		params.TrialStart, err = parseOptionalTime(request.TrialStart)
		if err != nil {
			return repository.UpdateSubscriptionParams{}, err
		}
	}
	if request.TrialEnd != nil {
		params.TrialEnd, err = parseOptionalTime(request.TrialEnd)
		if err != nil {
			return repository.UpdateSubscriptionParams{}, err
		}
	}
	return params, nil
}

func (s *SubscriptionService) recordUpdateEvent(
	ctx context.Context,
	current model.Subscription,
	updated model.Subscription,
	actorUserID string,
	reason string,
) error {
	eventType := "subscription_updated"
	var oldStatus *model.SubscriptionStatus
	var newStatus *model.SubscriptionStatus
	metadata := map[string]any{"reason": strings.TrimSpace(reason)}
	if current.Status != updated.Status {
		eventType = "subscription_status_changed"
		oldStatus = &current.Status
		newStatus = &updated.Status
	}
	if current.PlanID != updated.PlanID {
		eventType = "subscription_plan_changed"
		metadata["old_plan_id"] = current.PlanID
		metadata["new_plan_id"] = updated.PlanID
	}
	_, err := s.store.CreateEvent(ctx, repository.SubscriptionEventParams{
		SubscriptionID: updated.ID,
		OrganizationID: updated.OrganizationID,
		Type:           eventType,
		OldStatus:      oldStatus,
		NewStatus:      newStatus,
		ActorUserID:    stringPointer(actorUserID),
		Metadata:       metadata,
	})
	return err
}

func (s *SubscriptionService) syncEntitlements(
	ctx context.Context,
	subscriptionRecord model.Subscription,
	actorUserID string,
	reason string,
) error {
	if s.entitlementStore == nil || s.entitlementSink == nil {
		return nil
	}
	entitlements, err := s.entitlementStore.ListByPlanID(ctx, subscriptionRecord.PlanID)
	if err != nil {
		return err
	}
	effectiveFrom := s.now().UTC()
	if subscriptionRecord.CurrentPeriodStart != nil {
		effectiveFrom = subscriptionRecord.CurrentPeriodStart.UTC()
	}
	_, err = s.entitlementSink.SyncPlanEntitlements(ctx, repository.SyncPlanEntitlementsParams{
		OrganizationID: subscriptionRecord.OrganizationID,
		SubscriptionID: subscriptionRecord.ID,
		Entitlements:   entitlements,
		EffectiveFrom:  effectiveFrom,
		EffectiveUntil: subscriptionRecord.CurrentPeriodEnd,
		ActorUserID:    actorUserID,
		Reason:         reason,
	})
	return err
}

func (s *SubscriptionService) expireEntitlements(
	ctx context.Context,
	subscriptionRecord model.Subscription,
	actorUserID string,
	reason string,
) error {
	if s.entitlementSink == nil {
		return nil
	}
	effectiveUntil := s.now().UTC()
	if subscriptionRecord.CurrentPeriodEnd != nil {
		effectiveUntil = subscriptionRecord.CurrentPeriodEnd.UTC()
	}
	_, err := s.entitlementSink.ExpirePlanEntitlements(
		ctx,
		subscriptionRecord.OrganizationID,
		subscriptionRecord.ID,
		effectiveUntil,
		actorUserID,
		reason,
	)
	return err
}

func subscriptionListFilter(query dto.SubscriptionListQuery) (repository.SubscriptionListFilter, int, int, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	filter := repository.SubscriptionListFilter{
		OrganizationID: strings.TrimSpace(query.OrganizationID),
		PlanID:         strings.TrimSpace(query.PlanID),
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}
	if strings.TrimSpace(query.Status) != "" {
		status := model.SubscriptionStatus(strings.TrimSpace(query.Status))
		if !status.IsValid() {
			return repository.SubscriptionListFilter{}, 0, 0, validationError("subscription status is invalid")
		}
		filter.Status = status
	}
	return filter, page, perPage, nil
}

func canTransitionSubscriptionStatus(from model.SubscriptionStatus, to model.SubscriptionStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case model.SubscriptionStatusTrialing:
		return to == model.SubscriptionStatusActive ||
			to == model.SubscriptionStatusSuspended ||
			to == model.SubscriptionStatusCanceled ||
			to == model.SubscriptionStatusExpired
	case model.SubscriptionStatusActive:
		return to == model.SubscriptionStatusPastDue ||
			to == model.SubscriptionStatusGracePeriod ||
			to == model.SubscriptionStatusSuspended ||
			to == model.SubscriptionStatusCanceled ||
			to == model.SubscriptionStatusExpired
	case model.SubscriptionStatusPastDue:
		return to == model.SubscriptionStatusActive ||
			to == model.SubscriptionStatusGracePeriod ||
			to == model.SubscriptionStatusSuspended ||
			to == model.SubscriptionStatusCanceled ||
			to == model.SubscriptionStatusExpired
	case model.SubscriptionStatusGracePeriod:
		return to == model.SubscriptionStatusActive ||
			to == model.SubscriptionStatusSuspended ||
			to == model.SubscriptionStatusCanceled ||
			to == model.SubscriptionStatusExpired
	case model.SubscriptionStatusSuspended:
		return to == model.SubscriptionStatusActive ||
			to == model.SubscriptionStatusCanceled ||
			to == model.SubscriptionStatusExpired
	default:
		return false
	}
}

func shouldSyncSubscriptionEntitlements(current model.Subscription, updated model.Subscription) bool {
	return updated.IsUsable() && (current.PlanID != updated.PlanID || !current.IsUsable())
}

func shouldExpireSubscriptionEntitlements(current model.Subscription, updated model.Subscription) bool {
	return current.IsUsable() && !updated.IsUsable()
}

func upgradeActivationReason(invoice SubscriptionInvoice) string {
	reason := "subscription upgraded after invoice payment"
	if requestedReason, ok := metadataStringValue(invoice.Metadata, "reason"); ok {
		reason = reason + ": " + requestedReason
	}
	return reason
}

func scheduledCancellationReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "subscription scheduled for cancellation at period end"
	}
	return "subscription scheduled for cancellation at period end: " + reason
}

func metadataStringValue(metadata map[string]any, key string) (string, bool) {
	if metadata == nil {
		return "", false
	}
	value, ok := metadata[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

func isUpgradeRequestMetadata(metadata map[string]any) bool {
	value, ok := metadataStringValue(metadata, "billing_action")
	return ok && value == "upgrade_request"
}
