# Footer Management & Plug-and-Play Footer Template Development Tasks

Dokumen ini adalah task development khusus untuk kebutuhan Footer Management dan
pemilihan template/variant section footer yang plug-and-play pada module Landing Page.

Dokumen ini melengkapi:

- `docs/reference-landing-page.md`
- `docs/landing-page-development-tasks.md`
- `docs/landing-page-public-api-contract.md`
- `docs/landing-page-traceability-index.md`
- `docs/footer-management-traceability-index.md`

## Tujuan

Menjadikan footer landing page sebagai unit yang benar-benar dikelola tenant
(bukan section kosong yang kebetulan dirender), dengan dua kebutuhan utama:

1. **Footer Management** — konten footer (brand, link, copyright, trust badge,
   secondary CTA, newsletter) dikonfigurasi lewat menu admin yang sudah ada
   (Brand & Theme, Navigation, Settings) dan otomatis muncul benar di halaman publik,
   tanpa duplikasi tempat edit.
2. **Plug-and-play footer template** — tenant dapat memilih dari beberapa layout
   footer siap pakai (variant) tanpa perlu menulis konten mentah section secara
   manual, dan developer dapat menambah layout baru tanpa mengubah arsitektur inti.

Scope utama:

- Definisikan model konten footer secara lengkap dan sumber datanya masing-masing.
- Definisikan katalog template/variant footer v1 dan komponen rendernya.
- Sediakan picker variant di admin (menu Content) yang menulis ke
  `landing_page_sections.variant`.
- Pastikan endpoint publik resolve selalu mengirim data yang dibutuhkan derivasi
  footer (branding, menu location=footer, page settings).
- Backward compatible: section footer lama (`variant=null`) tetap tampil benar
  (resolve ke variant `default`).

## Non-Goal

- Tidak membangun visual drag-and-drop editor untuk footer.
- Tidak menambah section type baru di luar `footer` yang sudah ada.
- Tidak mengubah data model Brand & Theme, Navigation, atau Settings yang sudah
  dipakai fitur lain (Header nav, SEO, dsb).
- Tidak membangun UI create/edit/delete untuk tabel `landing_section_templates`
  pada v1 (lihat Design Decision) — didokumentasikan sebagai opsi v2/deferred.
- Tidak menambah migration database untuk v1 (lihat Schema Traceability di
  `footer-management-traceability-index.md`).

## Current Repository Findings

Hasil pembacaan repository saat dokumen ini dibuat (frontend: `frontend.zyad.cloud`,
backend: `zyad.cloud`):

- **Rendering plug-and-play sudah ada sebagian.**
  `frontend/src/features/landing/renderer/components/SectionRenderer.vue` resolve
  komponen lewat `resolveSection(type, variant)` dari
  `frontend/src/features/landing/renderer/registry/section-registry.ts`
  (`sectionRegistry[type][variantKey]`, fallback ke key `'default'`, ada juga
  fallback khusus ke variant `'enterprise'` untuk 32 slug design-family platform
  template). Untuk `type: 'footer'`, registry saat ini **hanya punya satu entri**:
  `{ default: LandingFooter, LandingFooter: LandingFooter }`
  (`renderer/sections/footer/LandingFooter.vue`).
- **Field `variant` sudah plumbed penuh tapi tanpa UI.**
  `LandingSection.variant` ada di type (`shared/types/landing.types.ts`), di-mapping
  di `landing.api.ts` (`pick(raw, 'variant', 'Variant', undefined)`), dan dipakai
  `SectionRenderer.vue` untuk resolve component. Satu-satunya tempat field ini
  ditampilkan di UI adalah `LandingTemplatesManagementPage.vue` (read-only badge,
  halaman ini `hiddenFromMenu: true`). **Tidak ada dropdown/selector di manapun**
  bagi admin tenant untuk memilih atau mengubah variant sebuah section.
