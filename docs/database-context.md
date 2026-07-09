# Database Context — zyad.cloud Backend

> Sumber: `migrations/000001`–`000062` (dibaca langsung, bukan dari dokumen lama). Diperbarui 2026-07-01
> setelah refactor domain-split modul `billing` → `product`/`subscription`/`billing` (migration 000058–000062,
> lihat [product-subscription-billing-concept.md](product-subscription-billing-concept.md)). Untuk detail
> schema per fitur yang lebih dalam, rujuk `docs/multi-tenant-schema-query-audit.md`,
> `docs/landing-page-schema-audit.md`, dan `docs/multi-tenant-rls.md` — dokumen ini adalah ringkasan
> lintas-modul, bukan pengganti dokumen-dokumen tersebut.

## 1. Daftar Tabel per Domain

### Auth & User (migration 000001–000004)
`permissions`, `roles`, `role_permissions`, `users`, `user_profiles`, `auth_identities`, `sessions`,
`refresh_tokens`, `password_reset_tokens`, `email_verification_tokens`, `otp_codes`, `user_roles`,
`user_permissions`, `login_histories`, `audit_logs`

### Notification (migration 000005–000012)
`notification_templates`, `notification_logs`, `notification_preferences`, `notification_outbox_events`
(detail lengkap: `docs/reference-notification.md`, `docs/notification-traceability-index.md`)

### Organization / Multi-Tenant (migration 000013–000024)
`organizations`, `organization_memberships`, `organization_domains`, `reserved_subdomains`,
`organization_entitlements`, `organization_usage_counters`, `organization_impersonation_sessions`

### Landing Page (migration 000025–000044)
`landing_pages`, `landing_page_sections`, `landing_forms`, `landing_form_fields`, `landing_submissions`,
`landing_submission_notes`, `landing_page_versions`, `landing_page_schedules`, `landing_brandings`,
`landing_analytics_events`, `landing_analytics_daily`, `landing_media_assets`, `landing_section_templates`,
`landing_ctas`, `landing_menus`, `landing_menu_items`, `landing_page_revisions`, `landing_slug_redirects`,
`landing_lead_integrations`, `landing_lead_delivery_logs`, `landing_domain_bindings`

### Product / Catalog (migration 000058, menggantikan tabel `billing_*` lama dari 000050)
`product_features`, `product_plans`, `product_plan_prices`, `product_plan_entitlements`

### Subscription (migration 000059, menggantikan tabel `billing_*` lama dari 000051/000053)
`customer_subscriptions`, `subscription_events`

### Billing — invoice & payment (migration 000052–000053, TIDAK berubah oleh refactor)
`billing_invoices`, `billing_invoice_items`, `billing_payments`, `billing_payment_events`

**Total: 52 tabel** (jumlah tabel domain bisnis billing tidak berubah — hanya 6 tabel di-rename/dipecah
menjadi domain Product/Subscription, 4 tabel invoice/payment tetap). Migration 000045–000049 berisi seed
role (`member`, `organization_owner`) dan seed permission tambahan — bukan tabel baru (lihat bagian Seed).
Migration 000050–000057 (`billing_plans`, `billing_plan_prices`, `billing_features`,
`billing_plan_entitlements`, `billing_subscriptions`, `billing_subscription_events`, plus seed & permission
lama) **sudah di-drop** oleh migration 000060 — tetap ada di riwayat migration untuk jejak historis, tapi
tabelnya sendiri sudah tidak ada di database saat ini.

## 2. Urutan Migration (Kelompok Besar)

| Range | Kelompok | Catatan |
|---|---|---|
| 000001–000004 | Permission & user auth dasar | Fondasi RBAC + tabel user |
| 000005–000012 | Notification | Template, log, preference, outbox |
| 000013–000024 | Organization & multi-tenant | Termasuk **000019: fondasi RLS** (`app_current_organization_id()`, `apply_organization_rls()`) |
| 000025–000044 | Landing page | Core tables → forms → versions → schedules → branding → analytics → reusable components (media/CTA/menu) → revisions → integrations → seed data & permission → template repair/backfill |
| 000045–000049 | Seed role & permission tambahan | `member`, `organization_owner`, seed `super_admin` user permission |
| 000050–000057 | Billing (lama, sudah di-drop) | Plan catalog → subscriptions → invoices/payments → events → seed. Tabel plan/feature/subscription dari sini sudah digantikan 000058–000059; invoice/payment tetap dipakai |
| 000058–000062 | Refactor domain-split (2026-07-01) | **000058**: `product_features`/`product_plans`/`product_plan_prices`/`product_plan_entitlements`. **000059**: `customer_subscriptions`/`subscription_events`. **000060**: drop 6 tabel `billing_*` lama + repoint FK `billing_invoices`. **000061**: seed ulang product catalog ke tabel baru. **000062**: rename 10 permission slug (`platform.billing.plan/plan_price/feature/entitlement.*` → `platform.product.*`, `platform.billing.subscription.*` → `platform.subscription.*`) |

