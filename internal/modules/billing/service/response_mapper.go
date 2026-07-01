package service

import (
	"time"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
)

func planResponse(plan model.Plan, prices []model.PlanPrice) dto.PlanResponse {
	return dto.PlanResponse{
		ID:          plan.ID,
		Code:        plan.Code,
		Name:        plan.Name,
		Description: plan.Description,
		PlanType:    string(plan.Type),
		IsPublic:    plan.IsPublic,
		IsActive:    plan.IsActive,
		SortOrder:   plan.SortOrder,
		Metadata:    mapOrEmpty(plan.Metadata),
		Prices:      planPriceResponses(prices),
		CreatedAt:   formatTime(plan.CreatedAt),
		UpdatedAt:   formatTime(plan.UpdatedAt),
		DeletedAt:   formatOptionalTime(plan.DeletedAt),
	}
}

func planPriceResponse(price model.PlanPrice) dto.PlanPriceResponse {
	return dto.PlanPriceResponse{
		ID:              price.ID,
		PlanID:          price.PlanID,
		BillingInterval: string(price.BillingInterval),
		Currency:        price.Currency,
		Amount:          price.Amount,
		IsActive:        price.IsActive,
		Metadata:        mapOrEmpty(price.Metadata),
		CreatedAt:       formatTime(price.CreatedAt),
		UpdatedAt:       formatTime(price.UpdatedAt),
		DeletedAt:       formatOptionalTime(price.DeletedAt),
	}
}

func planPriceResponses(prices []model.PlanPrice) []dto.PlanPriceResponse {
	items := make([]dto.PlanPriceResponse, 0, len(prices))
	for _, price := range prices {
		items = append(items, planPriceResponse(price))
	}
	return items
}

func featureResponse(feature model.Feature) dto.FeatureResponse {
	return dto.FeatureResponse{
		ID:            feature.ID,
		FeatureKey:    feature.Key,
		Module:        feature.Module,
		Name:          feature.Name,
		Description:   feature.Description,
		ValueType:     string(feature.ValueType),
		Unit:          feature.Unit,
		ResetStrategy: string(feature.ResetStrategy),
		IsActive:      feature.IsActive,
		CreatedAt:     formatTime(feature.CreatedAt),
		UpdatedAt:     formatTime(feature.UpdatedAt),
	}
}

func featureResponses(features []model.Feature) []dto.FeatureResponse {
	items := make([]dto.FeatureResponse, 0, len(features))
	for _, feature := range features {
		items = append(items, featureResponse(feature))
	}
	return items
}

func planEntitlementResponse(entitlement model.PlanEntitlement) dto.PlanEntitlementResponse {
	return dto.PlanEntitlementResponse{
		ID:           entitlement.ID,
		PlanID:       entitlement.PlanID,
		FeatureID:    entitlement.FeatureID,
		FeatureKey:   entitlement.FeatureKey,
		ValueBool:    entitlement.ValueBool,
		ValueInt:     entitlement.ValueInt,
		ValueDecimal: entitlement.ValueDecimal,
		ValueString:  entitlement.ValueString,
		Limits:       mapOrEmpty(entitlement.Limits),
		CreatedAt:    formatTime(entitlement.CreatedAt),
		UpdatedAt:    formatTime(entitlement.UpdatedAt),
	}
}

func planEntitlementResponses(entitlements []model.PlanEntitlement) []dto.PlanEntitlementResponse {
	items := make([]dto.PlanEntitlementResponse, 0, len(entitlements))
	for _, entitlement := range entitlements {
		items = append(items, planEntitlementResponse(entitlement))
	}
	return items
}

func subscriptionResponse(subscription model.Subscription, plan *model.Plan) dto.SubscriptionResponse {
	response := dto.SubscriptionResponse{
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
	if plan != nil {
		planValue := planResponse(*plan, nil)
		response.Plan = &planValue
	}
	return response
}

func subscriptionResponses(subscriptions []model.Subscription) []dto.SubscriptionResponse {
	items := make([]dto.SubscriptionResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		items = append(items, subscriptionResponse(subscription, nil))
	}
	return items
}

func invoiceResponse(invoice model.Invoice, items []model.InvoiceItem) dto.InvoiceResponse {
	return dto.InvoiceResponse{
		ID:             invoice.ID,
		OrganizationID: invoice.OrganizationID,
		SubscriptionID: invoice.SubscriptionID,
		InvoiceNumber:  invoice.InvoiceNumber,
		Status:         string(invoice.Status),
		Currency:       invoice.Currency,
		SubtotalAmount: invoice.SubtotalAmount,
		DiscountAmount: invoice.DiscountAmount,
		TaxAmount:      invoice.TaxAmount,
		TotalAmount:    invoice.TotalAmount,
		DueDate:        formatOptionalTime(invoice.DueDate),
		PaidAt:         formatOptionalTime(invoice.PaidAt),
		Items:          invoiceItemResponses(items),
		Metadata:       mapOrEmpty(invoice.Metadata),
		CreatedAt:      formatTime(invoice.CreatedAt),
		UpdatedAt:      formatTime(invoice.UpdatedAt),
	}
}

func invoiceResponses(invoices []model.Invoice) []dto.InvoiceResponse {
	items := make([]dto.InvoiceResponse, 0, len(invoices))
	for _, invoice := range invoices {
		items = append(items, invoiceResponse(invoice, nil))
	}
	return items
}

func invoiceItemResponse(item model.InvoiceItem) dto.InvoiceItemResponse {
	return dto.InvoiceItemResponse{
		ID:          item.ID,
		InvoiceID:   item.InvoiceID,
		ItemType:    string(item.Type),
		Description: item.Description,
		Quantity:    item.Quantity,
		UnitAmount:  item.UnitAmount,
		TotalAmount: item.TotalAmount,
		Metadata:    mapOrEmpty(item.Metadata),
		CreatedAt:   formatTime(item.CreatedAt),
		UpdatedAt:   formatTime(item.UpdatedAt),
	}
}

func invoiceItemResponses(items []model.InvoiceItem) []dto.InvoiceItemResponse {
	responses := make([]dto.InvoiceItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, invoiceItemResponse(item))
	}
	return responses
}

func paymentResponse(payment model.Payment) dto.PaymentResponse {
	return dto.PaymentResponse{
		ID:                payment.ID,
		InvoiceID:         payment.InvoiceID,
		OrganizationID:    payment.OrganizationID,
		Provider:          string(payment.Provider),
		ProviderReference: payment.ProviderReference,
		PaymentMethod:     payment.PaymentMethod,
		Status:            string(payment.Status),
		Amount:            payment.Amount,
		Currency:          payment.Currency,
		PaidAt:            formatOptionalTime(payment.PaidAt),
		RawPayload:        mapOrEmpty(payment.RawPayload),
		CreatedAt:         formatTime(payment.CreatedAt),
		UpdatedAt:         formatTime(payment.UpdatedAt),
	}
}

func paymentEventResponse(event model.PaymentEvent) dto.PaymentEventResponse {
	return dto.PaymentEventResponse{
		ID:              event.ID,
		PaymentID:       event.PaymentID,
		InvoiceID:       event.InvoiceID,
		Provider:        string(event.Provider),
		EventType:       event.Type,
		ProviderEventID: event.ProviderEventID,
		Payload:         mapOrEmpty(event.Payload),
		ProcessedAt:     formatOptionalTime(event.ProcessedAt),
		CreatedAt:       formatTime(event.CreatedAt),
	}
}

func paginationMeta(page, perPage int, total int64) dto.PaginationMeta {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return dto.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}

func mapOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}
