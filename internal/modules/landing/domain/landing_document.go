package domain

import "time"

// LandingPageDocument is the 1:1 GrapesJS store for a landing page whose
// builder == PageBuilderGrapesJS. `Project` is the editable source of truth
// (grapesjs getProjectData); `HTML`/`CSS` are the last-saved export used as the
// working copy — the published copy is sanitized into landing_page_versions.
type LandingPageDocument struct {
	LandingPageID  string
	OrganizationID string
	Project        map[string]any
	HTML           string
	CSS            string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
