# Footer Management Traceability Index

Dokumen ini menghubungkan kebutuhan Footer Management dan plug-and-play footer
template ke task development, komponen renderer, schema, test, dan status
implementasi.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `blocked`: menunggu dependency atau keputusan.
- `deferred`: sengaja ditunda.

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-landing-page.md` | Requirement Landing Page secara umum |
| `docs/landing-page-development-tasks.md` | Breakdown development module Landing |
| `docs/landing-page-public-api-contract.md` | Kontrak API publik Landing/renderer |
| `docs/landing-page-traceability-index.md` | Traceability module Landing utama |
| `docs/footer-management-development-tasks.md` | Breakdown detail Footer Management |

## Module Mapping

| Module | Path | Responsibility |
| --- | --- | --- |
| Section renderer | `frontend/src/features/landing/renderer/components/SectionRenderer.vue` | Resolve komponen per `type`+`variant`, terima `content-override` |
| Page renderer | `frontend/src/features/landing/renderer/components/LandingPageRenderer.vue` | Susun urutan section, oper `footerContent` ke section type `footer` |
| Public page host | `frontend/src/features/landing/renderer/pages/DynamicLandingPage.vue` | Fetch `/public/landing/resolve`, derivasi `footerContent` dari Branding+Menus+Settings |
| Section registry | `frontend/src/features/landing/renderer/registry/section-registry.ts` | Katalog komponen per type/variant (plug-and-play) |
| Footer components | `frontend/src/features/landing/renderer/sections/footer/*.vue` | Implementasi tiap variant footer |
| Content admin UI | `frontend/src/features/landing/builder/pages/LandingContentManagementPage.vue` | Picker variant, preset section |
| Settings admin UI | `frontend/src/features/landing/builder/pages/LandingSettingsManagementPage.vue` | Copyright, trust badge, secondary CTA, newsletter form binding |
| Navigation admin UI | `frontend/src/features/landing/builder/pages/LandingNavigationManagementPage.vue` | Menu `location=footer` |
| Brand & Theme admin UI | `frontend/src/features/landing/builder/pages/LandingBrandThemeManagementPage.vue` | `company_name`, `logo_light_url`/`logo_dark_url`, `tagline`, `social_links`, `contact` |
| Section handler | `internal/modules/landing/handler/admin_section_handler.go` | CRUD + reorder section, termasuk `variant` |
| Page handler | `internal/modules/landing/handler/admin_page_handler.go` | `PageSettings` mapper (`pageSettingsFromDomain`/`pageSettingsToDomain`) |
| Page domain | `internal/modules/landing/domain/landing_page.go` | `PageSettings` struct |
| Page DTO | `internal/modules/landing/dto/request.go`, `dto/response.go` | `PageSettingsRequest`/`PageSettingsResponse` |
| Public resolve | `internal/modules/landing/handler/public_landing_handler.go`, `service/resolver_service.go`, `service/resolver_contract.go` | `ResolvedPage{Page, Branding, Sections, Menus, ...}` |
| Section template (deferred) | `internal/modules/landing/handler/admin_template_handler.go`, `service/template_service.go`, `domain/landing_reusable.go` | CRUD `landing_section_templates`, `InstantiateToPage` |
| Migrations | `migrations/000030_create_landing_reusable_tables.*.sql` dst. | Schema `landing_section_templates`, `landing_page_sections.variant` |

## Requirement Traceability

| Requirement | Task ID | Component/API | Status |
| --- | --- | --- | --- |
| Breakdown zona konten footer & sumber data | FTR-0001 | `docs/footer-management-development-tasks.md` §Footer Content Model | done |
| Tetapkan katalog variant v1 | FTR-0002 | §Footer Template Catalog v1 | done |
| Brand zone (nama, logo, tagline) dari Brand & Theme | FTR-FE-011 | `useFooterContent()`, `LandingBrandThemeManagementPage.vue` | done |
| Navigation zone (kolom link) dari menu `location=footer` | FTR-FE-011 | `useFooterContent()`, `LandingNavigationManagementPage.vue` | done |
| Legal/copyright dari Settings | FTR-FE-011 | `LandingSettingsManagementPage.vue` (`footer_copyright_text`) | done |
| Trust badges dari Settings | FTR-FE-014 | `LandingSettingsManagementPage.vue` (`trust_badges`), `FooterMega.vue` | done |
| Secondary CTA dari Settings + CTA module | FTR-FE-014 | `LandingSettingsManagementPage.vue` (`secondary_cta_tracking_key`), `landing_ctas`, `FooterMega.vue` | done (butuh `ResolvedPage.CTAs` baru di FTR-BE-002 — tidak ada jalur publik CTA sebelumnya) |
| Newsletter form binding | FTR-FE-013 | `LandingSettingsManagementPage.vue` (`newsletter_form_id`), `landing_forms`, `FooterNewsletter.vue` | done |
| Social/contact zone (backlog) | FTR-ADV-002 | `LandingBranding.social_links`/`.contact` | deferred (field sudah ada di `FooterContent`/`useFooterContent()`, belum dirender variant manapun) |
| Section footer content selalu ter-override otomatis saat render | — | `LandingPageRenderer.vue` (`content-override`), `SectionRenderer.vue` | done |
| Validasi `variant` allow-list per section type | FTR-BE-001 | `section_service.go` (`validateFooterVariant`) | done |
| Resolve payload selalu bawa Branding/Menus/Settings | FTR-BE-002 | `public_landing_handler.go`, `resolver_service.go`, `resolver_repository.go` | done (bug ditemukan & diperbaiki: `settings` tidak ter-select; lihat Decision Log) |
| Kontrak `FooterContent` bersama | FTR-FE-010 | `shared/types/landing.types.ts` | done |
| Composable `useFooterContent()` | FTR-FE-011 | `renderer/composables/useFooterContent.ts` | done |
| Variant `default` (Standard) | — | `renderer/sections/footer/LandingFooter.vue` | done |
| Variant `simple` | FTR-FE-012 | `renderer/sections/footer/FooterSimple.vue` | done |
| Variant `newsletter` | FTR-FE-013 | `renderer/sections/footer/FooterNewsletter.vue` | done |
| Variant `mega` | FTR-FE-014 | `renderer/sections/footer/FooterMega.vue` | done |
| Registrasi semua variant di registry | FTR-FE-015 | `section-registry.ts` | done |
| Picker variant di admin | FTR-FE-020 | `LandingContentManagementPage.vue` | done |
| Preview/label per variant | FTR-FE-021 | `LandingContentManagementPage.vue` | done |
| Contextual field Settings per variant | FTR-FE-022 | `LandingSettingsManagementPage.vue` | planned (nice-to-have, belum dikerjakan) |
| `secondary_cta_tracking_key` tidak silent-dropped | — | `domain/landing_page.go`, `dto/request.go`, `dto/response.go`, `admin_page_handler.go` | done |
| `newsletter_form_id` tidak silent-dropped | — | sda | done |
| Tenant-authored footer template (v2) | FTR-ADV-001 | `admin_template_handler.go`, `landing_section_templates` | deferred |

## Component/API Index

| File | Task ID | Status | Notes |
| --- | --- | --- | --- |
| `renderer/sections/footer/LandingFooter.vue` | — | done | Variant `default`, existing sebelum fitur ini; diupdate memakai type `FooterContent` |
| `renderer/sections/footer/FooterSimple.vue` | FTR-FE-012 | done | Baru — brand+copyright saja |
| `renderer/sections/footer/FooterNewsletter.vue` | FTR-FE-013 | done | Baru — submit langsung ke `POST /public/landing/forms/:formKey/submissions` |
| `renderer/sections/footer/FooterMega.vue` | FTR-FE-014 | done | Baru — trust badges + secondary CTA |
| `renderer/registry/section-registry.ts` (`footer` map) | FTR-FE-015 | done | `{ default, LandingFooter, simple, newsletter, mega }` |
| `renderer/composables/useFooterContent.ts` | FTR-FE-011 | done | Ekstraksi dari `DynamicLandingPage.vue` + resolusi trustBadges/secondaryCta/newsletterFormId baru |
| `builder/pages/LandingContentManagementPage.vue` (`footerVariantOptions`, picker) | FTR-FE-020, FTR-FE-021 | done | Card picker 4 variant, simpan via `style.variant` |
| `PATCH /admin/landing-pages/:id/sections/:sectionId` (`style.variant`) | FTR-BE-001, FTR-FE-020 | done | Endpoint sudah ada; validasi allow-list di service layer + UI picker selesai |
| `GET /public/landing/resolve` | FTR-BE-002 | done | `Branding`/`Menus`/`Page.Settings` (bug fix: `settings` tidak ter-select) + `CTAs` baru untuk resolusi `secondary_cta_tracking_key` |
| `PATCH /admin/landing-pages/:id` (`settings`) | — | done | `secondary_cta_tracking_key`/`newsletter_form_id` sudah round-trip |

## Schema Traceability

| Schema Object | Related Task | Status | Notes |
| --- | --- | --- | --- |
| `landing_page_sections.style` (`variant` key, jsonb) | FTR-BE-001, FTR-FE-015, FTR-FE-020 | available | **Koreksi**: tidak ada kolom `variant` tersendiri di schema — variant footer disimpan sebagai key `variant` di dalam kolom `style` yang sudah ada (konsisten dengan fallback `SectionRenderer.vue`), tidak ada migration baru |
| `landing_pages.settings` (jsonb) | — | available | Menyimpan `footer_copyright_text`, `trust_badges`, `secondary_cta_tracking_key`, `newsletter_form_id` |
| `landing_brandings.*` | — | available | `company_name`, `logo_light_url`, `logo_dark_url`, `tagline`, `social_links`, `contact` |
| `landing_menus` / `landing_menu_items` (`location='footer'`) | — | available | Dipakai `useFooterContent()` |
| `landing_ctas` | FTR-FE-014 | available | Resolusi `secondary_cta_tracking_key` → label/url via `ResolvedPage.CTAs` (baru, FTR-BE-002) |
| `landing_forms` | FTR-FE-013 | available | `newsletter_form_id` dipakai langsung sebagai `formKey` di `POST /public/landing/forms/:formKey/submissions` — tidak ada lookup form metadata (label/consent copy) di v1 |
| `landing_section_templates` (`section_type='footer'`) | FTR-ADV-001 | available, tidak dipakai v1 | Lihat Design Decision di development-tasks |

**Tidak ada migration baru yang dibutuhkan untuk v1** (task `FTR-BE-003`).

## Test Traceability

| Test Area | Task ID | Minimum Coverage | Status |
| --- | --- | --- | --- |
| Variant allow-list validation | FTR-BE-001 | Variant valid diterima, variant tak dikenal ditolak 422, kosong = backward compatible | done (`section_service_unit_test.go`) |
| Resolve payload completeness | FTR-BE-002 | Branding/Menus/Settings/CTAs selalu ada di payload page dengan section footer | done (`resolver_service_test.go`, tag `integration`) |
| Footer component props minimal | FTR-TEST-001 | Logo kosong, columns kosong, trustBadges/secondaryCta/newsletter kosong per variant | planned |
| Manual E2E per variant | FTR-TEST-002 | 4 variant dicoba di subdomain tenant nyata | planned |
| Newsletter submit | FTR-FE-013 | Submit form dari `FooterNewsletter.vue` tercatat di `landing_submissions` | planned |

## Dependency Traceability

| Dependency | Related Task | Status | Notes |
| --- | --- | --- | --- |
| Section registry plug-and-play mechanism | FTR-FE-015 | done (mekanisme), planned (entri footer) | Sudah dipakai section type lain (hero, cta, dst) |
| `PageSettings` field `secondary_cta_tracking_key`/`newsletter_form_id` | FTR-FE-013, FTR-FE-014 | done | Prasyarat sudah selesai sebelum dokumen ini dibuat |
| Public resolve mengirim `Menus`/`Branding` | FTR-FE-011 | done | Prasyarat derivasi footer sudah berjalan |
| Forms module (submit + submission list) | FTR-FE-013 | available | Reuse `PublicLandingHandler.SubmitForm`, tidak ada perubahan |
| CTA module (`landing_ctas`) | FTR-FE-014 | available | Reuse endpoint CTA existing, tidak ada perubahan |
| `landing_section_templates` CRUD | FTR-ADV-001 | available, deferred | Backend siap dipakai kapan pun v2 dikerjakan |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-07-15 | Footer content model memakai 8 zona (Brand, Navigation, Legal, Trust badges, Secondary CTA, Newsletter, Social*, Contact*) | Mencakup pola footer umum di web modern tanpa menambah data model baru — 6 zona pertama sudah didukung menu admin existing |
| 2026-07-15 | Katalog footer v1 statis di frontend (4 variant: default/simple/newsletter/mega), bukan `landing_section_templates` | `landing_section_templates` semantiknya whole-page design family milik 32 template platform; reuse langsung berisiko mencampur konsep dan merusak data existing |
| 2026-07-15 | Tidak ada migration baru untuk v1 | Semua field yang dibutuhkan (`variant`, `settings.*`, branding, menu) sudah ada sebelum dokumen ini dibuat |
| 2026-07-15 | Tenant-authored footer template ditunda ke v2 (`FTR-ADV-001`) | Backend `landing_section_templates`+`InstantiateToPage` sudah siap tapi butuh keputusan produk terpisah soal UX-nya |
| 2026-07-16 | Variant footer disimpan di `style.variant`, BUKAN kolom `landing_page_sections.variant` terpisah | Riset ulang saat implementasi FTR-BE-001 menemukan kolom itu tidak pernah ada di schema/migration manapun — dokumen versi sebelumnya salah asumsi. `SectionRenderer.vue` sudah fallback ke `style.variant`/`style.renderer_component`, jadi tetap konsisten dengan keputusan "tidak ada migration baru" |
| 2026-07-16 | Tambah `ResolvedPage.CTAs` ke public resolve payload | Diperlukan supaya `secondary_cta_tracking_key` (FTR-FE-014) bisa di-resolve ke label/url tanpa endpoint admin terpisah (yang butuh auth dan tidak bisa diakses pengunjung publik) — sebelumnya tidak ada jalur publik untuk `landing_ctas` sama sekali |
| 2026-07-16 | Perbaiki bug: `resolver_repository.go` tidak men-select kolom `settings` | Audit FTR-BE-002 menemukan `ResolveBySlug`/`ResolveByDomain` tidak pernah mengambil `landing_pages.settings`, jadi `Page.Settings` (termasuk `footer_copyright_text`, `trust_badges`) selalu kosong di payload publik meski dokumen sebelumnya mengklaim ini "done (behavior)" |
| 2026-07-16 | `FooterNewsletter.vue` memanggil `POST /public/landing/forms/:formKey/submissions` langsung, bukan "reuse komponen form section publik" | Riset menemukan tidak ada section type `form` yang terdaftar di `section-registry.ts`/tidak ada komponen renderer form publik untuk direuse — dokumen versi sebelumnya berasumsi komponen itu ada. `newsletterFormId` dipakai langsung sebagai `formKey` tanpa lookup metadata form (label tombol, salinan consent generik dipakai) |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-07-15 | Footer auto-derivation dari Branding+Navigation+Settings (baseline sebelum dokumen ini) | `DynamicLandingPage.vue`, `LandingPageRenderer.vue`, `SectionRenderer.vue` | Menutup bug footer kosong/logo platform; jadi fondasi `useFooterContent()` (`FTR-FE-011`) |
| 2026-07-15 | Field `secondary_cta_tracking_key`/`newsletter_form_id` ditambahkan ke `PageSettings` (baseline sebelum dokumen ini) | `internal/modules/landing/domain/landing_page.go`, `dto/request.go`, `dto/response.go`, `admin_page_handler.go` | Menutup bug field silent-dropped; prasyarat `FTR-FE-013`/`FTR-FE-014` |
| 2026-07-15 | Ditulis dokumen requirement + task + traceability Footer Management | `docs/footer-management-development-tasks.md`, `docs/footer-management-traceability-index.md` | Dokumen ini — belum ada implementasi task `FTR-BE-*`/`FTR-FE-01x`/`FTR-FE-02x` |
| 2026-07-16 | Implementasi Phase 1 Backend selesai (FTR-BE-001/002/003) | `internal/modules/landing/service/section_service.go` (+`section_service_unit_test.go`), `internal/modules/landing/repository/resolver_repository.go`, `internal/modules/landing/service/resolver_service.go` (+`resolver_service_test.go`), `internal/modules/landing/service/resolver_contract.go`, `internal/app/app.go` | `go test ./internal/modules/landing/...` dan `go test -tags integration ./internal/modules/landing/...` hijau. Termasuk 2 bug fix nyata yang ditemukan saat audit: kolom `settings` tidak ter-select di resolve, dan tidak ada jalur publik untuk CTA |
| 2026-07-16 | Implementasi Phase 2+3 Frontend selesai (FTR-FE-010 s.d. FTR-FE-021) | `frontend/src/features/landing/shared/types/landing.types.ts` (`FooterContent`), `renderer/composables/useFooterContent.ts` (+spec), `renderer/sections/footer/{FooterSimple,FooterNewsletter,FooterMega}.vue` (+spec masing-masing), `renderer/sections/footer/LandingFooter.vue`, `renderer/registry/section-registry.ts`, `renderer/pages/DynamicLandingPage.vue`, `builder/pages/LandingContentManagementPage.vue` | `npx vue-tsc --noEmit` bersih, `npx vitest run` 23 test hijau. `FTR-FE-022` (contextual Settings field per variant) belum dikerjakan (nice-to-have); `FTR-TEST-002` (manual E2E per variant di subdomain tenant nyata) belum dijalankan |

## Update Rules

- Saat task `FTR-*` mulai dikerjakan, ubah status menjadi `in_progress` di kedua
  dokumen (development-tasks dan traceability-index).
- Saat endpoint/variant baru selesai, update
  `docs/landing-page-public-api-contract.md` dan `api/openapi.yaml`.
- Saat komponen footer baru selesai, update tabel Component/API Index dan
  Requirement Traceability jadi `done`.
- Saat ada keputusan teknis baru (mis. jadi pakai `landing_section_templates`),
  tambahkan entry ke Decision Log sebelum mengubah task terkait.
- Saat task selesai, ubah status menjadi `done` dan catat command/test yang
  dijalankan pada Development History.