Migration **wajib dijalankan berurutan** (`go run ./cmd/migrate -direction up -dir migrations -steps 0`) —
lihat `docs/migration-guide.md` untuk detail cara kerja runner.

## 3. Row-Level Security (RLS) — Cakupan Aktual

**Fondasi** (000019_add_rls_foundation.up.sql):
- Fungsi `app_current_organization_id()` — membaca session variable `app.organization_id`.
- Fungsi `apply_organization_rls(target_table regclass)` — mengaktifkan RLS + membuat 2 policy
  (`organization_isolation_boundary` RESTRICTIVE dan `organization_tenant_access` PERMISSIVE), keduanya
  memvalidasi `organization_id` cocok dengan session variable, termasuk `WITH CHECK` untuk INSERT/UPDATE.

**RLS diterapkan HANYA pada tabel `landing_*`** (dikonfirmasi lewat pencarian pemanggilan
`apply_organization_rls`/`ENABLE ROW LEVEL SECURITY` di seluruh migration): `landing_pages`,
`landing_page_sections`, `landing_forms`, `landing_form_fields`, `landing_submissions`,
`landing_submission_notes`, dan tabel landing lainnya yang dibuat di migration 000025–000044.

**RLS TIDAK diterapkan pada:**
- Tabel `organization_*` (`organizations`, `organization_memberships`, `organization_domains`,
  `organization_entitlements`, `organization_usage_counters`, `organization_impersonation_sessions`)
- Tabel `product_*` (`product_features`, `product_plans`, `product_plan_prices`,
  `product_plan_entitlements`) — tabel katalog global, tidak per-organization, RLS memang tidak relevan
- Tabel `customer_subscriptions`, `subscription_events` (organization-scoped, TIDAK ada RLS)
- Tabel `billing_*` (`billing_invoices`, `billing_invoice_items`, `billing_payments`,
  `billing_payment_events` — organization-scoped, TIDAK ada RLS)
- Tabel user/auth (`users`, `sessions`, dll — memang tidak per-organization secara langsung)

**Implikasi**: isolasi tenant di tabel `organization_*`, `customer_subscriptions`/`subscription_events`, dan
`billing_*` bergantung sepenuhnya pada filter `organization_id` yang ditulis manual di setiap query
repository. **Ini adalah gap arsitektur yang perlu diverifikasi eksplisit**: apakah ini keputusan desain
sadar (mis. karena tabel-tabel ini hanya diakses lewat service layer yang selalu inject `organization_id`,
dan RLS dianggap redundant) atau memang belum sempat diterapkan. Detail rekomendasi ada di
[next-development-tasks.md](next-development-tasks.md) (P0/P1 tergantung hasil klarifikasi). Lihat juga
`docs/multi-tenant-rls.md` untuk desain RLS yang sudah didokumentasikan sebelumnya.

Untuk query manual yang butuh scope RLS, session variable harus di-set eksplisit:
```sql
SET app.organization_id = '<org_uuid>';
```
Di level aplikasi, ini ditangani lewat `internal/platform/database/tenant_transaction.go` — pastikan alur
request melewati middleware resolver tenant (`ResolveAuthenticatedOrganization`/`ResolvePublicOrganization`)
sebelum repository landing dipanggil.

## 4. Relasi & Constraint Penting

- **Composite FK pola landing**: banyak tabel landing memakai composite foreign key
  `(organization_id, landing_page_id) → landing_pages(organization_id, id)` alih-alih FK sederhana ke `id`
  saja — ini memastikan konsistensi tenant di level FK, bukan cuma RLS. Contoh: `landing_page_sections`,
  `landing_forms`.
