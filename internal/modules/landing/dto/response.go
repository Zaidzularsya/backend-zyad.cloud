package dto

import "time"

// PaginationMeta wraps database query totals and counts.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PageResponse represents the details of a landing page.
type PageResponse struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Title            string         `json:"title"`
	Slug             string         `json:"slug"`
	PageType         string         `json:"page_type"`
	Status           string         `json:"status"`
	Visibility       string         `json:"visibility"`
	Locale           string         `json:"locale"`
	Timezone         string         `json:"timezone"`
	IsHomepage       bool           `json:"is_homepage"`
	PublishedVersion int            `json:"published_version"`
	PublishAt        *time.Time     `json:"publish_at,omitempty"`
	UnpublishAt      *time.Time     `json:"unpublish_at,omitempty"`
	PublishedAt      *time.Time     `json:"published_at,omitempty"`
	Settings         map[string]any `json:"settings"`
	SEO              map[string]any `json:"seo,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        *time.Time     `json:"deleted_at,omitempty"`
}

// PageListResponse wraps pages and metadata.
type PageListResponse struct {
	Items []PageResponse `json:"items"`
	Meta  PaginationMeta `json:"meta"`
}

// SectionResponse represents a single landing page layout section.
type SectionResponse struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	SortOrder int            `json:"sort_order"`
	IsEnabled bool           `json:"is_enabled"`
	Content   map[string]any `json:"content"`
	Style     map[string]any `json:"style"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FormResponse represents form specifications.
type FormResponse struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Key            string         `json:"key"`
	SubmitLabel    string         `json:"submit_label"`
	SuccessMessage string         `json:"success_message"`
	RedirectURL    *string        `json:"redirect_url,omitempty"`
	IsActive       bool           `json:"is_active"`
	Consent        map[string]any `json:"consent,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// FormFieldResponse represents form input field metadata.
type FormFieldResponse struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Placeholder string         `json:"placeholder"`
	Options     []string       `json:"options"`
	Validation  map[string]any `json:"validation"`
	Required    bool           `json:"required"`
	SortOrder   int            `json:"sort_order"`
}

// SubmissionResponse represents a client form entry.
type SubmissionResponse struct {
	ID            string         `json:"id"`
	LandingPageID string         `json:"landing_page_id"`
	FormID        string         `json:"form_id"`
	Reference     string         `json:"reference"`
	Status        string         `json:"status"`
	SubmittedData map[string]any `json:"submitted_data"`
	SourceURL     string         `json:"source_url,omitempty"`
	Referrer      string         `json:"referrer,omitempty"`
	UTMSource     string         `json:"utm_source,omitempty"`
	UTMMedium     string         `json:"utm_medium,omitempty"`
	UTMCampaign   string         `json:"utm_campaign,omitempty"`
	UTMTerm       string         `json:"utm_term,omitempty"`
	UTMContent    string         `json:"utm_content,omitempty"`
	SubmittedAt   time.Time      `json:"submitted_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// SubmissionListResponse wraps list items and metrics.
type SubmissionListResponse struct {
	Items []SubmissionResponse `json:"items"`
	Meta  PaginationMeta       `json:"meta"`
}

