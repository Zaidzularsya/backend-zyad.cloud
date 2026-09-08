package domain

// Entitlement feature keys for the CRM module. These must match the
// feature_key values seeded in billing_features (migrations 000055, 000074).
const (
	FeatureCRMEnabled      = "crm.enabled"
	FeatureCRMMaxContacts  = "crm.max_contacts"
	FeatureCRMImportExport = "crm.import_export"
	FeatureCRMPipeline     = "crm.pipeline"
	FeatureCRMLeadForm     = "crm.lead_form"
)
