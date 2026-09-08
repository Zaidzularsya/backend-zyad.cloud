package dto

// CompanyListQuery binds query parameters for listing companies.
type CompanyListQuery struct {
	Page        int    `form:"page"`
	PerPage     int    `form:"per_page"`
	Search      string `form:"search"`
	OwnerUserID string `form:"owner_user_id"`
}

// CreateCompanyRequest binds the payload for creating a company.
type CreateCompanyRequest struct {
	Name        string         `json:"name" binding:"required,min=1,max=200"`
	Industry    string         `json:"industry"`
	Website     string         `json:"website"`
	Phone       string         `json:"phone"`
	Email       string         `json:"email" binding:"omitempty,email"`
	Address     map[string]any `json:"address"`
	SizeRange   string         `json:"size_range"`
	Notes       string         `json:"notes"`
	Tags        []string       `json:"tags"`
	OwnerUserID string         `json:"owner_user_id"`
}

// UpdateCompanyRequest binds the payload for updating a company. All fields
// are optional pointers; only fields present are applied.
type UpdateCompanyRequest struct {
	Name        *string        `json:"name"`
	Industry    *string        `json:"industry"`
	Website     *string        `json:"website"`
	Phone       *string        `json:"phone"`
	Email       *string        `json:"email"`
	Address     map[string]any `json:"address"`
	SizeRange   *string        `json:"size_range"`
	Notes       *string        `json:"notes"`
	Tags        []string       `json:"tags"`
	OwnerUserID *string        `json:"owner_user_id"`
}

// ContactListQuery binds query parameters for listing contacts.
type ContactListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Search         string `form:"search"`
	CompanyID      string `form:"company_id"`
	OwnerUserID    string `form:"owner_user_id"`
	LifecycleStage string `form:"lifecycle_stage"`
	IsCustomer     *bool  `form:"is_customer"`
}

// CreateContactRequest binds the payload for creating a contact.
type CreateContactRequest struct {
	CompanyID      string         `json:"company_id"`
	FirstName      string         `json:"first_name" binding:"required,min=1,max=120"`
	LastName       string         `json:"last_name"`
	Email          string         `json:"email" binding:"omitempty,email"`
	Phone          string         `json:"phone"`
	JobTitle       string         `json:"job_title"`
	Address        map[string]any `json:"address"`
	Tags           []string       `json:"tags"`
	Source         string         `json:"source"`
	OwnerUserID    string         `json:"owner_user_id"`
	LifecycleStage string         `json:"lifecycle_stage"`
}

// UpdateContactRequest binds the payload for updating a contact.
type UpdateContactRequest struct {
	CompanyID      *string        `json:"company_id"`
	FirstName      *string        `json:"first_name"`
	LastName       *string        `json:"last_name"`
	Email          *string        `json:"email"`
	Phone          *string        `json:"phone"`
	JobTitle       *string        `json:"job_title"`
	Address        map[string]any `json:"address"`
	Tags           []string       `json:"tags"`
	Source         *string        `json:"source"`
	OwnerUserID    *string        `json:"owner_user_id"`
	IsCustomer     *bool          `json:"is_customer"`
	LifecycleStage *string        `json:"lifecycle_stage"`
}

// LeadListQuery binds query parameters for listing leads.
type LeadListQuery struct {
	Page        int    `form:"page"`
	PerPage     int    `form:"per_page"`
	Search      string `form:"search"`
	Status      string `form:"status"`
	OwnerUserID string `form:"owner_user_id"`
}

// CreateLeadRequest binds the payload for creating a lead.
type CreateLeadRequest struct {
	ContactName string `json:"contact_name" binding:"required,min=1,max=200"`
	CompanyName string `json:"company_name"`
	Email       string `json:"email" binding:"omitempty,email"`
	Phone       string `json:"phone"`
	Source      string `json:"source"`
	Score       int    `json:"score" binding:"omitempty,min=0,max=100"`
	OwnerUserID string `json:"owner_user_id"`
	Notes       string `json:"notes"`
}