// SubmissionNoteResponse wraps note text and metadata.
type SubmissionNoteResponse struct {
	ID           string    `json:"id"`
	SubmissionID string    `json:"submission_id"`
	Note         string    `json:"note"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// BrandingResponse represents organization branding theme parameters.
type BrandingResponse struct {
	CompanyName    string           `json:"company_name"`
	Tagline        string           `json:"tagline"`
	LogoLightURL   string           `json:"logo_light_url"`
	LogoDarkURL    string           `json:"logo_dark_url"`
	FaviconURL     string           `json:"favicon_url"`
	SocialImageURL string           `json:"social_image_url"`
	Colors         map[string]any   `json:"colors"`
	Typography     map[string]any   `json:"typography"`
	Shape          map[string]any   `json:"shape"`
	Layout         map[string]any   `json:"layout"`
	Contact        map[string]any   `json:"contact"`
	SocialLinks    []map[string]any `json:"social_links"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// ThemeResponse represents the combined/effective token values.
type ThemeResponse struct {
	Branding BrandingResponse `json:"branding"`
}

// CTAResponse represents Call to Action configurations.
type CTAResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Type        string    `json:"type"`
	Target      string    `json:"target"`
	Destination string    `json:"destination"`
	TrackingKey string    `json:"tracking_key"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SectionTemplateResponse represents visual layout templates.
type SectionTemplateResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	SectionType string         `json:"section_type"`
	Content     map[string]any `json:"content"`
	Style       map[string]any `json:"style"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// MenuResponse represents headers/footers navigation.
type MenuResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MenuItemResponse represents a single item inside navigation menus.
type MenuItemResponse struct {
	ID          string             `json:"id"`
	ParentID    *string            `json:"parent_id,omitempty"`
	Label       string             `json:"label"`
	LinkType    string             `json:"link_type"`
	Destination string             `json:"destination"`
	Target      string             `json:"target"`
	SortOrder   int                `json:"sort_order"`
	IsEnabled   bool               `json:"is_enabled"`
	Children    []MenuItemResponse `json:"children,omitempty"`
}

// DomainBindingResponse custom domain page bindings.
type DomainBindingResponse struct {
	ID                   string    `json:"id"`
	OrganizationDomainID string    `json:"organization_domain_id"`
	LandingPageID        string    `json:"landing_page_id"`
	IsPrimary            bool      `json:"is_primary"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// MediaAssetResponse represents stored binary metadata references.
type MediaAssetResponse struct {
	ID               string     `json:"id"`
	Filename         string     `json:"filename"`
	MimeType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	Width            *int       `json:"width,omitempty"`
	Height           *int       `json:"height,omitempty"`
	DurationSeconds  *int       `json:"duration_seconds,omitempty"`
	AltText          string     `json:"alt_text"`
	ProcessingStatus string     `json:"processing_status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// RevisionResponse represents page snapshots.
type RevisionResponse struct {
	ID             string         `json:"id"`
	LandingPageID  string         `json:"landing_page_id"`
	RevisionNumber int            `json:"revision_number"`
	ChangeNote     string         `json:"change_note,omitempty"`
	Snapshot       map[string]any `json:"snapshot"`
	CreatedBy      string         `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ScheduleResponse represents scheduled action jobs.
type ScheduleResponse struct {
	ID            string     `json:"id"`
	LandingPageID string     `json:"landing_page_id"`
	Action        string     `json:"action"`
	ScheduledAt   time.Time  `json:"scheduled_at"`
	Status        string     `json:"status"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	Attempts      int        `json:"attempts"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// LeadIntegrationResponse represents configured lead webhook targets.
type LeadIntegrationResponse struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Credentials  map[string]any `json:"credentials"` // Should have secrets masked when serialized
	EventFilters []any          `json:"event_filters"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// LeadDeliveryLogResponse represents retry outbox queues for integrations.
type LeadDeliveryLogResponse struct {
	ID              string     `json:"id"`
	IntegrationID   string     `json:"integration_id"`
	SubmissionID    string     `json:"submission_id"`
	Status          string     `json:"status"`
	ResponsePayload *string    `json:"response_payload,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	Attempts        int        `json:"attempts"`
	NextRetryAt     *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PublicPageDetails represents standard details exposed to public visitors.
type PublicPageDetails struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Locale  string `json:"locale"`
	Version int    `json:"version"`
}

// PublicResolveResponse represents the aggregate public payload envelope.
type PublicResolveResponse struct {
	Page         PublicPageDetails  `json:"page"`
	Branding     BrandingResponse   `json:"branding"`
	SEO          map[string]any     `json:"seo"`
	Sections     []SectionResponse  `json:"sections"`
	Forms        []FormResponse     `json:"forms"`
	CanonicalURL string             `json:"canonical_url"`
	PublishedAt  time.Time          `json:"published_at"`
}

// PublicSubmissionResponse is returned upon success.
type PublicSubmissionResponse struct {
	Reference      string  `json:"reference"`
	SuccessMessage string  `json:"success_message"`
	RedirectURL    *string `json:"redirect_url,omitempty"`
}
