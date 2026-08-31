package domain

import "time"

type PageStatus string

const (
	PageStatusDraft       PageStatus = "draft"
	PageStatusPublished   PageStatus = "published"
	PageStatusUnpublished PageStatus = "unpublished"
	PageStatusArchived    PageStatus = "archived"
)

func (s PageStatus) IsValid() bool {
	switch s {
	case PageStatusDraft, PageStatusPublished, PageStatusUnpublished, PageStatusArchived:
		return true
	default:
		return false
	}
}

type PageVisibility string

const (
	PageVisibilityPublic            PageVisibility = "public"
	PageVisibilityPrivate           PageVisibility = "private"
	PageVisibilityPasswordProtected PageVisibility = "password_protected"
)

func (v PageVisibility) IsValid() bool {
	switch v {
	case PageVisibilityPublic, PageVisibilityPrivate, PageVisibilityPasswordProtected:
		return true
	default:
		return false
	}
}

type PageType string

const (
	PageTypeHomepage           PageType = "homepage"
	PageTypeCompanyProfile     PageType = "company_profile"
	PageTypeProductService     PageType = "product_service"
	PageTypeCampaign           PageType = "campaign"
	PageTypePricing            PageType = "pricing"
	PageTypeContact            PageType = "contact"
	PageTypeLeadCapture        PageType = "lead_capture"
	PageTypePromoEvent         PageType = "promo_event"
	PageTypePortfolioCaseStudy PageType = "portfolio_case_study"
)

func (p PageType) IsValid() bool {
	switch p {
	case PageTypeHomepage,
		PageTypeCompanyProfile,
		PageTypeProductService,
		PageTypeCampaign,
		PageTypePricing,
		PageTypeContact,
		PageTypeLeadCapture,
		PageTypePromoEvent,
		PageTypePortfolioCaseStudy:
		return true
	default:
		return false
	}
}

type TrustBadge struct {
	ImageURL string `json:"image_url"`
	Label    string `json:"label"`
}

// PageSettings is the typed, validated shape for landing_pages.settings.
// It intentionally excludes concerns (workspace access, role scope,
// approval flow, analytics hooks) that require subsystems that don't
// exist yet; AnalyticsHooks stays an opaque passthrough for forward-compat.
type PageSettings struct {
	PublishRequireApproval bool           `json:"publish_require_approval"`
	LeadNotificationEmails []string       `json:"lead_notification_emails"`
	FooterCopyrightText    string         `json:"footer_copyright_text"`
	TrustBadges            []TrustBadge   `json:"trust_badges"`
	SecondaryCTATrackingKey string        `json:"secondary_cta_tracking_key"`
	NewsletterFormID       string         `json:"newsletter_form_id"`
	AnalyticsHooks         map[string]any `json:"analytics_hooks,omitempty"`
}

type LandingPage struct {
	ID               string
	OrganizationID   string
	Name             string
	Title            string
	Slug             string
	Type             PageType
	Status           PageStatus
	Visibility       PageVisibility
	PasswordHash     string
	SEO              map[string]any
	Settings         PageSettings
	Locale           string
	Timezone         string
	IsHomepage       bool
	IsTemplate       bool
	PublishedVersion int
	PublishAt        *time.Time
	UnpublishAt      *time.Time
	PublishedAt      *time.Time
	CreatedBy        string
	UpdatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