// UpdateLeadRequest binds the payload for updating a lead.
type UpdateLeadRequest struct {
	ContactName *string `json:"contact_name"`
	CompanyName *string `json:"company_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Source      *string `json:"source"`
	Status      *string `json:"status"`
	Score       *int    `json:"score"`
	OwnerUserID *string `json:"owner_user_id"`
	Notes       *string `json:"notes"`
}

// AssignLeadRequest binds the payload for assigning a lead to a user.
type AssignLeadRequest struct {
	OwnerUserID string `json:"owner_user_id" binding:"required"`
}

// ConvertLeadRequest binds the payload for converting a lead.
type ConvertLeadRequest struct {
	CreateCompany bool   `json:"create_company"`
	OwnerUserID   string `json:"owner_user_id"`
}

// StageRequest binds one pipeline stage in a create/replace-stages request.
// ID is omitted for a new stage; set it to update an existing stage in place.
type StageRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name" binding:"required,min=1,max=200"`
	Position    int    `json:"position"`
	Probability string `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
}

// CreatePipelineRequest binds the payload for creating a pipeline.
type CreatePipelineRequest struct {
	Name      string         `json:"name" binding:"required,min=1,max=200"`
	IsDefault bool           `json:"is_default"`
	Stages    []StageRequest `json:"stages"`
}

// UpdatePipelineRequest binds the payload for updating a pipeline.
type UpdatePipelineRequest struct {
	Name      *string `json:"name"`
	IsDefault *bool   `json:"is_default"`
}

// ReplaceStagesRequest binds the payload for PUT .../pipelines/:id/stages.
type ReplaceStagesRequest struct {
	Stages []StageRequest `json:"stages" binding:"required"`
}

// PipelineListQuery binds query parameters for listing pipelines.
type PipelineListQuery struct {
	Page            int  `form:"page"`
	PerPage         int  `form:"per_page"`
	IncludeArchived bool `form:"include_archived"`
}

// DealListQuery binds query parameters for listing deals.
type DealListQuery struct {
	Page        int    `form:"page"`
	PerPage     int    `form:"per_page"`
	Search      string `form:"search"`
	PipelineID  string `form:"pipeline_id"`
	StageID     string `form:"stage_id"`
	Status      string `form:"status"`
	OwnerUserID string `form:"owner_user_id"`
}

// CreateDealRequest binds the payload for creating a deal.
type CreateDealRequest struct {
	PipelineID        string  `json:"pipeline_id" binding:"required"`
	StageID           string  `json:"stage_id" binding:"required"`
	CompanyID         string  `json:"company_id"`
	ContactID         string  `json:"contact_id"`
	Title             string  `json:"title" binding:"required,min=1,max=200"`
	Value             string  `json:"value"`
	Currency          string  `json:"currency"`
	ExpectedCloseDate *string `json:"expected_close_date"`
	OwnerUserID       string  `json:"owner_user_id"`
}

// UpdateDealRequest binds the payload for updating a deal.
type UpdateDealRequest struct {
	StageID           *string `json:"stage_id"`
	CompanyID         *string `json:"company_id"`
	ContactID         *string `json:"contact_id"`
	Title             *string `json:"title"`
	Value             *string `json:"value"`
	Currency          *string `json:"currency"`
	ExpectedCloseDate *string `json:"expected_close_date"`
	OwnerUserID       *string `json:"owner_user_id"`
}

// MoveDealStageRequest binds the payload for POST .../deals/:id/move-stage.
type MoveDealStageRequest struct {
	StageID string `json:"stage_id" binding:"required"`
}

// CloseLostDealRequest binds the payload for POST .../deals/:id/close-lost.
type CloseLostDealRequest struct {
	LostReason string `json:"lost_reason"`
}

// ApproveDealDiscountRequest binds the payload for POST .../deals/:id/approve-discount.
type ApproveDealDiscountRequest struct {
	DiscountPercent string `json:"discount_percent" binding:"required"`
}

// ActivityListQuery binds query parameters for listing activities.
type ActivityListQuery struct {
	Page              int    `form:"page"`
	PerPage           int    `form:"per_page"`
	RelatedEntityType string `form:"related_entity_type"`
	RelatedEntityID   string `form:"related_entity_id"`
	AssigneeUserID    string `form:"assignee_user_id"`
	Status            string `form:"status"`
}

// CreateActivityRequest binds the payload for creating an activity.
type CreateActivityRequest struct {
	RelatedEntityType string  `json:"related_entity_type" binding:"required"`
	RelatedEntityID   string  `json:"related_entity_id" binding:"required"`
	Type              string  `json:"type" binding:"required"`
	Subject           string  `json:"subject" binding:"required,min=1,max=200"`
	Description       string  `json:"description"`
	DueAt             *string `json:"due_at"`
	AssigneeUserID    string  `json:"assignee_user_id"`
}

// UpdateActivityRequest binds the payload for updating an activity.
type UpdateActivityRequest struct {
	Type           *string `json:"type"`
	Subject        *string `json:"subject"`
	Description    *string `json:"description"`
	DueAt          *string `json:"due_at"`
	AssigneeUserID *string `json:"assignee_user_id"`
}

// AssignActivityRequest binds the payload for assigning an activity.
type AssignActivityRequest struct {
	AssigneeUserID string `json:"assignee_user_id" binding:"required"`
}

// LineItemRequest binds one quotation/invoice line item.
type LineItemRequest struct {
	Description     string `json:"description" binding:"required,min=1,max=500"`
	Quantity        string `json:"quantity"`
	UnitPrice       string `json:"unit_price"`
	DiscountPercent string `json:"discount_percent"`
}

// QuotationListQuery binds query parameters for listing quotations.
type QuotationListQuery struct {
	Page      int    `form:"page"`
	PerPage   int    `form:"per_page"`
	Status    string `form:"status"`
	DealID    string `form:"deal_id"`
	ContactID string `form:"contact_id"`
	CompanyID string `form:"company_id"`
}

// CreateQuotationRequest binds the payload for creating a quotation.
type CreateQuotationRequest struct {
	DealID          string            `json:"deal_id"`
	ContactID       string            `json:"contact_id"`
	CompanyID       string            `json:"company_id"`
	QuotationNumber string            `json:"quotation_number" binding:"required"`
	ValidUntil      *string           `json:"valid_until"`
	Currency        string            `json:"currency"`
	Notes           string            `json:"notes"`
	TaxTotal        string            `json:"tax_total"`
	Items           []LineItemRequest `json:"items" binding:"required,min=1,dive"`
}

// UpdateQuotationRequest binds the payload for updating a quotation.
type UpdateQuotationRequest struct {
	DealID     *string `json:"deal_id"`
	ContactID  *string `json:"contact_id"`
	CompanyID  *string `json:"company_id"`
	ValidUntil *string `json:"valid_until"`
	Notes      *string `json:"notes"`
}

// InvoiceListQuery binds query parameters for listing invoices.
type InvoiceListQuery struct {
	Page        int    `form:"page"`
	PerPage     int    `form:"per_page"`
	Status      string `form:"status"`
	DealID      string `form:"deal_id"`
	ContactID   string `form:"contact_id"`
	CompanyID   string `form:"company_id"`
	QuotationID string `form:"quotation_id"`
}

// CreateInvoiceRequest binds the payload for creating an invoice. When
// QuotationID is set and Items is empty, items/totals are copied from that
// quotation instead of being computed from Items.
type CreateInvoiceRequest struct {
	QuotationID   string            `json:"quotation_id"`
	DealID        string            `json:"deal_id"`
	ContactID     string            `json:"contact_id"`
	CompanyID     string            `json:"company_id"`
	InvoiceNumber string            `json:"invoice_number" binding:"required"`
	IssueDate     *string           `json:"issue_date"`
	DueDate       *string           `json:"due_date"`
	Currency      string            `json:"currency"`
	TaxTotal      string            `json:"tax_total"`
	Items         []LineItemRequest `json:"items"`
}

// UpdateInvoiceRequest binds the payload for updating an invoice.
type UpdateInvoiceRequest struct {
	QuotationID *string `json:"quotation_id"`
	DealID      *string `json:"deal_id"`
	ContactID   *string `json:"contact_id"`
	CompanyID   *string `json:"company_id"`
	IssueDate   *string `json:"issue_date"`
	DueDate     *string `json:"due_date"`
}

// MarkInvoicePaidRequest binds the payload for POST .../invoices/:id/mark-paid.
type MarkInvoicePaidRequest struct {
	AmountPaid string `json:"amount_paid" binding:"required"`
}

// IntegrationListQuery binds query parameters for listing integrations.
type IntegrationListQuery struct {
	Page     int    `form:"page"`
	PerPage  int    `form:"per_page"`
	Provider string `form:"provider"`
}

// CreateIntegrationRequest binds the payload for creating an integration.
type CreateIntegrationRequest struct {
	Provider string         `json:"provider" binding:"required"`
	Name     string         `json:"name" binding:"required,min=1,max=200"`
	Config   map[string]any `json:"config"`
	Secret   string         `json:"secret"`
}

// UpdateIntegrationRequest binds the payload for updating an integration
// (excluding its secret — see UpdateIntegrationSecretRequest).
type UpdateIntegrationRequest struct {
	Name     *string        `json:"name"`
	Config   map[string]any `json:"config"`
	IsActive *bool          `json:"is_active"`
}

// UpdateIntegrationSecretRequest binds the payload for
// PUT .../integrations/:id/secret.
type UpdateIntegrationSecretRequest struct {
	Secret string `json:"secret" binding:"required"`
}
