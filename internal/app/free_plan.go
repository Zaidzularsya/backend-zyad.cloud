package app

import (
	"encoding/json"
	"strconv"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
)

// freePlanStorageBytes adalah 500 MB dalam satuan yang dibaca modul asset (storage.max_bytes).
const freePlanStorageBytes int64 = 500 * 1024 * 1024

// FreePlanFeatures adalah definisi paket Free (spec entitlement-upgrade-ux §4): fitur produk katalog
// platform ber-SKU SELF_SERVE_FREE_PRODUCT_SKU. Tidak ada key kuota untuk payment, credit note, refund,
// dan produk katalog (tidak dibatasi). Key yang tidak tercantum di sini sengaja tidak diberikan ke Free.
func FreePlanFeatures() []catalogdomain.ProductFeature {
	type entry struct {
		key   string
		value json.RawMessage
	}
	boolean := func(key string, v bool) entry { return entry{key, json.RawMessage(strconv.FormatBool(v))} }
	integer := func(key string, v int64) entry { return entry{key, json.RawMessage(strconv.FormatInt(v, 10))} }

	entries := []entry{
		boolean("landing.enabled", true),
		integer("landing.max_pages", 3),
		integer("landing.max_sections_per_page", 12),
		boolean("landing.custom_domain", false),

		boolean("domain.enabled", true),
		integer("domain.max_domains", 1),

		boolean("media.enabled", true),
		integer("media.max_storage_mb", 500),
		// storage.max_bytes adalah key yang benar-benar ditegakkan modul asset; media.max_storage_mb
		// dipertahankan konsisten untuk tampilan katalog.
		integer("storage.max_bytes", freePlanStorageBytes),
		integer("media.max_file_size_mb", 10),

		integer("users.max_users", 2),
		boolean("users.invite_user", true),

		boolean("crm.enabled", true),
		boolean("crm.pipeline", true),
		boolean("crm.lead_form", true),
		integer("crm.max_leads", 50),
		integer("crm.max_contacts", 100),
		integer("crm.max_companies", 50),
		integer("crm.max_deals", 25),
		integer("crm.max_pipelines", 1),
		integer("crm.max_quotations_per_month", 10),
		integer("crm.max_sales_orders_per_month", 10),

		boolean("receivable.enabled", true),
		integer("receivable.max_invoices_per_month", 10),
		// Keputusan pemilik produk 23 Okt 2026: tenant Free boleh menagih pelanggannya online
		// (checkout mandiri); semua pembayaran tetap masuk ke DOKU Zyad.
		boolean("receivable.online_payment", true),

		// WhatsApp tetap ada di Free: produk FREE produksi sudah memakainya dan modulnya ditegakkan
		// kode (gerbang rute /app/whatsapp dan pengiriman dokumen). Sengaja tanpa whatsapp.max_sessions
		// di Free (produk FREE produksi tidak pernah membawanya).
		boolean("whatsapp.enabled", true),
	}
	out := make([]catalogdomain.ProductFeature, len(entries))
	for i, e := range entries {
		out[i] = catalogdomain.ProductFeature{FeatureKey: e.key, Value: e.value, Position: i + 1}
	}
	return out
}
