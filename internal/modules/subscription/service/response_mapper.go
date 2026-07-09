package service

import (
	"zyad.cloud/internal/modules/subscription/dto"
	"zyad.cloud/internal/modules/subscription/model"
)

func subscriptionResponse(subscription model.Subscription) dto.SubscriptionResponse {
	return dto.SubscriptionResponse{
		ID:                 subscription.ID,
		OrganizationID:     subscription.OrganizationID,
		PlanID:             subscription.PlanID,
		Status:             string(subscription.Status),
		BillingInterval:    string(subscription.BillingInterval),
		CurrentPeriodStart: formatOptionalTime(subscription.CurrentPeriodStart),
		CurrentPeriodEnd:   formatOptionalTime(subscription.CurrentPeriodEnd),
		TrialStart:         formatOptionalTime(subscription.TrialStart),
		TrialEnd:           formatOptionalTime(subscription.TrialEnd),
		CancelAtPeriodEnd:  subscription.CancelAtPeriodEnd,
		CanceledAt:         formatOptionalTime(subscription.CanceledAt),
		SuspendedAt:        formatOptionalTime(subscription.SuspendedAt),
		Metadata:           mapOrEmpty(subscription.Metadata),
		CreatedAt:          formatTime(subscription.CreatedAt),
		UpdatedAt:          formatTime(subscription.UpdatedAt),
	}
}

func subscriptionResponses(subscriptions []model.Subscription) []dto.SubscriptionResponse {
	items := make([]dto.SubscriptionResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		items = append(items, subscriptionResponse(subscription))
	}
	return items
}
