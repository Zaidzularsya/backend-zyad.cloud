# Module Map — zyad.cloud Backend

> Peta lengkap modul di `internal/modules/`. Untuk detail requirement per fitur, rujuk dokumen
> `reference-*.md` yang sudah ada (lihat tabel referensi di [repository-context.md](repository-context.md)).
> Status modul ditentukan dari isi folder aktual (bukan asumsi), per 2026-07-01 (diperbarui setelah refactor
> domain-split `billing` → `product`/`subscription`/`billing`, lihat
> [product-subscription-billing-concept.md](product-subscription-billing-concept.md)).

## Ringkasan Status

| Status | Arti |
|---|---|
| **Stabil** | Layer handler/service/repository lengkap + test, dipakai di production flow |
| **WIP** | Sudah ada implementasi nyata tapi belum lengkap/belum semua integrasi selesai |
| **Stub** | Hanya scaffold (`doc.go`/`errors.go`/`routes.go` kosong atau nyaris kosong), belum ada business logic |
| **Legacy/kosong** | Folder ada tapi 0 file — sisa struktur lama, tidak dipakai |

## Tabel Module Map

| Module | Lokasi | Tujuan | Endpoint terkait | Tabel DB terkait | Dependency ke modul lain | Status | Catatan risiko |
|---|---|---|---|---|---|---|---|
| **user** | `internal/modules/user/` | Autentikasi, manajemen user, role assignment, session, audit log | `/api/v1/auth/*`, `/api/v1/users/me*`, `/api/v1/admin/users/*`, `/api/v1/admin/login-histories`, `/api/v1/admin/audit-logs` | `users`, `user_profiles`, `auth_identities`, `sessions`, `refresh_tokens`, `password_reset_tokens`, `email_verification_tokens`, `otp_codes`, `login_histories`, `audit_logs`, `roles`, `permissions`, `role_permissions`, `user_roles`, `user_permissions` | `organization` (membership context), dipakai oleh semua modul lain (auth context) | **Stabil** | Seeder `super_admin.go` adalah single source permission dasar — perubahan daftar permission harus lewat seeder + migration seed, bukan hardcode di modul lain |
| **organization** | `internal/modules/organization/` | Multi-tenant core: organisasi, membership, domain binding+verifikasi, entitlement, impersonation, switch org | `/api/v1/organization*`, `/api/v1/platform/organizations*`, `/api/v1/users/me/organizations`, `/api/v1/users/me/switch-organization`, `/api/v1/organization/domains*` | `organizations`, `organization_domains`, `organization_memberships`, `organization_entitlements`, `organization_usage_counters`, `organization_impersonation_sessions`, `reserved_subdomains` | tidak bergantung modul lain (fondasi) — didepend oleh `landing`, `billing`, `user` | **Stabil** | Tabel `organization_*` **tidak** memakai RLS — isolasi tenant bergantung filter manual di repository; trigger version-increment (`organization_memberships`, `organization_entitlements`) harus tetap konsisten kalau ada migration baru |
| **landing** | `internal/modules/landing/` | Landing page builder: page, section, template, form, branding, domain binding, publish/schedule, revision, analytics, media, navigasi, CTA, lead delivery | `/api/v1/admin/landing-pages*`, `/api/v1/admin/landing/*` (branding/ctas/domain-bindings/media/menus/section-templates/theme), `/api/v1/admin/landing-submissions*`, `/api/v1/public/landing/*` | `landing_pages`, `landing_page_sections`, `landing_page_versions`, `landing_page_schedules`, `landing_page_revisions`, `landing_forms`, `landing_form_fields`, `landing_submissions`, `landing_submission_notes`, `landing_brandings`, `landing_analytics_events`, `landing_analytics_daily`, `landing_media_assets`, `landing_section_templates`, `landing_ctas`, `landing_menus`, `landing_menu_items`, `landing_domain_bindings`, `landing_slug_redirects`, `landing_lead_integrations`, `landing_lead_delivery_logs` | `organization` (domain models, entitlement check via billing) | **Stabil, aktif dikembangkan** | Satu-satunya modul dengan RLS penuh; folder `internal/modules/landingpage/` (kosong) jangan dikira modul aktif |
| **product** | `internal/modules/product/` | Katalog produk: feature master, plan, harga plan, entitlement per plan (template, bukan runtime) | `/api/v1/platform/product/*` (plans/features/plans/:id/prices/plans/:id/entitlements) | `product_features`, `product_plans`, `product_plan_prices`, `product_plan_entitlements` | `organization` (tidak langsung — dikonsumsi lewat `subscription`) | **Refactor dari `billing`** (2026-07-01) | Hasil pemisahan domain dari modul `billing` lama — lihat [product-subscription-billing-concept.md](product-subscription-billing-concept.md); permission `platform.product.*` |
| **subscription** | `internal/modules/subscription/` | Subscription aktif organization ke plan, audit trail lifecycle, sinkronisasi entitlement ke organization, guard feature/quota | `/api/v1/platform/subscriptions*` | `customer_subscriptions`, `subscription_events` | `product` (baca plan/entitlement), `organization` (sinkron ke `organization_entitlements`/`organization_usage_counters`) | **Refactor dari `billing`** (2026-07-01) | Berisi `EntitlementSink` (sync ke `organization_entitlements`) dan `SubscriptionGuardService` (dipakai `organization`/`landing` untuk cek quota); permission `platform.subscription.*` |
| **billing** | `internal/modules/billing/` (ramping) | Invoice, payment, webhook payment provider, composition self-service billing tenant | `/api/v1/platform/billing/invoices*`, `/api/v1/app/billing/*` (current-plan/usage/invoices/upgrade/cancel) | `billing_invoices`, `billing_invoice_items`, `billing_payments`, `billing_payment_events` | `product` + `subscription` (via `TenantBillingService` composition layer, untuk route `/app/billing/*`) | **Refactor** (ramping dari modul `billing` lama, 2026-07-01) | Payment gateway (Xendit) belum lengkap terintegrasi; tidak ada RLS di tabel `billing_*`; route `/app/billing/*` & permission `organization.billing.*` TIDAK berubah oleh refactor |
| account | `internal/modules/account/` | Akun/profil/preferensi (rencana) | — | — | — | **Stub** | Perlu klarifikasi scope vs `user` module (potensi tumpang tindih tanggung jawab "akun") |
| asset | `internal/modules/asset/` | Aset fisik/digital (router, ODP, perangkat) | — | — | — | **Stub** | Terkait potensi produk ISP/network (di luar scope landing/billing saat ini) |
| contract | `internal/modules/contract/` | Kontrak pelanggan & dokumen | — | — | — | **Stub** | — |
| **crm** | `internal/modules/crm/` | CRM tenant-only: lead, contact/customer, company, deal, pipeline, activity, quotation, invoice CRM, integration | `/api/v1/app/crm/*` | `crm_companies`, `crm_contacts`, `crm_leads`, `crm_pipelines`, `crm_pipeline_stages`, `crm_deals`, `crm_activities`, `crm_quotations`, `crm_quotation_items`, `crm_invoices`, `crm_invoice_items`, `crm_integrations` | `organization` (tenant context), `subscription` (entitlement guard `crm.*`) | **Stabil (BE+FE)** | Backend dan frontend lengkap untuk 10 resource (Fase 0-5 selesai). Belum ada unit/integration test otomatis. Lihat `docs/reference-crm.md` "Status Akhir" untuk daftar lengkap yang ditunda (import/export/merge, price/amount masking, openapi.yaml, dst); **hanya organization tipe `customer`** yang boleh akses (beda dari `landing` yang bisa diakses kedua tipe); `crm_invoices` JANGAN tertukar dengan `billing_invoices` (beda pihak — lihat Naming Conflict di reference-crm.md) |
| dashboard | `internal/modules/dashboard/` | Statistik, grafik, ringkasan | — | — | — | **Stub** | Butuh data dari modul lain (landing analytics, billing) sebelum bisa digarap |
| landingpage | `internal/modules/landingpage/` | Tidak jelas — folder 0 file | — | — | — | **Legacy/kosong** | Rekomendasi: hapus atau dokumentasikan alasan dipertahankan (lihat next-development-tasks.md) |
| mikrotik | `internal/modules/mikrotik/` | Data router MikroTik sisi aplikasi | — | — | — | **Stub** | Client API sudah ada di `internal/platform/mikrotik/` (platform layer), modul domain belum |
| newsaggregator | `internal/modules/newsaggregator/` | Agregasi berita/konten eksternal | — | — | — | **Stub** | — |
| order | `internal/modules/order/` | Order lifecycle, approval, cancellation | — | — | — | **Stub** | Berpotensi overlap dengan `billing`/`subscription` (invoice/subscription) — perlu batasan jelas sebelum digarap |
| payment | `internal/modules/payment/` | Payment gateway, webhook, reconciliation (rencana) | — | — | — | **Stub** | Fungsi pembayaran nyata sudah ada di `internal/modules/billing/` + `internal/platform/xendit/` — klarifikasi dulu apakah modul ini akan diduplikasi atau memang scope beda (mis. non-billing payment) |
| provisioning | `internal/modules/provisioning/` | Aktivasi/deaktivasi layanan teknis | — | — | — | **Stub** | — |
| radius | `internal/modules/radius/` | RADIUS user/profile/session/AAA | — | — | — | **Stub** | — |
| resource | `internal/modules/resource/` | IP pool, VLAN, bandwidth profile | — | — | — | **Stub** | — |