- **`organizations`**: constraint `CHECK` untuk slug format (lowercase alphanumeric + hyphen), constraint
  "hanya boleh 1 organisasi bertipe platform" (`idx_organizations_single_platform`), dan trigger
  `trg_prevent_platform_organization_removal` yang mencegah penghapusan/modifikasi tipe organisasi platform.
- **`organization_memberships`**: unique `(user_id, organization_id)`; trigger
  `trg_increment_membership_version` dan `trg_increment_membership_version_for_role` — versi membership
  naik otomatis saat status/owner/role berubah. Migration baru yang menyentuh tabel ini harus memastikan
  trigger tetap konsisten (tidak menyebabkan version drift).
- **`organization_entitlements`**: trigger `trg_increment_entitlement_version`; constraint khusus
  `platform_override` (wajib punya `effective_until`, `created_by`, `reason`).
- **`organization_domains`**: unique `canonical_host` (di antara domain aktif), unique 1 domain primary per
  organisasi (`idx_organization_domains_primary_active_unique`), validasi format host via constraint.
- **`customer_subscriptions`**: unique partial index — hanya boleh **1 subscription aktif** per organisasi
  (status IN trialing/active/past_due/grace_period). Ini pola standar unique index Postgres — aman terhadap
  race condition (insert konkuren kedua akan menunggu lock index lalu gagal dengan unique_violation, bukan
  membuat duplikat).
- **`billing_invoices`**: FK komposit ke `customer_subscriptions(id, organization_id)` (bukan lagi ke
  `billing_subscriptions` lama — di-repoint oleh migration 000060).
- **`billing_payments`**: unique `(provider, provider_reference)` untuk mencegah duplikasi pemrosesan
  payment dari webhook yang sama.
- **`billing_payment_events`**: unique `(provider, provider_event_id)` — idempotency untuk webhook event.
- **`landing_submissions`**: unique `(organization_id, idempotency_key)` untuk mencegah submission ganda
  dari form yang sama.
- **Soft delete**: pola `deleted_at IS NULL` dipakai konsisten di `users`, `organizations`,
  `organization_domains`, dan sebagian besar tabel `landing_*` — index unique banyak yang partial
  (`WHERE deleted_at IS NULL`) agar slug/identifier bisa dipakai ulang setelah soft-delete.

## 5. Seed yang Tersedia

| Seed | Command | Dependency |
|---|---|---|
| Super admin | `go run ./cmd/seed -name super-admin` | Tidak bergantung seed lain — **harus jalan pertama** |
| Platform organization | `go run ./cmd/seed -name platform-organization` | **Bergantung pada seed super-admin** — fungsi `SeedPlatformOrganization` memanggil `findPlatformOwner()` yang mencari user super-admin yang sudah ada di database |

Seeder super-admin (`internal/modules/user/seeder/super_admin.go`) membuat: 60+ permission dasar (naming
`module.resource.action`), role `super_admin` (nama dari `SEED_ADMIN_ROLE`, default `super_admin`) dengan
semua permission ter-assign scope `all`, user super-admin (idempotent by email), auth identity provider
`local`, dan assignment role ke user secara **global** (tanpa `organization_id`).

Seeder platform organization (`internal/modules/organization/seeder/platform_organization.go`) membuat:
organisasi bertipe `platform` (idempotent by slug/ID dari `PLATFORM_ORGANIZATION_*`), domain platform
(auto-verified), membership yang menghubungkan super-admin sebagai owner, entitlement platform.

Migration seed tambahan (bukan lewat `cmd/seed`, tapi langsung sebagai SQL migration):
- `000045_seed_member_role.up.sql` — role `member`
- `000046_seed_organization_owner_role.up.sql` — role `organization_owner`
- `000048_seed_super_admin_user_permissions.up.sql` — memastikan user super-admin (by convention/lookup)
  punya semua permission

**Data default wajib agar aplikasi bisa berjalan**: migration sampai 000062 + seed `super-admin` +
`platform-organization`. Tanpa keduanya, tidak ada user yang bisa login dan tidak ada organisasi platform
untuk resolusi tenant default. Seed katalog produk (5 plan, 27 feature, 8 harga, ~124 entitlement) otomatis
terisi lewat migration 000061 (bukan lewat `cmd/seed`).

## 6. Potensi Masalah Migration / Risiko

