package dto

import "time"

// PageListQuery binds query parameters for listing landing pages.
type PageListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Search         string `form:"search"`
	Status         string `form:"status"`
	PageType       string `form:"page_type"`
	IsTemplate     *bool  `form:"is_template"`
	SortBy         string `form:"sort_by"`
	SortOrder      string `form:"sort_order"`
	IncludeDeleted bool   `form:"include_deleted"`
}

// TrustBadgeItem binds a single footer trust badge.
type TrustBadgeItem struct {
	ImageURL string `json:"image_url"`
	Label    string `json:"label"`
}

// PageSettingsRequest binds the typed, validated landing page settings payload.
// When present on an update, it replaces the settings object wholesale (matching
// how SEO/branding sub-objects are replaced elsewhere in this module).
type PageSettingsRequest struct {
	PublishRequireApproval  bool             `json:"publish_require_approval"`
	LeadNotificationEmails  []string         `json:"lead_notification_emails"`
	FooterCopyrightText     string           `json:"footer_copyright_text"`
	TrustBadges             []TrustBadgeItem `json:"trust_badges"`
	SecondaryCTATrackingKey string           `json:"secondary_cta_tracking_key"`
	NewsletterFormID        string           `json:"newsletter_form_id"`
	AnalyticsHooks          map[string]any   `json:"analytics_hooks"`
}

// CreatePageRequest binds the payload for creating a landing page.
type CreatePageRequest struct {
	Name       string               `json:"name" binding:"required,min=1,max=200"`
	Title      string               `json:"title" binding:"required,min=1,max=255"`
	Slug       string               `json:"slug" binding:"required,lowercase,alphanumhyphen"`
	PageType   string               `json:"page_type" binding:"required"`
	Visibility string               `json:"visibility" binding:"required"`
	Locale     string               `json:"locale"`
	Timezone   string               `json:"timezone"`
	IsHomepage bool                 `json:"is_homepage"`
	IsTemplate bool                 `json:"is_template"`
	Settings   *PageSettingsRequest `json:"settings" binding:"omitnil"`
}

// UpdatePageRequest binds the partial payload for updating a landing page.
type UpdatePageRequest struct {
	Name       *string              `json:"name" binding:"omitnil,min=1,max=200"`
	Title      *string              `json:"title" binding:"omitnil,min=1,max=255"`
	Slug       *string              `json:"slug" binding:"omitnil,lowercase,alphanumhyphen"`
	PageType   *string              `json:"page_type"`
	Visibility *string              `json:"visibility"`
	Locale     *string              `json:"locale"`
	Timezone   *string              `json:"timezone"`
	IsHomepage *bool                `json:"is_homepage"`
	IsTemplate *bool                `json:"is_template"`
	Settings   *PageSettingsRequest `json:"settings" binding:"omitnil"`
}

// CreatePageFromTemplateRequest binds the payload for creating a new page
// seeded from an existing page-level template (IsTemplate=true), copying its
// sections, SEO, and optionally its per-page branding override in one call.
type CreatePageFromTemplateRequest struct {
	TemplatePageID  string `json:"template_page_id" binding:"required,uuid"`
	Name            string `json:"name" binding:"required,min=1,max=200"`
	Title           string `json:"title" binding:"required,min=1,max=255"`
	Slug            string `json:"slug" binding:"required,lowercase,alphanumhyphen"`
	Visibility      string `json:"visibility" binding:"required"`
	Locale          string `json:"locale"`
	Timezone        string `json:"timezone"`
	IncludeBranding bool   `json:"include_branding"`
}

// UpdatePageAccessRequest binds request for updating visibility and password.
type UpdatePageAccessRequest struct {
	Visibility string `json:"visibility" binding:"required"`
	Password   string `json:"password" binding:"required_if=Visibility password_protected"`
}

// DuplicatePageRequest binds duplicating options.
type DuplicatePageRequest struct {
	Name         string `json:"name" binding:"required,min=1,max=200"`
	Slug         string `json:"slug" binding:"required,lowercase,alphanumhyphen"`
	IncludeForms bool   `json:"include_forms"`
}

// RobotsSettings matches robots configuration.
type RobotsSettings struct {
	Index  bool `json:"index"`
	Follow bool `json:"follow"`
}

// OpenGraphSettings matches OG preview options.
type OpenGraphSettings struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ImageAssetID string `json:"image_asset_id"`
}

// SitemapSettings matches sitemap config.
type SitemapSettings struct {
	Included bool    `json:"included"`
	Priority float64 `json:"priority"`
}

// UpdatePageSEORequest binds SEO parameters.
type UpdatePageSEORequest struct {
	MetaTitle       string            `json:"meta_title"`
	MetaDescription string            `json:"meta_description"`
	MetaKeywords    []string          `json:"meta_keywords"`
	CanonicalURL    *string           `json:"canonical_url"`
	Robots          RobotsSettings    `json:"robots"`
	OpenGraph       OpenGraphSettings `json:"open_graph"`
	TwitterCard     string            `json:"twitter_card"`
	SchemaMarkup    map[string]any    `json:"schema_markup"`
	Sitemap         SitemapSettings   `json:"sitemap"`
}