- **Backend sudah punya CRUD section template generik**, belum dipakai untuk footer
  picker. Tabel `landing_section_templates` (migration
  `migrations/000030_create_landing_reusable_tables.up.sql`), kolom:
  `id, organization_id, name, description, section_type (CHECK termasuk 'footer'),
  content jsonb, style jsonb, created_by, updated_by, created_at, updated_at,
  deleted_at`. Service: `internal/modules/landing/service/template_service.go`
  (`Create/FindByID/List/Update/Delete/InstantiateToPage`). Handler:
  `internal/modules/landing/handler/admin_template_handler.go`, route
  `/admin/landing/section-templates`, permission `landing.section_template.manage`.
  `InstantiateToPage` meng-clone `content`/`style` verbatim ke section baru pada
  suatu page — cocok secara teknis untuk "instantiate footer template", tapi
  **saat ini hanya dipakai untuk 32 whole-page template platform** (di-backfill
  migration `000040`, `000043`), bukan katalog footer per-tenant.
- **Preset "Footer Section" saat ini kosong by design.**
  `frontend/src/features/landing/builder/pages/LandingContentManagementPage.vue` →
  `sectionPresets` → entri `{ key: 'footer-section', type: 'footer', content: {},
  style: {} }`. Konten section footer TIDAK diedit manual — sudah di-derive otomatis
  saat render publik (lihat poin berikut), dan menu Content menampilkan pesan
  penjelasan ("Konten footer otomatis diambil dari Brand & Theme/Navigation/
  Settings") saat section bertipe footer dipilih.
- **Derivasi otomatis footer sudah live** (dikerjakan sesi sebelumnya, sebelum
  dokumen ini dibuat):
  `frontend/src/features/landing/renderer/pages/DynamicLandingPage.vue` menyusun
  objek `footerContent` dari (a) `Branding` hasil `/public/landing/resolve`
  (`company_name`, `logo_light_url`, `tagline`), (b) menu dengan
  `location === 'footer'` (jadi `columns: [{title, links}]`), dan (c)
  `Page.Settings.footer_copyright_text`. Objek ini dioper sebagai prop
  `footer-content` ke `LandingPageRenderer.vue` → diteruskan sebagai
  `content-override` ke `SectionRenderer.vue` khusus untuk section bertipe
  `footer`, menimpa `section.content` yang kosong.
- **Backend `PageSettings` sudah punya field lengkap** untuk kebutuhan footer:
  `footer_copyright_text`, `trust_badges` (`[]TrustBadge{image_url,label}`),
  `secondary_cta_tracking_key`, `newsletter_form_id` (dua terakhir baru ditambah
  sesi sebelumnya — sebelumnya silent-dropped oleh struct `PageSettingsRequest`/
  `PageSettingsResponse` yang tidak mengenal field itu). Lihat
  `internal/modules/landing/domain/landing_page.go` (`PageSettings`),
  `internal/modules/landing/dto/request.go` (`PageSettingsRequest`),
  `internal/modules/landing/dto/response.go` (`PageSettingsResponse`),
  mapper `pageSettingsFromDomain`/`pageSettingsToDomain` di
  `internal/modules/landing/handler/admin_page_handler.go`.
- **Belum dipakai LandingFooter.vue**: `LandingBranding.social_links` dan
  `LandingBranding.contact` sudah ada di data model (dipakai preview di menu
  Brand & Theme) tapi belum pernah dialirkan ke komponen footer manapun.

## Footer Content Model

Breakdown zona konten footer dan sumber data existing yang menjadi acuan seluruh
task di dokumen ini:

| Zona | Field | Bentuk data | Sumber data (menu/endpoint) | Wajib di v1? |
| --- | --- | --- | --- | --- |
| Brand | `brandName` | string | Brand & Theme (`company_name`) | ya |
| Brand | `logoUrl` | string (light/dark) | Brand & Theme (`logo_light_url`/`logo_dark_url`) | ya |
| Brand | `description` | string | Brand & Theme (`tagline`) | ya |
| Navigation | `columns[]` | `{title, links:[{label,href}]}` | Navigation, menu `location=footer` | ya |
| Legal/Bottom bar | `copyright` | string | Settings (`footer_copyright_text`) | ya |
| Trust badges | `trustBadges[]` | `{image_url,label}` | Settings (`trust_badges`) | opsional (variant `mega`) |
| Secondary CTA | `secondaryCta` | `{label,url}` (resolve dari tracking key) | Settings (`secondary_cta_tracking_key`) + CTA module (`landing_ctas`) | opsional (variant `mega`) |
| Newsletter | `newsletterFormId` | string (form id) | Settings (`newsletter_form_id`) + Forms module (`landing_forms`) | opsional (variant `newsletter`) |
| Social (future) | `socialLinks[]` | `{platform,url}` | Brand & Theme (`social_links`) — belum dialirkan ke footer manapun | tidak, backlog |
| Contact (future) | `contact` | `{email,phone,address}` | Brand & Theme (`contact`) — belum dialirkan ke footer manapun | tidak, backlog |

Catatan desain: semua field "wajib" sudah sepenuhnya tersedia lewat menu admin
existing (Brand & Theme, Navigation, Settings) — tidak perlu form input baru untuk
konten, hanya perlu wiring render per-variant dan picker variant.

## Footer Template Catalog v1

| Variant key | Nama tampilan | Zona yang dipakai | Komponen renderer | Status |
| --- | --- | --- | --- | --- |
| `default` | Footer Standard | Brand + Navigation (≤3 kolom) + Legal | `renderer/sections/footer/LandingFooter.vue` (sudah ada) | ada, dipakai semua section footer existing |
| `simple` | Footer Simple | Brand + Legal saja | `renderer/sections/footer/FooterSimple.vue` (baru) | belum dibangun |
| `newsletter` | Footer Newsletter | Standard + Newsletter | `renderer/sections/footer/FooterNewsletter.vue` (baru) | belum dibangun |
| `mega` | Footer Mega | Standard + Trust badges + Secondary CTA | `renderer/sections/footer/FooterMega.vue` (baru) | belum dibangun |

Target use case tiap variant:

- **Standard** — default untuk kebanyakan landing page company profile/produk.
- **Simple** — halaman ringkas (mis. campaign/promo single-page) yang tidak
  butuh banyak link.
- **Newsletter** — halaman lead-generation yang aktif kumpulkan email.
- **Mega** — halaman enterprise/SaaS dengan banyak trust signal (badge, CTA
  sekunder di footer).

## Design Decision

**Katalog footer v1 memakai daftar statis di frontend** (pola sama seperti
`sectionPresets` di `LandingContentManagementPage.vue`), bukan tabel
`landing_section_templates`.

Alasan:

- `landing_section_templates` saat ini semantiknya adalah "design family" milik
  32 whole-page template platform (di-backfill dan distamp lewat migration
  `000040`/`000043`), bukan katalog independen per-tenant. Memakainya sebagai
  footer picker akan mencampur dua konsep berbeda dan berisiko merusak data
  template platform yang sudah ada.
- Kebutuhan v1 (4 layout tetap, dikelola developer) tidak butuh CRUD dinamis;
  daftar statis lebih sederhana, nol migration, dan konsisten dengan pola
  `sectionPresets` yang sudah dipahami tim.
- Opsi "tenant bisa bikin footer template sendiri lewat `landing_section_templates`"
  didokumentasikan sebagai v2/deferred (`FTR-ADV-001`), karena backend-nya sudah
  siap kalau suatu saat dibutuhkan.

## Development Rules

- Perubahan render section HANYA menambah entri baru di
  `section-registry.ts` + komponen baru di `renderer/sections/footer/` — tidak
  mengubah kontrak `SectionRenderer.vue`/`LandingPageRenderer.vue` selain prop
  `content-override` yang sudah ada.
- Semua komponen footer baru WAJIB menerima shape `content` yang sama
  (kontrak `FooterContent`, task `FTR-FE-010`) supaya `DynamicLandingPage.vue`
  tidak perlu tahu variant mana yang aktif saat menyusun data.
- Field baru pada `PageSettings`/DTO tidak boleh menghapus/mengganti field yang
  sudah ada (lihat insiden field silent-dropped sebelum sesi ini).
- Endpoint final wajib disinkronkan ke `api/openapi.yaml` dan
  `docs/landing-page-public-api-contract.md`.

## Phase 0 — Decision

### FTR-0001: Confirm Footer Content Model & Data Ownership

Status: `done`

- Breakdown seluruh zona konten footer dan sumber data existing (lihat
  "Footer Content Model" di atas).
- Konfirmasi: brand → Brand & Theme, link → Navigation (`location=footer`),
  copyright/trust badge/CTA/newsletter → Settings.

Acceptance: tidak ada zona konten footer yang butuh menu/tabel baru untuk v1.

### FTR-0002: Confirm Footer Template Catalog v1 Scope

Status: `done`

- Tetapkan 4 variant v1: `default`, `simple`, `newsletter`, `mega` (lihat
  "Footer Template Catalog v1").
- Tetapkan katalog statis frontend, bukan `landing_section_templates`
  (lihat "Design Decision").

Acceptance: setiap variant punya target use case dan daftar zona konten yang
jelas sebelum implementasi dimulai.

## Phase 1 — Backend

### FTR-BE-001: Validate `variant` Allow-List per Section Type

Status: `done`

- Tambahkan validasi di `internal/modules/landing/service/section_service.go`
  (`validateFooterVariant`, dipanggil dari `Create`/`Update`) supaya `style.variant`
  untuk `section_type='footer'` hanya menerima salah satu dari
  `default|simple|newsletter|mega` (atau kosong).
- Tolak dengan `VALIDATION_ERROR` (422) jika variant tidak dikenal untuk type tersebut.
- **Koreksi temuan riset**: `landing_page_sections` tidak punya kolom `variant`
  tersendiri (lihat FTR-BE-003) — variant footer disimpan sebagai key `variant`
  di dalam `style` (jsonb) yang sudah ada, konsisten dengan fallback yang sudah
  dibaca `SectionRenderer.vue` (`style.variant`/`style.renderer_component`).

Acceptance: request update section footer dengan variant di luar allow-list
ditolak 422; variant kosong/`default` tetap diterima (backward compatible).
Verified: `go test ./internal/modules/landing/service/...` — lihat
`TestSectionServiceCreateRejectsUnknownFooterVariant`,
`TestSectionServiceCreateAllowsKnownFooterVariants`,
`TestSectionServiceCreateIgnoresVariantForNonFooterSection`,
`TestSectionServiceUpdateRejectsUnknownFooterVariant`,
`TestSectionServiceUpdateAllowsKnownFooterVariant` di
`internal/modules/landing/service/section_service_unit_test.go`.

### FTR-BE-002: Audit Public Resolve Payload untuk Footer

Status: `done`

- Audit menemukan bug nyata: `resolver_repository.go` (`ResolveBySlug`/
  `ResolveByDomain`) TIDAK men-select kolom `settings` dari `landing_pages`,
  jadi `ResolvedPage.Page.Settings` selalu zero-value di payload publik
  meskipun `Branding`/`Menus` sudah dikirim tanpa syarat. Diperbaiki dengan
  menambah `settings`/`p.settings` ke query SELECT dan scan target di kedua
  fungsi.
- Sekalian menambah `ResolvedPage.CTAs` (`ResolvedCTA`, hasil
  `ReusableRepository.ListCTAs`) supaya `secondary_cta_tracking_key` (FTR-FE-014)
  bisa di-resolve ke label/url tanpa request admin terpisah — sebelumnya tidak
  ada jalur publik sama sekali untuk membaca `landing_ctas`.
- Test baru: `TestResolverServiceIntegration/ResolveBySlug_-_Published_page_always_carries_Branding/Menus/Settings_for_footer`
  di `internal/modules/landing/service/resolver_service_test.go` (build tag
  `integration`) — meng-assert `Page.Settings.FooterCopyrightText`, `Branding`,
  `Menus` (location=footer), dan `CTAs` semuanya terisi untuk page published
  dengan section footer.

Acceptance: test baru hijau; tidak ada perubahan kontrak field yang sudah dipakai
`DynamicLandingPage.vue` saat ini (field baru `Page.Settings`-yang-benar-terisi
dan `CTAs` bersifat aditif). Verified:
`go test -tags integration ./internal/modules/landing/service/... -run TestResolverServiceIntegration`.

### FTR-BE-003: Document No-New-Migration Decision

Status: `done`

- Tidak ada migration baru. Variant footer disimpan di `style.variant`
  (jsonb yang sudah ada), copyright/trust badge/secondary CTA tracking
  key/newsletter form id di `landing_pages.settings` (jsonb yang sudah ada),
  brand di `landing_brandings`, link di `landing_menus`/`landing_menu_items`.
- Perbaikan FTR-BE-002 (kolom `settings` hilang dari query resolve, CTA publik)
  adalah perbaikan bug pada kode Go yang sudah ada (`resolver_repository.go`,
  `resolver_service.go`), bukan perubahan schema/migration.

Acceptance: reviewer paham tidak ada migration yang hilang/terlewat.

## Phase 2 — Renderer (Plug-and-Play Mechanism)

### FTR-FE-010: Define Shared `FooterContent` Contract

Status: `done`

- Ditambahkan `FooterContent` di
  `frontend/src/features/landing/shared/types/landing.types.ts` mencakup semua
  field di "Footer Content Model" (brand, columns, copyright, trustBadges,
  secondaryCta, newsletterFormId, socialLinks, contact).
- `LandingFooter.vue` dan 3 komponen footer baru memakai type ini untuk prop
  `content`.

Acceptance: type-check bersih — `npx vue-tsc --noEmit` (exit 0).

### FTR-FE-011: Extract `useFooterContent()` Composable

Status: `done`

Dependency: `FTR-FE-010`

- Logic penyusunan `footerContent` dipindah dari `DynamicLandingPage.vue` ke
  `frontend/src/features/landing/renderer/composables/useFooterContent.ts`
  (`useFooterContent(rawResolvePayload, fallbackTitle): FooterContent`).
- Composable ini sekaligus menutup gap yang belum ada sebelumnya: resolusi
  `trustBadges` (langsung dari `Settings`), `secondaryCta` (match
  `secondary_cta_tracking_key` terhadap `CTAs` baru dari FTR-BE-002), dan
  `newsletterFormId`.
- `DynamicLandingPage.vue` memanggil composable ini, tidak mengubah perilaku
  brand/columns/copyright yang sudah ada.

Acceptance: unit test `useFooterContent.spec.ts` (7 test) hijau — mencakup
fallback kosong, resolusi trust badge, resolusi secondary CTA
match/no-match, newsletter form id, dan kompatibilitas key PascalCase (Go JSON).

### FTR-FE-012: Build `FooterSimple.vue`

Status: `done`

Dependency: `FTR-FE-010`

- `renderer/sections/footer/FooterSimple.vue`: brand block + copyright saja,
  tanpa kolom navigasi.

Acceptance: `FooterSimple.spec.ts` (4 test) — render dengan `FooterContent`
minimal, fallback ke `BrandLogo` saat `logoUrl` kosong.

### FTR-FE-013: Build `FooterNewsletter.vue`

Status: `done`

Dependency: `FTR-FE-010`, `FTR-FE-011`

- `renderer/sections/footer/FooterNewsletter.vue`: layout Standard + form
  newsletter yang submit ke `POST /public/landing/forms/:formKey/submissions`
  (endpoint publik `PublicLandingHandler.SubmitForm` yang sudah ada — **catatan**:
  tidak ada komponen renderer "form" generik yang bisa direuse di frontend
  saat riset dilakukan, jadi submit call dibuat langsung di komponen ini,
  bukan reuse komponen lain).

Acceptance: `FooterNewsletter.spec.ts` (4 test) — blok newsletter disembunyikan
saat `newsletterFormId` kosong, submit mengirim `{fields:{email}, consent}` ke
endpoint yang benar dan menampilkan pesan sukses.

### FTR-FE-014: Build `FooterMega.vue`

Status: `done`

Dependency: `FTR-FE-010`

- `renderer/sections/footer/FooterMega.vue`: layout Standard + trust badges
  row + secondary CTA button.

Acceptance: `FooterMega.spec.ts` (6 test) — trust badges/secondary CTA
disembunyikan saat kosong, muncul dengan benar saat terisi.

### FTR-FE-015: Register All Footer Variants in Section Registry

Status: `done`

Dependency: `FTR-FE-012`, `FTR-FE-013`, `FTR-FE-014`

- `renderer/registry/section-registry.ts` →
  `footer: { default: LandingFooter, LandingFooter, simple: FooterSimple,
  newsletter: FooterNewsletter, mega: FooterMega }`.

Acceptance: section footer dengan `variant` apa pun dari 4 key di atas resolve
ke komponen yang benar (fallback `default` tetap berlaku via
`resolveSection()`, tidak diubah).

## Phase 3 — Admin UI

### FTR-FE-020: Footer Variant Picker di Menu Content

Status: `done`

Dependency: `FTR-FE-015`

- Saat section aktif di `LandingContentManagementPage.vue` bertipe `footer`,
  tampilkan pilihan 4 variant sebagai card (bukan dropdown polos) di bawah
  pesan penjelasan statis yang sudah ada.
- Simpan pilihan lewat `landingApi.updateSection(pageId, sectionId, { style:
  { ...section.style, variant } })` — **koreksi dari rencana awal**: variant
  disimpan sebagai key `variant` di dalam `style` (jsonb yang sudah ada), bukan
  field `variant` top-level terpisah (lihat Decision Log di
  footer-management-traceability-index.md, 2026-07-16) — konsisten dengan
  validasi backend `validateFooterVariant` (FTR-BE-001).

Acceptance: pilihan tersimpan dan bertahan setelah reload halaman admin
(`activeFooterVariant` computed membaca `style.variant`); card aktif
ter-highlight sesuai variant section saat ini. Verified: `npx vue-tsc --noEmit`
bersih, manual check di dev server (lihat Development History).

### FTR-FE-021: Variant Preview/Label

Status: `done`

Dependency: `FTR-FE-020`

- Deskripsi zona per variant ditulis langsung di `footerVariantOptions`
  (`LandingContentManagementPage.vue`), mis. "Brand + navigasi (maks. 3
  kolom) + copyright" untuk Standard.