## Detail Modul Aktif

### user
- **Layer**: `handler/` (`auth_handler.go`, `user_handler.go`), `service/` (`auth_service.go`, `user_service.go`),
  `repository/` (`auth_repository.go`, `user_repository.go`), `model/`, `dto/`, `seeder/` (`super_admin.go`).
- **Interface kunci**: `AuthService.Login/GoogleAuth/Logout/ChangePassword/ForgotPassword/ResetPassword`.
- **Test**: `auth_service_test.go`, `user_service_test.go`, `super_admin_test.go`,
  `auth_repository_integration_test.go`, `user_repository_integration_test.go`, `user_handler_test.go`,
  plus test model (`permission_test.go`, `session_test.go`, `user_status_test.go`).

### organization
- **Layer**: `model/` (`organization.go`, `domain.go`, `membership.go`, `entitlement.go`,
  `impersonation_session.go`), `service/` (81+ file termasuk resolver), `repository/`, `handler/` (12 file),
  `dto/`, `seeder/` (`platform_organization.go`).
- **Interface kunci**: `AuthenticatedResolver.ResolveAuthenticatedOrganization`,
  `PublicHostResolver.ResolvePublicHost(Detail)`, `DomainService` (Create/Verify/Activate/SetPrimary/Delete),
  `MembershipService` (Invite/Accept/ChangeStatus/SyncRoles/TransferOwnership).
