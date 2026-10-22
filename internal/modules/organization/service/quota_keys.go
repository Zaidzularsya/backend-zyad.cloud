package service

// uncappedWhenAbsentQuotaKeys are the quota keys introduced by migrations
// 000158/000159. Products and contracts sold before those migrations never
// carried them, so an organization with NO entitlement row for one of these
// keys is treated as uncapped (RequireRoom allows, the summary reports
// `unlimited`). A row with explicit enabled=false is still "not enabled", and a
// row with a limit is enforced. Older keys (crm.max_contacts, landing.max_pages,
// users.max_users, storage.max_bytes, ...) keep their original semantics.
var uncappedWhenAbsentQuotaKeys = []string{
	"crm.max_leads", "crm.max_companies", "crm.max_deals", "crm.max_pipelines",
	"crm.max_quotations_per_month", "crm.max_sales_orders_per_month",
	"receivable.max_invoices_per_month", "domain.max_domains",
}

// UncappedWhenAbsentQuotaKeys returns a copy of the quota keys that are
// uncapped for an organization holding no entitlement row for them.
func UncappedWhenAbsentQuotaKeys() []string {
	return append([]string(nil), uncappedWhenAbsentQuotaKeys...)
}

// IsUncappedWhenAbsent reports whether a missing entitlement row for featureKey
// means "no cap" rather than "not enabled".
func IsUncappedWhenAbsent(featureKey string) bool {
	for _, k := range uncappedWhenAbsentQuotaKeys {
		if k == featureKey {
			return true
		}
	}
	return false
}