Acceptance: user bisa membedakan 4 variant dari nama + deskripsi tanpa
klik-coba satu-satu (tidak ada thumbnail visual — dianggap cukup untuk v1,
bisa ditingkatkan nanti bila dibutuhkan).

### FTR-FE-022: Contextual Settings Fields per Variant (Nice to Have)

Status: `planned`

Dependency: `FTR-FE-020`

- Di menu Settings, sembunyikan/beri highlight field yang relevan dengan
  variant footer terpilih (mis. field newsletter hanya ditonjolkan kalau
  variant `newsletter` aktif) — butuh Settings tahu variant section footer
  page aktif (`landingApi.getSections(pageId)` filter type=footer).

Acceptance: tidak ada regresi ke field yang sudah ada; murni penyesuaian
visual/urutan, semua field tetap bisa diisi kapan pun.

## Phase 4 — Test & Rollout

### FTR-TEST-001: Component Test per Variant

Status: `done`

- `FooterSimple.spec.ts`, `FooterNewsletter.spec.ts`, `FooterMega.spec.ts`,
  `useFooterContent.spec.ts` (23 test total) — logo kosong (fallback),
  columns kosong, trustBadges/secondaryCta/newsletter kosong (blok
  disembunyikan, bukan error).

Acceptance: seluruh test hijau (`npx vitest run`), tidak ada console error saat
props minimal.

