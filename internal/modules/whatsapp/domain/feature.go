package domain

const (
	FeatureWhatsAppEnabled     = "whatsapp.enabled"
	FeatureWhatsAppMaxSessions = "whatsapp.max_sessions"

	// DefaultMaxSessionsFallback applies when an organization has
	// whatsapp.enabled but no whatsapp.max_sessions runtime entitlement yet
	// (plan synced before migration 000128). The next plan sync replaces it.
	DefaultMaxSessionsFallback = 1
)
