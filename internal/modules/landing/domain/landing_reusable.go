package domain

import "time"

type CTAType string

const (
	CTATypeContactForm       CTAType = "contact_form"
	CTATypeWhatsApp          CTAType = "whatsapp"
	CTATypeExternalLink      CTAType = "external_link"
	CTATypeInternalPage      CTAType = "internal_page"
	CTATypeDocumentDownload  CTAType = "document_download"
)

func (t CTAType) IsValid() bool {
	switch t {
	case CTATypeContactForm, CTATypeWhatsApp, CTATypeExternalLink, CTATypeInternalPage, CTATypeDocumentDownload:
		return true
	default:
		return false
	}
}

type CTATarget string

const (
	CTATargetSelf   CTATarget = "self"
	CTATargetNewTab CTATarget = "new_tab"
)

func (t CTATarget) IsValid() bool {
	switch t {
	case CTATargetSelf, CTATargetNewTab:
		return true
	default:
		return false
	}
}

type LandingCTA struct {
	ID             string
	OrganizationID string
	Name           string
	Label          string
	Type           CTAType
	Target         CTATarget
	Destination    string
	TrackingKey    string
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type SectionTemplate struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	SectionType    SectionType
	Content        map[string]any
	Style          map[string]any
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type MenuLocation string

const (
	MenuLocationHeader  MenuLocation = "header"
	MenuLocationFooter  MenuLocation = "footer"
	MenuLocationSidebar MenuLocation = "sidebar"
)

func (l MenuLocation) IsValid() bool {
	switch l {
	case MenuLocationHeader, MenuLocationFooter, MenuLocationSidebar:
		return true
	default:
		return false
	}
}

type LandingMenu struct {
	ID             string
	OrganizationID string
	Name           string
	Location       MenuLocation
	IsActive       bool
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type LinkType string

const (
	LinkTypeInternalPage LinkType = "internal_page"
	LinkTypeExternalLink LinkType = "external_link"
	LinkTypeAnchor       LinkType = "anchor"
	LinkTypeButton       LinkType = "button"
)

func (t LinkType) IsValid() bool {
	switch t {
	case LinkTypeInternalPage, LinkTypeExternalLink, LinkTypeAnchor, LinkTypeButton:
		return true
	default:
		return false
	}
}

type LandingMenuItem struct {
	ID             string
	OrganizationID string
	MenuID         string
	ParentID       *string
	Label          string
	LinkType       LinkType
	Destination    string
	Target         CTATarget
	SortOrder      int
	IsEnabled      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