### FTR-TEST-002: Manual Verification Checklist

Status: `planned`

- Checklist verifikasi di subdomain tenant nyata: ganti variant lewat admin,
  refresh halaman publik, cek tiap zona konten tampil sesuai data
  Brand&Theme/Navigation/Settings yang diisi.

Acceptance: checklist dijalankan minimal sekali per variant sebelum rilis.

### FTR-DOC-001: Update API Contract & Reference Docs

Status: `planned`

- Update `docs/landing-page-public-api-contract.md` dan `api/openapi.yaml`
  untuk field `variant` allow-list pada section footer serta field
  `secondary_cta_tracking_key`/`newsletter_form_id` di `PageSettings`.
- Update `docs/reference-landing-page.md` bagian navigasi/footer.

Acceptance: dokumentasi publik konsisten dengan implementasi.

### FTR-ROLLOUT-001: Backward Compatibility Note

Status: `planned`

- Konfirmasi section footer existing (`variant=null`/`''`) otomatis resolve ke
  `default` tanpa perlu backfill data.

Acceptance: tidak ada script migrasi data yang diperlukan untuk rilis fitur ini.

## Deferred Roadmap

### FTR-ADV-001: Tenant-Authored Footer Template (via `landing_section_templates`)

Status: `deferred`

- Bangun UI CRUD untuk `landing_section_templates` (`section_type='footer'`)
  supaya tenant bisa menyimpan kombinasi konten footer kustom sendiri sebagai
  template yang bisa dipakai ulang lintas page, memakai endpoint
  `InstantiateToPage` yang sudah ada di backend.

### FTR-ADV-002: Social & Contact Zone di Footer

Status: `deferred`

- Alirkan `LandingBranding.social_links` dan `.contact` ke komponen footer
  (khususnya variant `mega`) — data sudah ada, belum pernah dipakai footer.