- **Test**: cakupan luas — resolver (unit + integration), domain, membership, entitlement, lifecycle,
  platform, onboarding, switch, worker resolver, semuanya punya pasangan unit + integration test.

### landing
- **Layer**: `domain/` (model + konstanta status/tipe), `service/` (52 file: page/section/template/domain/
  branding/form/publish/revision/analytics/media/navigation/CTA/resolver dst.), `repository/` (42 file),
  `handler/` (17 file admin + public), `dto/`, `routes/`.
- **Interface kunci**: `PageService`, `DomainService`, `SectionService`, plus service tambahan
  (Analytics/Branding/Form/Template/Revision/Publish/Delivery/Visibility/Media/Navigation/CTA/Resolver).
- **Test**: sangat luas, termasuk unit test baru per 2026-06-30 (`domain_service_unit_test.go`,
  `section_service_unit_test.go`, `template_service_unit_test.go`, `page_service_test.go`) — indikasi modul
  ini sedang aktif digarap/di-harden.

### product
- **Layer**: `model/` (`plan.go`, `feature.go`), `repository/` (`plan_repository.go`,
  `feature_repository.go`, `plan_entitlement_repository.go`), `service/` (`plan_service.go`,
  `feature_service.go`, `plan_entitlement_service.go`), `handler/` (`platform_product_handler.go`), `dto/`,
  `errors.go` (PLAN_NOT_FOUND, FEATURE_NOT_FOUND, dst).
