package dto

import (
	"time"

	"zyad.cloud/internal/modules/crm/domain"
)

// PaginationMeta wraps database query totals and counts — mirrors
// internal/modules/landing/dto.PaginationMeta.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// BuildMeta computes pagination metadata returned as the sibling "meta"
// field of the response envelope (not nested inside "data") — matching
// frontend/docs/architecture.md's documented envelope contract
// {data, meta}, which frontend/src/lib/api-client.ts and
// frontend/src/types/api.ts (ApiEnvelope<T>) are built around.
func BuildMeta(page, perPage int, total int64) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}

type CompanyResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Industry    string         `json:"industry,omitempty"`
	Website     string         `json:"website,omitempty"`
	Phone       string         `json:"phone,omitempty"`
	Email       string         `json:"email,omitempty"`
	Address     map[string]any `json:"address"`
	SizeRange   string         `json:"size_range,omitempty"`
	Notes       string         `json:"notes,omitempty"`
	Tags        []string       `json:"tags"`
	OwnerUserID string         `json:"owner_user_id,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   *time.Time     `json:"deleted_at,omitempty"`
}

func CompanyFromDomain(c domain.Company) CompanyResponse {
	return CompanyResponse{
		ID:          c.ID,
		Name:        c.Name,
		Industry:    c.Industry,
		Website:     c.Website,
		Phone:       c.Phone,
		Email:       c.Email,
		Address:     c.Address,
		SizeRange:   c.SizeRange,
		Notes:       c.Notes,
		Tags:        c.Tags,
		OwnerUserID: c.OwnerUserID,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		DeletedAt:   c.DeletedAt,
	}
}

func CompanyListFromDomain(companies []domain.Company) []CompanyResponse {
	items := make([]CompanyResponse, 0, len(companies))
	for _, c := range companies {
		items = append(items, CompanyFromDomain(c))
	}
	return items
}

type ContactResponse struct {
	ID             string         `json:"id"`
	CompanyID      *string        `json:"company_id,omitempty"`
	FirstName      string         `json:"first_name"`
	LastName       string         `json:"last_name,omitempty"`
	Email          string         `json:"email,omitempty"`
	Phone          string         `json:"phone,omitempty"`
	JobTitle       string         `json:"job_title,omitempty"`
	Address        map[string]any `json:"address"`
	Tags           []string       `json:"tags"`
	Source         string         `json:"source,omitempty"`
	OwnerUserID    string         `json:"owner_user_id,omitempty"`
	IsCustomer     bool           `json:"is_customer"`
	LifecycleStage string         `json:"lifecycle_stage"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      *time.Time     `json:"deleted_at,omitempty"`
}

func ContactFromDomain(c domain.Contact) ContactResponse {
	return ContactResponse{
		ID:             c.ID,
		CompanyID:      c.CompanyID,
		FirstName:      c.FirstName,
		LastName:       c.LastName,
		Email:          c.Email,
		Phone:          c.Phone,
		JobTitle:       c.JobTitle,
		Address:        c.Address,
		Tags:           c.Tags,
		Source:         c.Source,
		OwnerUserID:    c.OwnerUserID,
		IsCustomer:     c.IsCustomer,
		LifecycleStage: string(c.LifecycleStage),
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
		DeletedAt:      c.DeletedAt,
	}
}

func ContactListFromDomain(contacts []domain.Contact) []ContactResponse {
	items := make([]ContactResponse, 0, len(contacts))
	for _, c := range contacts {
		items = append(items, ContactFromDomain(c))
	}
	return items
}

