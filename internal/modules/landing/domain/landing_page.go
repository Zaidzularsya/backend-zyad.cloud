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
	Locale           string
	Timezone         string
	IsHomepage       bool
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
