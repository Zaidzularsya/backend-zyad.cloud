package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/platform/database"
)

const paymentSelectColumns = `
	id,
	invoice_id,
	organization_id,
	provider,
	COALESCE(provider_reference, ''),
	COALESCE(payment_method, ''),
	status,
	amount::text,
	currency,
	paid_at,
	raw_payload,
	created_at,
	updated_at
`

type PaymentRepository struct {
	db *database.Pool
}

type PaymentListFilter struct {
	OrganizationID string
	InvoiceID      string
	Provider       model.PaymentProvider
	Status         model.PaymentStatus
	Limit          int
	Offset         int
}

type CreatePaymentParams struct {
	InvoiceID         string
	OrganizationID    string
	Provider          model.PaymentProvider
	ProviderReference string
	PaymentMethod     string
	Status            model.PaymentStatus
	Amount            string
	Currency          string
	PaidAt            *time.Time
	RawPayload        map[string]any
}

type PaymentEventParams struct {
	PaymentID       *string
	InvoiceID       *string
	Provider        model.PaymentProvider
	Type            string
	ProviderEventID string
	Payload         map[string]any
	ProcessedAt     *time.Time
}

func NewPaymentRepository(db *database.Pool) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, params CreatePaymentParams) (model.Payment, error) {
	rawPayload, err := encodeMap(params.RawPayload)
	if err != nil {
		return model.Payment{}, err
	}
	status := params.Status
	if status == "" {
		status = model.PaymentStatusPending
	}
	var payment model.Payment
	var rawPayloadBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO billing_payments (
			invoice_id,
			organization_id,
			provider,
			provider_reference,
			payment_method,
			status,
			amount,
			currency,
			paid_at,
			raw_payload
		)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7::numeric, $8, $9, $10::jsonb)
		RETURNING `+paymentSelectColumns,
		strings.TrimSpace(params.InvoiceID),
		strings.TrimSpace(params.OrganizationID),
		string(params.Provider),
		strings.TrimSpace(params.ProviderReference),
		strings.TrimSpace(params.PaymentMethod),
		string(status),
		amountOrDefault(params.Amount),
		upperOrDefault(params.Currency, "IDR"),
		params.PaidAt,
		rawPayload,
	).Scan(paymentScanDest(&payment, &rawPayloadBytes)...)
	if err != nil {
		return model.Payment{}, err
	}
	if err := decodeMap(rawPayloadBytes, &payment.RawPayload); err != nil {
		return model.Payment{}, err
	}
	return payment, nil
}

func (r *PaymentRepository) FindByID(ctx context.Context, organizationID string, id string) (model.Payment, error) {
	var payment model.Payment
	var rawPayloadBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+paymentSelectColumns+`
		FROM billing_payments
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(paymentScanDest(&payment, &rawPayloadBytes)...)
	if err != nil {
		return model.Payment{}, err
	}
	if err := decodeMap(rawPayloadBytes, &payment.RawPayload); err != nil {
		return model.Payment{}, err
	}
	return payment, nil
}

func (r *PaymentRepository) FindEventByProviderEventID(
	ctx context.Context,
	provider model.PaymentProvider,
	providerEventID string,
) (model.PaymentEvent, error) {
	var event model.PaymentEvent
	var payloadBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			payment_id,
			invoice_id,
			provider,
			event_type,
			COALESCE(provider_event_id, ''),
			payload,
			processed_at,
			created_at
		FROM billing_payment_events
		WHERE provider = $1
			AND provider_event_id = $2
	`, string(provider), strings.TrimSpace(providerEventID)).Scan(
		&event.ID,
		&event.PaymentID,
		&event.InvoiceID,
		&event.Provider,
		&event.Type,
		&event.ProviderEventID,
		&payloadBytes,
		&event.ProcessedAt,
		&event.CreatedAt,
	)
	if err != nil {
		return model.PaymentEvent{}, err
	}
	if err := decodeMap(payloadBytes, &event.Payload); err != nil {
		return model.PaymentEvent{}, err
	}
	return event, nil
}

func (r *PaymentRepository) List(ctx context.Context, filter PaymentListFilter) ([]model.Payment, int64, error) {
	where, args := paymentWhere(filter)
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM billing_payments"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+paymentSelectColumns+`
		FROM billing_payments`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	payments := make([]model.Payment, 0)
	for rows.Next() {
		var payment model.Payment
		var rawPayloadBytes []byte
		if err := rows.Scan(paymentScanDest(&payment, &rawPayloadBytes)...); err != nil {
			return nil, 0, err
		}
		if err := decodeMap(rawPayloadBytes, &payment.RawPayload); err != nil {
			return nil, 0, err
		}
		payments = append(payments, payment)
	}
	return payments, total, rows.Err()
}

func (r *PaymentRepository) CreateEvent(ctx context.Context, params PaymentEventParams) (model.PaymentEvent, error) {
	payload, err := encodeMap(params.Payload)
	if err != nil {
		return model.PaymentEvent{}, err
	}
	var event model.PaymentEvent
	var payloadBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO billing_payment_events (
			payment_id,
			invoice_id,
			provider,
			event_type,
			provider_event_id,
			payload,
			processed_at
		)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4, NULLIF($5, ''), $6::jsonb, $7)
		RETURNING
			id,
			payment_id,
			invoice_id,
			provider,
			event_type,
			COALESCE(provider_event_id, ''),
			payload,
			processed_at,
			created_at
	`,
		stringPointerValue(params.PaymentID),
		stringPointerValue(params.InvoiceID),
		string(params.Provider),
		canonicalKey(params.Type),
		strings.TrimSpace(params.ProviderEventID),
		payload,
		params.ProcessedAt,
	).Scan(
		&event.ID,
		&event.PaymentID,
		&event.InvoiceID,
		&event.Provider,
		&event.Type,
		&event.ProviderEventID,
		&payloadBytes,
		&event.ProcessedAt,
		&event.CreatedAt,
	)
	if err != nil {
		return model.PaymentEvent{}, err
	}
	if err := decodeMap(payloadBytes, &event.Payload); err != nil {
		return model.PaymentEvent{}, err
	}
	return event, nil
}

func paymentWhere(filter PaymentListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if filter.OrganizationID != "" {
		args = append(args, strings.TrimSpace(filter.OrganizationID))
		conditions = append(conditions, fmt.Sprintf("organization_id = $%d::uuid", len(args)))
	}
	if filter.InvoiceID != "" {
		args = append(args, strings.TrimSpace(filter.InvoiceID))
		conditions = append(conditions, fmt.Sprintf("invoice_id = $%d::uuid", len(args)))
	}
	if filter.Provider != "" {
		args = append(args, string(filter.Provider))
		conditions = append(conditions, fmt.Sprintf("provider = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func paymentScanDest(payment *model.Payment, rawPayload *[]byte) []any {
	return []any{
		&payment.ID,
		&payment.InvoiceID,
		&payment.OrganizationID,
		&payment.Provider,
		&payment.ProviderReference,
		&payment.PaymentMethod,
		&payment.Status,
		&payment.Amount,
		&payment.Currency,
		&payment.PaidAt,
		rawPayload,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	}
}
