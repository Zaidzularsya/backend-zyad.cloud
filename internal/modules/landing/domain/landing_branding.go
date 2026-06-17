package domain

import "time"

type BrandingColors struct {
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
	Muted      string `json:"muted"`
}

type BrandingTypography struct {
	HeadingFont string `json:"heading_font"`
	BodyFont    string `json:"body_font"`
}

type BrandingShape struct {
	ButtonRadius string `json:"button_radius"`
	CardRadius   string `json:"card_radius"`
}

type BrandingLayout struct {
	Width           string `json:"width"`
	Spacing         string `json:"spacing"`
	BackgroundStyle string `json:"background_style"`
	ColorMode       string `json:"color_mode"`
	HeaderStyle     string `json:"header_style"`
	FooterStyle     string `json:"footer_style"`
}

type BrandingContact struct {
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
}

type BrandingSocialLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type LandingBranding struct {
	ID             string
	OrganizationID string
	LandingPageID  *string
	CompanyName    string
	Tagline        string
	LogoLightURL   string
	LogoDarkURL    string
	FaviconURL     string
	SocialImageURL string
	Colors         BrandingColors
	Typography     BrandingTypography
	Shape          BrandingShape
	Layout         BrandingLayout
	Contact        BrandingContact
	SocialLinks    []BrandingSocialLink
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type LandingDomainBinding struct {
	ID                   string
	OrganizationID       string
	OrganizationDomainID string
	LandingPageID        string
	IsPrimary            bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
