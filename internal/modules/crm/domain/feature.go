package domain

// Entitlement feature keys for the CRM module. These must match the
// feature_key values seeded in product_features (migrations 000055, 000074).
const (
	FeatureCRMEnabled      = "crm.enabled"
	FeatureCRMMaxContacts  = "crm.max_contacts"
	FeatureCRMImportExport = "crm.import_export"
	FeatureCRMPipeline     = "crm.pipeline"
	FeatureCRMLeadForm     = "crm.lead_form"

	// Quota keys (integer limits; *PerMonth reset monthly). Registered in migration 000158.
	FeatureCRMMaxLeads               = "crm.max_leads"
	FeatureCRMMaxCompanies           = "crm.max_companies"
	FeatureCRMMaxDeals               = "crm.max_deals"
	FeatureCRMMaxPipelines           = "crm.max_pipelines"
	FeatureCRMMaxQuotationsPerMonth  = "crm.max_quotations_per_month"
	FeatureCRMMaxSalesOrdersPerMonth = "crm.max_sales_orders_per_month"
)