// CreateSectionRequest binds section creation.
type CreateSectionRequest struct {
	Key       string         `json:"key" binding:"required,lowercase,alphanumhyphen"`
	Type      string         `json:"type" binding:"required"`
	Name      string         `json:"name" binding:"required,min=1,max=200"`
	SortOrder int            `json:"sort_order" binding:"min=0"`
	IsEnabled bool           `json:"is_enabled"`
	Content   map[string]any `json:"content"`
	Style     map[string]any `json:"style"`
}

// UpdateSectionRequest binds section partial update.
type UpdateSectionRequest struct {
	Name      *string         `json:"name" binding:"omitnil,min=1,max=200"`
	IsEnabled *bool           `json:"is_enabled"`
	Content   *map[string]any `json:"content"`
	Style     *map[string]any `json:"style"`
}

// SectionReorderItem binds an order position.
type SectionReorderItem struct {
	ID        string `json:"id" binding:"required,uuid"`
	SortOrder int    `json:"sort_order" binding:"min=0"`
}

// ReorderSectionsRequest binds bulk ordering options.
type ReorderSectionsRequest struct {
	Items []SectionReorderItem `json:"items" binding:"required,dive,required"`
}

// CreateFormRequest binds form creation.
type CreateFormRequest struct {
	Name           string         `json:"name" binding:"required,min=1,max=200"`
	Key            string         `json:"key" binding:"required,lowercase,alphanumhyphen"`
	IsActive       bool           `json:"is_active"`
	SubmitLabel    string         `json:"submit_label" binding:"required"`
	SuccessMessage string         `json:"success_message"`
	RedirectURL    *string        `json:"redirect_url"`
	Consent        map[string]any `json:"consent"`
}

// UpdateFormRequest binds form updates.
type UpdateFormRequest struct {
	Name           *string         `json:"name" binding:"omitnil,min=1,max=200"`
	IsActive       *bool           `json:"is_active"`
	SubmitLabel    *string         `json:"submit_label"`
	SuccessMessage *string         `json:"success_message"`
	RedirectURL    *string         `json:"redirect_url"`
	Consent        *map[string]any `json:"consent"`
}

// FormFieldItem binds a single field specification.
type FormFieldItem struct {
	Key         string         `json:"key" binding:"required,lowercase,alphanumunderscore"`
	Type        string         `json:"type" binding:"required"`
	Label       string         `json:"label" binding:"required,min=1,max=255"`
	Placeholder string         `json:"placeholder"`
	Options     []string       `json:"options"`
	Validation  map[string]any `json:"validation"`
	Required    bool           `json:"required"`
	SortOrder   int            `json:"sort_order" binding:"min=0"`
}

// ReplaceFormFieldsRequest binds the bulk replace structure.
type ReplaceFormFieldsRequest struct {
	Fields []FormFieldItem `json:"fields" binding:"required,dive,required"`
}

// SubmissionListQuery binds query filters for form submissions.
type SubmissionListQuery struct {
	Page          int        `form:"page"`
	PerPage       int        `form:"per_page"`
	LandingPageID string     `form:"landing_page_id"`
	FormID        string     `form:"form_id"`
	Status        string     `form:"status"`
	UTMSource     string     `form:"utm_source"`
	UTMCampaign   string     `form:"utm_campaign"`
	Search        string     `form:"search"`
	SortBy        string     `form:"sort_by"`
	SortOrder     string     `form:"sort_order"`
	DateFrom      *time.Time `form:"date_from"`
	DateTo        *time.Time `form:"date_to"`
}

// UpdateSubmissionStatusRequest binds submission updates.
type UpdateSubmissionStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

// CreateSubmissionNoteRequest binds note creations.
type CreateSubmissionNoteRequest struct {
	Note string `json:"note" binding:"required,min=1"`
}

// SocialLinkRequest binds a single social link entry. Platform is a free-text
// key (e.g. "instagram", "whatsapp") — there is no closed enum since brands
// can link to any platform.
type SocialLinkRequest struct {
	Platform string `json:"platform" binding:"required"`
	URL      string `json:"url" binding:"required"`
}

// ThemeBrandingRequest binds theme configuration.
type ThemeBrandingRequest struct {
	CompanyName    *string              `json:"company_name"`
	Tagline        *string              `json:"tagline"`
	LogoLightURL   *string              `json:"logo_light_url"`
	LogoDarkURL    *string              `json:"logo_dark_url"`
	FaviconURL     *string              `json:"favicon_url"`
	SocialImageURL *string              `json:"social_image_url"`
	Colors         *map[string]any      `json:"colors" binding:"omitnil"`
	Typography     *map[string]any      `json:"typography" binding:"omitnil"`
	Shape          *map[string]any      `json:"shape" binding:"omitnil"`
	Layout         *map[string]any      `json:"layout" binding:"omitnil"`
	Contact        *map[string]any      `json:"contact" binding:"omitnil"`
	SocialLinks    *[]SocialLinkRequest `json:"social_links" binding:"omitnil,dive"`
}

