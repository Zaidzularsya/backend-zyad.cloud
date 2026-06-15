package domain

import "time"

type SectionType string

const (
	SectionTypeHero            SectionType = "hero"
	SectionTypeAbout           SectionType = "about"
	SectionTypeFeatures        SectionType = "features"
	SectionTypeServices        SectionType = "services"
	SectionTypeProductShowcase SectionType = "product_showcase"
	SectionTypeContent         SectionType = "content"
	SectionTypeGallery         SectionType = "gallery"
	SectionTypePortfolio       SectionType = "portfolio"
	SectionTypeTestimonial     SectionType = "testimonial"
	SectionTypePricing         SectionType = "pricing"
	SectionTypeFAQ             SectionType = "faq"
	SectionTypeCTA             SectionType = "cta"
	SectionTypeForm            SectionType = "form"
	SectionTypeContact         SectionType = "contact"
	SectionTypeNewsletter      SectionType = "newsletter"
	SectionTypePartnerLogos    SectionType = "partner_logos"
	SectionTypeStatistics      SectionType = "statistics"
	SectionTypeFooter          SectionType = "footer"
)

func (s SectionType) IsValid() bool {
	switch s {
	case SectionTypeHero,
		SectionTypeAbout,
		SectionTypeFeatures,
		SectionTypeServices,
		SectionTypeProductShowcase,
		SectionTypeContent,
		SectionTypeGallery,
		SectionTypePortfolio,
		SectionTypeTestimonial,
		SectionTypePricing,
		SectionTypeFAQ,
		SectionTypeCTA,
		SectionTypeForm,
		SectionTypeContact,
		SectionTypeNewsletter,
		SectionTypePartnerLogos,
		SectionTypeStatistics,
		SectionTypeFooter:
		return true
	default:
		return false
	}
}

type LandingSection struct {
	ID            string
	LandingPageID string
	Key           string
	Type          SectionType
	Name          string
	SortOrder     int
	IsEnabled     bool
	Content       map[string]any
	Style         map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