- **Interface kunci**: `PlanService`, `FeatureService`, `PlanEntitlementService`.
- Hasil pemindahan dari `internal/modules/billing/` (Fase D refactor domain-split, 2026-07-01).

### subscription
- **Layer**: `model/` (`subscription.go`, `event.go`), `repository/` (`subscription_repository.go`,
  `entitlement_sink.go`), `service/` (`subscription_service.go`, `subscription_guard_service.go`),
  `handler/` (`platform_subscription_handler.go`), `dto/`, `errors.go` (SUBSCRIPTION_NOT_FOUND,
  SUBSCRIPTION_INACTIVE, SUBSCRIPTION_SUSPENDED, QUOTA_EXCEEDED, dst).
- **Interface kunci**: `SubscriptionService`, `EntitlementSink` (sync ke `organization_entitlements`,
  `SyncPlanEntitlements`/`ExpirePlanEntitlements`), `SubscriptionGuardService` (rename dari
  `BillingGuardService` — enforcement feature/quota untuk `organization`/`landing`).
- Hasil pemindahan dari `internal/modules/billing/` (Fase D refactor domain-split, 2026-07-01).

### billing (ramping)
- **Layer**: `model/` (`invoice.go`, `payment.go`), `repository/` (`invoice_repository.go`,
  `payment_repository.go`), `service/` (`invoice_service.go`, `payment_service.go`,
  `tenant_billing_service.go` — composition layer memanggil `product`+`subscription`), `handler/`
  (`platform_billing_handler.go` ramping hanya invoice, `tenant_billing_handler.go` tetap), `dto/`,
  `errors.go` (INVOICE_NOT_FOUND, PAYMENT_ALREADY_PROCESSED, PAYMENT_EVENT_ALREADY_PROCESSED, dst).
- **Interface kunci**: `InvoiceService`, `PaymentService`, `TenantBillingService` (satu-satunya yang lintas
  domain, untuk route `/app/billing/*` yang tetap satu permukaan customer-facing).
- **Referensi requirement lengkap (riwayat, sebelum refactor)**: `docs/billing-plan-concept-reference.md`,
  `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md`. **Struktur final
  domain-split**: `docs/product-subscription-billing-concept.md`,
  `docs/product-subscription-billing-refactor-plan.md`, `docs/product-subscription-billing-traceability.md`.

## Modul Stub — Checklist Sebelum Mulai Menggarap

Sebelum mengimplementasi salah satu dari 11 modul stub (di luar `landingpage` yang berstatus
legacy/kosong), lakukan:
- [ ] Cek apakah ada overlap tanggung jawab dengan modul aktif (lihat kolom "Catatan risiko" di tabel di atas)
- [ ] Cek apakah sudah ada requirement doc terkait di `docs/` (grep nama modul) — jika belum ada, buat
  `reference-<modul>.md` mengikuti pola dokumen existing sebelum mulai coding
- [ ] Pastikan pola layer `handler/service/repository/dto/model` diikuti (lihat `AGENTS.md`)
- [ ] Untuk modul yang menyimpan data ter-scope organization, tentukan sejak awal apakah perlu RLS
  (ikuti pola `landing`) atau cukup filter manual (ikuti pola `organization`/`billing`) — dan dokumentasikan
  keputusannya
