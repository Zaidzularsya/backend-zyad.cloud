package domain

// AllowedStyleKeys is the allowlist of top-level keys permitted inside
// landing_page_sections.style. Anything outside this set is dropped on write
// (see sectionService.sanitizeStyleMap). Keeping style an allowlist means a
// compromised admin token cannot smuggle arbitrary attributes into the style
// object that renderer components interpolate straight into inline styles /
// CSS url() (e.g. EnterpriseTemplateSection reads style.hero.backgroundImage).
var AllowedStyleKeys = map[string]bool{
	"variant":            true, // section variant selector (footer catalog, template family)
	"family":             true, // template design family
	"renderer_component": true, // legacy variant pointer, still read by SectionRenderer.vue
	"spacing":            true, // { top, bottom } in px
	"background":         true, // { type, color, image }
	"hero":               true, // { backgroundImage, overlay }
	"colors":             true, // { primary, secondary, surface, text, muted }
	"align":              true, // left | center | right
	"visible":            true, // bool rendering hint (mirrors is_enabled)
}

// StyleURLKeys are leaf keys whose string value is treated as a URL. Their
// value must pass the CSS-safe URL check in sectionService.sanitizeStyleURL
// (http/https or site-relative only, no characters that break out of url("…")).
var StyleURLKeys = map[string]bool{
	"image":           true,
	"backgroundImage": true,
	"url":             true,
	"src":             true,
}
