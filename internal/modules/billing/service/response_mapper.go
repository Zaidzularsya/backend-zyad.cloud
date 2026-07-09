package service

import (
	"time"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
)

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