| Risiko | Detail | Rekomendasi |
|---|---|---|
| RLS tidak konsisten lintas domain | Lihat bagian 3 — `billing_*`/`customer_subscriptions`/`organization_*` tidak ter-cover RLS | Audit eksplisit + keputusan desain (P0/P1, lihat next-development-tasks.md) |
| Trigger version-increment | `organization_memberships`, `organization_entitlements`, `organization_usage_counters` semua punya trigger auto-increment version. Migration baru yang mengubah struktur tabel ini berisiko merusak trigger tanpa disadari | Selalu jalankan integration test terkait (`membership_repository_integration_test.go`, `entitlement_repository_integration_test.go`) setelah migration baru menyentuh tabel ini |
| Migration besar tanpa transaksi eksplisit terlihat | **Status: Needs code verification** — cek `internal/platform/database/migration` apakah setiap file migration dibungkus transaksi otomatis oleh runner, atau developer harus menulis `BEGIN`/`COMMIT` manual di file SQL. Belum ada keputusan manual untuk poin ini | Baca `docs/migration-guide.md` dan kode runner sebelum menulis migration destructive |
| Duplikasi seed potensial | Seed `super-admin` dan migration `000048_seed_super_admin_user_permissions` sama-sama menyentuh permission user super-admin dari 2 mekanisme berbeda (CLI seeder vs SQL migration) | Klarifikasi mana yang jadi source of truth utama agar tidak ada race/duplikasi saat setup environment baru |
| Modul `payment`/`order` stub tapi `billing` sudah punya `billing_payments` | Jika modul stub tersebut digarap nanti, ada risiko membuat tabel baru yang tumpang tindih fungsi dengan `billing_*` | Cek [module-map.md](module-map.md) catatan risiko sebelum membuat migration baru untuk modul-modul ini |
| Migration 000050–000057 (`billing_plans`/`billing_features`/dst) sudah di-drop tapi filenya tetap ada di `migrations/` | Developer baru bisa salah kira tabel tsb masih ada karena filenya masih terlihat | Selalu jalankan `\d <nama_tabel>` di psql atau cek migration 000058–000062 untuk struktur final sebelum menulis query baru terkait product/subscription |

## 7. Index Penting (Highlight)

- `idx_users_email_active_unique`, `idx_users_username_active_unique` — partial unique (`deleted_at IS NULL`)
- `idx_organizations_slug_active_unique`, `idx_organizations_single_platform` — enforce single platform org
- `idx_organization_domains_canonical_host_unique`, `idx_organization_domains_primary_active_unique`
- `idx_landing_pages_organization_slug_active_unique`, `idx_landing_pages_organization_homepage_unique`
- `idx_customer_subscriptions_one_usable_per_org`, `idx_customer_subscriptions_org_status` (mendukung
  constraint 1 subscription aktif per organisasi)
- `idx_billing_payments_org_status_created`, `idx_billing_invoices_org_status_created` — query dashboard billing

## 8. Catatan Tenant/Organization

- Tabel domain bisnis yang organization-scoped (landing, `customer_subscriptions`/`subscription_events`,
  `billing_*`) punya kolom `organization_id` — konsisten sebagai kunci scoping tenant. Tabel `product_*`
  sengaja **tidak** punya `organization_id` — itu katalog global platform (plan/feature/harga/entitlement
  template), bukan data milik tenant tertentu.
- Organisasi bertipe `platform` (tunggal, di-enforce trigger) merepresentasikan Zyad Cloud sendiri —
  landing page marketing platform memakai module yang sama dengan landing page customer, dibedakan lewat
  `organization_id` yang konkret (lihat `AGENTS.md` bagian Multi-Tenant Workflow).
- `data_placement` (`shared`/`dedicated`) di tabel `organizations` mengindikasikan rencana dukungan
  database-per-tenant untuk enterprise. **Status (dikonfirmasi manual 2026-07-01): Deferred (future
  capability).** Keputusan resmi: "Current target implementation is shared database. Dedicated
  database-per-tenant is deferred as future capability." Fokus implementasi sekarang adalah shared
  database dengan isolasi lewat `organization_id`/`tenant_id` plus query filtering dan authorization guard
  yang aman (lihat bagian 3 di atas soal cakupan RLS dan filter manual) — **bukan** membangun mekanisme
  koneksi database terpisah per tenant. Ini menjadikan audit filter manual/RLS di tabel `organization_*`
  dan `billing_*` (bagian 3 dan bagian 6 baris pertama) lebih prioritas, karena shared database adalah
  arsitektur yang benar-benar dipakai saat ini.