type LeadResponse struct {
	ID                 string     `json:"id"`
	ContactName        string     `json:"contact_name"`
	CompanyName        string     `json:"company_name,omitempty"`
	Email              string     `json:"email,omitempty"`
	Phone              string     `json:"phone,omitempty"`
	Source             string     `json:"source,omitempty"`
	Status             string     `json:"status"`
	Score              int        `json:"score"`
	OwnerUserID        string     `json:"owner_user_id,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	ConvertedContactID *string    `json:"converted_contact_id,omitempty"`
	ConvertedCompanyID *string    `json:"converted_company_id,omitempty"`
	ConvertedDealID    *string    `json:"converted_deal_id,omitempty"`
	ConvertedAt        *time.Time `json:"converted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

func LeadFromDomain(l domain.Lead) LeadResponse {
	return LeadResponse{
		ID:                 l.ID,
		ContactName:        l.ContactName,
		CompanyName:        l.CompanyName,
		Email:              l.Email,
		Phone:              l.Phone,
		Source:             l.Source,
		Status:             string(l.Status),
		Score:              l.Score,
		OwnerUserID:        l.OwnerUserID,
		Notes:              l.Notes,
		ConvertedContactID: l.ConvertedContactID,
		ConvertedCompanyID: l.ConvertedCompanyID,
		ConvertedDealID:    l.ConvertedDealID,
		ConvertedAt:        l.ConvertedAt,
		CreatedAt:          l.CreatedAt,
		UpdatedAt:          l.UpdatedAt,
		DeletedAt:          l.DeletedAt,
	}
}

func LeadListFromDomain(leads []domain.Lead) []LeadResponse {
	items := make([]LeadResponse, 0, len(leads))
	for _, l := range leads {
		items = append(items, LeadFromDomain(l))
	}
	return items
}

type PipelineStageResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Probability string `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
}

type PipelineResponse struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	IsDefault  bool                    `json:"is_default"`
	ArchivedAt *time.Time              `json:"archived_at,omitempty"`
	Stages     []PipelineStageResponse `json:"stages"`
	CreatedAt  time.Time               `json:"created_at"`
	UpdatedAt  time.Time               `json:"updated_at"`
	DeletedAt  *time.Time              `json:"deleted_at,omitempty"`
}

func PipelineFromDomain(p domain.Pipeline) PipelineResponse {
	stages := make([]PipelineStageResponse, 0, len(p.Stages))
	for _, s := range p.Stages {
		stages = append(stages, PipelineStageResponse{
			ID:          s.ID,
			Name:        s.Name,
			Position:    s.Position,
			Probability: s.Probability,
			IsWon:       s.IsWon,
			IsLost:      s.IsLost,
		})
	}
	return PipelineResponse{
		ID:         p.ID,
		Name:       p.Name,
		IsDefault:  p.IsDefault,
		ArchivedAt: p.ArchivedAt,
		Stages:     stages,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
		DeletedAt:  p.DeletedAt,
	}
}

func PipelineListFromDomain(pipelines []domain.Pipeline) []PipelineResponse {
	items := make([]PipelineResponse, 0, len(pipelines))
	for _, p := range pipelines {
		items = append(items, PipelineFromDomain(p))
	}
	return items
}

type DealResponse struct {
	ID                 string     `json:"id"`
	PipelineID         string     `json:"pipeline_id"`
	StageID            string     `json:"stage_id"`
	CompanyID          *string    `json:"company_id,omitempty"`
	ContactID          *string    `json:"contact_id,omitempty"`
	Title              string     `json:"title"`
	Value              string     `json:"value"`
	Currency           string     `json:"currency"`
	ExpectedCloseDate  *time.Time `json:"expected_close_date,omitempty"`
	Status             string     `json:"status"`
	LostReason         string     `json:"lost_reason,omitempty"`
	OwnerUserID        string     `json:"owner_user_id,omitempty"`
	DiscountPercent    *string    `json:"discount_percent,omitempty"`
	DiscountApprovedBy string     `json:"discount_approved_by,omitempty"`
	DiscountApprovedAt *time.Time `json:"discount_approved_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

func DealFromDomain(d domain.Deal) DealResponse {
	return DealResponse{
		ID:                 d.ID,
		PipelineID:         d.PipelineID,
		StageID:            d.StageID,
		CompanyID:          d.CompanyID,
		ContactID:          d.ContactID,
		Title:              d.Title,
		Value:              d.Value,
		Currency:           d.Currency,
		ExpectedCloseDate:  d.ExpectedCloseDate,
		Status:             string(d.Status),
		LostReason:         d.LostReason,
		OwnerUserID:        d.OwnerUserID,
		DiscountPercent:    d.DiscountPercent,
		DiscountApprovedBy: d.DiscountApprovedBy,
		DiscountApprovedAt: d.DiscountApprovedAt,
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
		DeletedAt:          d.DeletedAt,
	}
}

func DealListFromDomain(deals []domain.Deal) []DealResponse {
	items := make([]DealResponse, 0, len(deals))
	for _, d := range deals {
		items = append(items, DealFromDomain(d))
	}
	return items
}

type ActivityResponse struct {
	ID                string     `json:"id"`
	RelatedEntityType string     `json:"related_entity_type"`
	RelatedEntityID   string     `json:"related_entity_id"`
	Type              string     `json:"type"`
	Subject           string     `json:"subject"`
	Description       string     `json:"description,omitempty"`
	DueAt             *time.Time `json:"due_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	Status            string     `json:"status"`
	AssigneeUserID    string     `json:"assignee_user_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
}

func ActivityFromDomain(a domain.Activity) ActivityResponse {
	return ActivityResponse{
		ID:                a.ID,
		RelatedEntityType: string(a.RelatedEntityType),
		RelatedEntityID:   a.RelatedEntityID,
		Type:              string(a.Type),
		Subject:           a.Subject,
		Description:       a.Description,
		DueAt:             a.DueAt,
		CompletedAt:       a.CompletedAt,
		Status:            string(a.Status),
		AssigneeUserID:    a.AssigneeUserID,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
		DeletedAt:         a.DeletedAt,
	}
}

func ActivityListFromDomain(activities []domain.Activity) []ActivityResponse {
	items := make([]ActivityResponse, 0, len(activities))
	for _, a := range activities {
		items = append(items, ActivityFromDomain(a))
	}
	return items
}

type LineItemResponse struct {
	ID              string  `json:"id"`
	Description     string  `json:"description"`
	Quantity        string  `json:"quantity"`
	UnitPrice       string  `json:"unit_price"`
	DiscountPercent *string `json:"discount_percent,omitempty"`
	LineTotal       string  `json:"line_total"`
	Position        int     `json:"position"`
}

type QuotationResponse struct {
	ID              string             `json:"id"`
	DealID          *string            `json:"deal_id,omitempty"`
	ContactID       *string            `json:"contact_id,omitempty"`
	CompanyID       *string            `json:"company_id,omitempty"`
	QuotationNumber string             `json:"quotation_number"`
	Status          string             `json:"status"`
	ValidUntil      *time.Time         `json:"valid_until,omitempty"`
	Subtotal        string             `json:"subtotal"`
	DiscountTotal   string             `json:"discount_total"`
	TaxTotal        string             `json:"tax_total"`
	GrandTotal      string             `json:"grand_total"`
	Currency        string             `json:"currency"`
	Notes           string             `json:"notes,omitempty"`
	SentAt          *time.Time         `json:"sent_at,omitempty"`
	ApprovedAt      *time.Time         `json:"approved_at,omitempty"`
	RejectedAt      *time.Time         `json:"rejected_at,omitempty"`
	Items           []LineItemResponse `json:"items"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	DeletedAt       *time.Time         `json:"deleted_at,omitempty"`
}

func quotationItemsFromDomain(items []domain.QuotationItem) []LineItemResponse {
	responses := make([]LineItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, LineItemResponse{
			ID:              item.ID,
			Description:     item.Description,
			Quantity:        item.Quantity,
			UnitPrice:       item.UnitPrice,
			DiscountPercent: item.DiscountPercent,
			LineTotal:       item.LineTotal,
			Position:        item.Position,
		})
	}
	return responses
}

func QuotationFromDomain(q domain.Quotation) QuotationResponse {
	return QuotationResponse{
		ID:              q.ID,
		DealID:          q.DealID,
		ContactID:       q.ContactID,
		CompanyID:       q.CompanyID,
		QuotationNumber: q.QuotationNumber,
		Status:          string(q.Status),
		ValidUntil:      q.ValidUntil,
		Subtotal:        q.Subtotal,
		DiscountTotal:   q.DiscountTotal,
		TaxTotal:        q.TaxTotal,
		GrandTotal:      q.GrandTotal,
		Currency:        q.Currency,
		Notes:           q.Notes,
		SentAt:          q.SentAt,
		ApprovedAt:      q.ApprovedAt,
		RejectedAt:      q.RejectedAt,
		Items:           quotationItemsFromDomain(q.Items),
		CreatedAt:       q.CreatedAt,
		UpdatedAt:       q.UpdatedAt,
		DeletedAt:       q.DeletedAt,
	}
}

func QuotationListFromDomain(quotations []domain.Quotation) []QuotationResponse {
	items := make([]QuotationResponse, 0, len(quotations))
	for _, q := range quotations {
		items = append(items, QuotationFromDomain(q))
	}
	return items
}

type InvoiceResponse struct {
	ID            string             `json:"id"`
	QuotationID   *string            `json:"quotation_id,omitempty"`
	DealID        *string            `json:"deal_id,omitempty"`
	ContactID     *string            `json:"contact_id,omitempty"`
	CompanyID     *string            `json:"company_id,omitempty"`
	InvoiceNumber string             `json:"invoice_number"`
	Status        string             `json:"status"`
	IssueDate     *time.Time         `json:"issue_date,omitempty"`
	DueDate       *time.Time         `json:"due_date,omitempty"`
	Subtotal      string             `json:"subtotal"`
	TaxTotal      string             `json:"tax_total"`
	GrandTotal    string             `json:"grand_total"`
	AmountPaid    string             `json:"amount_paid"`
	PaidAt        *time.Time         `json:"paid_at,omitempty"`
	Currency      string             `json:"currency"`
	Items         []LineItemResponse `json:"items"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	DeletedAt     *time.Time         `json:"deleted_at,omitempty"`
}

func invoiceItemsFromDomain(items []domain.InvoiceItem) []LineItemResponse {
	responses := make([]LineItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, LineItemResponse{
			ID:              item.ID,
			Description:     item.Description,
			Quantity:        item.Quantity,
			UnitPrice:       item.UnitPrice,
			DiscountPercent: item.DiscountPercent,
			LineTotal:       item.LineTotal,
			Position:        item.Position,
		})
	}
	return responses
}