// CTARequest binds Call to Action payloads.
type CTARequest struct {
	Name        string `json:"name" binding:"required,min=1,max=200"`
	Label       string `json:"label" binding:"required,min=1,max=100"`
	Type        string `json:"type" binding:"required"`
	Target      string `json:"target"`
	Destination string `json:"destination" binding:"required"`
	TrackingKey string `json:"tracking_key" binding:"required,lowercase,alphanumhyphen"`
}

// SectionTemplateRequest binds Section Template parameters.
type SectionTemplateRequest struct {
	Name        string         `json:"name" binding:"required,min=1,max=200"`
	Description string         `json:"description"`
	SectionType string         `json:"section_type" binding:"required"`
	Content     map[string]any `json:"content"`
	Style       map[string]any `json:"style"`
}

// MenuRequest binds menu payloads.
type MenuRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=200"`
	Location string `json:"location" binding:"required"`
	IsActive bool   `json:"is_active"`
}

// MenuItemRequest binds nested menu items.
type MenuItemRequest struct {
	ParentID    *string `json:"parent_id"`
	Label       string  `json:"label" binding:"required,min=1,max=100"`
	LinkType    string  `json:"link_type" binding:"required"`
	Destination string  `json:"destination" binding:"required"`
	Target      string  `json:"target"`
	SortOrder   int     `json:"sort_order" binding:"min=0"`
	IsEnabled   bool    `json:"is_enabled"`
}

// DomainBindingRequest binds custom domains to landing pages.
type DomainBindingRequest struct {
	OrganizationDomainID string `json:"organization_domain_id" binding:"required,uuid"`
	LandingPageID        string `json:"landing_page_id" binding:"required,uuid"`
	IsPrimary            bool   `json:"is_primary"`
}

// RevisionListQuery binds revision lists.
type RevisionListQuery struct {
	Page    int `form:"page"`
	PerPage int `form:"per_page"`
}

// ScheduleRequest binds schedule timers.
type ScheduleRequest struct {
	PublishAt   *time.Time `json:"publish_at"`
	UnpublishAt *time.Time `json:"unpublish_at"`
	Timezone    string     `json:"timezone"`
}

// LeadIntegrationRequest binds integration config options.
type LeadIntegrationRequest struct {
	Name         string         `json:"name" binding:"required,min=1,max=200"`
	Type         string         `json:"type" binding:"required"`
	Credentials  map[string]any `json:"credentials"`
	EventFilters []any          `json:"event_filters"`
	IsActive     bool           `json:"is_active"`
}

// PublicAccessChallengeRequest challenge page password.
type PublicAccessChallengeRequest struct {
	Password string `json:"password" binding:"required"`
}

// PublicSubmissionContext holds client-sent tracking data.
type PublicSubmissionContext struct {
	PageVersion int    `json:"page_version"`
	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	UTMTerm     string `json:"utm_term"`
	UTMContent  string `json:"utm_content"`
	Referrer    string `json:"referrer"`
}

// PublicSubmissionRequest binds a public visitor submission payload.
type PublicSubmissionRequest struct {
	Fields        map[string]any          `json:"fields" binding:"required"`
	Consent       bool                    `json:"consent" binding:"required"`
	Context       PublicSubmissionContext `json:"context"`
	CaptchaToken  string                  `json:"captcha_token"`
	Website       string                  `json:"website"` // Honeypot field (must be empty)
	UploadedFiles map[string][]string     `json:"uploaded_files"`
}

// PresignMediaUploadRequest meminta URL bertanda tangan untuk mengunggah media
// langsung ke object storage.
type PresignMediaUploadRequest struct {
	Filename  string `json:"filename" binding:"required,min=1,max=255"`
	MimeType  string `json:"mime_type" binding:"required"`
	SizeBytes int64  `json:"size_bytes" binding:"required,min=1"`
}

// ConfirmMediaUploadRequest mencatat metadata media setelah unggah presigned
// selesai.
type ConfirmMediaUploadRequest struct {
	ObjectKey string `json:"object_key" binding:"required"`
	Filename  string `json:"filename" binding:"required,min=1,max=255"`
	MimeType  string `json:"mime_type" binding:"required"`
	SizeBytes int64  `json:"size_bytes" binding:"required,min=1"`
	AltText   string `json:"alt_text"`
}

// PresignMediaDownloadRequest meminta URL bertanda tangan untuk mengunduh object
// milik organization langsung dari object storage.
type PresignMediaDownloadRequest struct {
	ObjectKey string `json:"object_key" binding:"required"`
}

// PublicAnalyticsEventRequest tracks visitor interaction.
type PublicAnalyticsEventRequest struct {
	Event       string         `json:"event" binding:"required"`
	PageID      string         `json:"page_id" binding:"required"`
	PageVersion int            `json:"page_version" binding:"required,min=1"`
	SectionKey  string         `json:"section_key"`
	TargetKey   string         `json:"target_key"`
	SessionID   string         `json:"session_id"`
	Context     map[string]any `json:"context"`
}