func InvoiceFromDomain(inv domain.Invoice) InvoiceResponse {
	return InvoiceResponse{
		ID:            inv.ID,
		QuotationID:   inv.QuotationID,
		DealID:        inv.DealID,
		ContactID:     inv.ContactID,
		CompanyID:     inv.CompanyID,
		InvoiceNumber: inv.InvoiceNumber,
		Status:        string(inv.Status),
		IssueDate:     inv.IssueDate,
		DueDate:       inv.DueDate,
		Subtotal:      inv.Subtotal,
		TaxTotal:      inv.TaxTotal,
		GrandTotal:    inv.GrandTotal,
		AmountPaid:    inv.AmountPaid,
		PaidAt:        inv.PaidAt,
		Currency:      inv.Currency,
		Items:         invoiceItemsFromDomain(inv.Items),
		CreatedAt:     inv.CreatedAt,
		UpdatedAt:     inv.UpdatedAt,
		DeletedAt:     inv.DeletedAt,
	}
}

func InvoiceListFromDomain(invoices []domain.Invoice) []InvoiceResponse {
	items := make([]InvoiceResponse, 0, len(invoices))
	for _, inv := range invoices {
		items = append(items, InvoiceFromDomain(inv))
	}
	return items
}

// IntegrationResponse deliberately has no secret field — the encrypted
// secret is never serialized in normal responses. Only
// IntegrationSecretResponse (returned from GET .../integrations/:id/secret,
// gated by integration.view_secret) carries plaintext.
type IntegrationResponse struct {
	ID           string         `json:"id"`
	Provider     string         `json:"provider"`
	Name         string         `json:"name"`
	Config       map[string]any `json:"config"`
	HasSecret    bool           `json:"has_secret"`
	IsActive     bool           `json:"is_active"`
	ConnectedAt  *time.Time     `json:"connected_at,omitempty"`
	LastSyncedAt *time.Time     `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}

func IntegrationFromDomain(integ domain.Integration) IntegrationResponse {
	return IntegrationResponse{
		ID:           integ.ID,
		Provider:     string(integ.Provider),
		Name:         integ.Name,
		Config:       integ.Config,
		HasSecret:    integ.SecretEncrypted != "",
		IsActive:     integ.IsActive,
		ConnectedAt:  integ.ConnectedAt,
		LastSyncedAt: integ.LastSyncedAt,
		CreatedAt:    integ.CreatedAt,
		UpdatedAt:    integ.UpdatedAt,
		DeletedAt:    integ.DeletedAt,
	}
}

func IntegrationListFromDomain(integrations []domain.Integration) []IntegrationResponse {
	items := make([]IntegrationResponse, 0, len(integrations))
	for _, integ := range integrations {
		items = append(items, IntegrationFromDomain(integ))
	}
	return items
}

// IntegrationSecretResponse carries the one place plaintext ever appears.
type IntegrationSecretResponse struct {
	Secret string `json:"secret"`
}

type LeadConversionResponse struct {
	Lead    LeadResponse     `json:"lead"`
	Contact ContactResponse  `json:"contact"`
	Company *CompanyResponse `json:"company,omitempty"`
}

func LeadConversionFromDomain(result domain.LeadConversionResult) LeadConversionResponse {
	resp := LeadConversionResponse{
		Lead:    LeadFromDomain(result.Lead),
		Contact: ContactFromDomain(result.Contact),
	}
	if result.Company != nil {
		companyResp := CompanyFromDomain(*result.Company)
		resp.Company = &companyResp
	}
	return resp
}
