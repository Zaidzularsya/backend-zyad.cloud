# Landing Page Development Tasks

Dokumen ini adalah breakdown development untuk:

```txt
internal/modules/landing
```

Sumber:

- `docs/reference-landing-page.md`
- `docs/reference-multi-tenant.md`
- `docs/multi-tenant-development-tasks.md`
- `README.md`
- `docs/migration-guide.md`
- `docs/landing-page-public-api-contract.md`

## Tujuan

Membangun Landing Page multi-tenant dalam satu module, termasuk page builder, section/template, CTA, form, submission, lead integration, media, branding/theme, navigation, SEO, publish/revision/schedule, domain, dan analytics.

Semua task utama berstatus `planned` sampai implementasi dan verifikasi selesai. Task pada bagian Deferred Roadmap berstatus `deferred`.

## Development Rules

- Handler hanya melakukan binding, auth/tenant context, validation, dan response.
- Service memegang business rule, transaction, publish lifecycle, idempotency, dan integration.
- Repository hanya mengakses database.
- Storage memakai `internal/platform/storage`.
- Notification memakai `internal/core/notification`.
- Jangan membuat module `media`, `forms`, `crm`, atau `analytics`.
- Capability media metadata, CTA, navigation, lead delivery, dan analytics tetap berada dalam `internal/modules/landing`.
- Database berubah hanya melalui migration baru di schema `public`.
- Endpoint final wajib disinkronkan ke `api/openapi.yaml`.

## Phase 0 - Foundation

### LAND-0001: Resolve Module Naming

Status: `done`

- Tetapkan `internal/modules/landing` sebagai canonical.
- Pindahkan atau hapus placeholder `internal/modules/landingpage`.
- Pastikan tidak ada dua route registration aktif.
- Update README architecture setelah code path berubah.

Acceptance: hanya satu package Landing aktif dan semua import memakai path canonical.

### LAND-0002: Module Skeleton

Status: `done`

- Buat `domain`, `dto`, `repository`, `service`, `handler`, `routes`, dan `module.go`.
- Buat dependency interfaces dan wire ke `internal/app`.

Acceptance: app compile, module dapat diregistrasikan, dan tidak ada dependency produksi baru.

### LAND-0003: Tenant and Permission Contract

Status: `in_progress`

Dependency:

- `MT-CORE-001..005`
- `MT-DATA-001..004`
- `MT-PLAT-001..003`

- Definisikan organization context.
- Definisikan dan seed permission Landing secara idempotent.
- Tetapkan super admin behavior dan audit requirement.

Progress:

- Permission catalog Landing sudah tersedia dan diuji.
- Typed organization context, authenticated/public/worker resolution, dan fail-closed tenant guards selesai melalui `MT-CORE-001..005`.
- Repository tenant scope contract selesai melalui `MT-DATA-002`; implementasi repository Landing wajib memakai `tenant.Scope`.
- Permission seed, RLS, dan platform organization masih mengikuti task multi-tenant berikutnya.

Acceptance: admin route memerlukan auth/permission dan repository tidak berjalan tanpa tenant scope kecuali explicit super admin method.

## Phase 1 - Database Foundation

### LAND-DB-001: Audit Existing Landing Schema

- Periksa `landing_pages` dan `brands` pada database aktual.
- Bandingkan dengan baseline arsip.
- Dokumentasikan mapping, conflict, dan backfill.

Acceptance: cleanup/compatibility memakai migration terpisah dan tidak mengasumsikan baseline sudah diterapkan.

### LAND-DB-002: Landing Page Core Tables

- Migration `landing_pages` dan `landing_page_sections`.
- Index tenant, slug, status, order, dan soft delete.

Acceptance: up/down tersedia, slug unik per organization, dan section order konsisten.

### LAND-DB-003: Form and Submission Tables

- Migration `landing_forms`, `landing_form_fields`, `landing_submissions`, dan `landing_submission_notes`.
- Index form, page, status, dan created time.

Acceptance: submission tetap dapat dibaca setelah schema form berubah dan snapshot label/schema yang diperlukan tersimpan.

### LAND-DB-004: Publish Version and Redirect Tables

- Migration `landing_page_versions` dan `landing_slug_redirects`.
- Simpan immutable published snapshot.

Acceptance: version unik per page dan redirect tidak loop.

### LAND-DB-005: Domain and Branding Tables

- Migration `landing_domain_bindings` dan `landing_brandings`.
- Gunakan `organization_domains` dari multi-tenant sebagai source of truth host/verification.
- Organization default dan page override.

Acceptance: branding resolve deterministik, binding hanya menerima verified domain dalam organization yang sama, dan tidak ada registry domain kedua.

### LAND-DB-006: Analytics Tables

- Migration `landing_analytics_events` dan `landing_analytics_daily`.
- Index aggregation dan retention.

Acceptance: event insert ringan dan daily aggregate idempotent.

### LAND-DB-007: Reusable Content and Navigation Tables

- Migration `landing_section_templates`, `landing_ctas`, `landing_menus`, dan `landing_menu_items`.
- Tenant ownership, soft delete, sort order, dan nested menu constraint.

Acceptance: reusable content tidak lintas tenant, menu cycle ditolak, dan published snapshot tidak bergantung pada mutable template.

### LAND-DB-008: Media Metadata Tables

- Migration `landing_media_assets` dan optional folder/tag metadata.
- Simpan storage object reference, MIME, size, dimensions/duration, alt text, dan processing status.

Acceptance: binary tidak disimpan di database dan asset yang masih dipakai tidak dapat dihapus permanen.

### LAND-DB-009: Revision and Schedule Tables

- Migration `landing_page_revisions`.
- Tambahkan publish/unpublish schedule state dan processing lock/idempotency metadata.

Acceptance: revision number unik, snapshot immutable, dan due schedule dapat diproses ulang dengan aman.

### LAND-DB-010: Lead Integration Tables

- Migration `landing_lead_integrations` dan `landing_lead_delivery_logs`.
- Simpan integration type, encrypted credential reference, event filter, retry, dan delivery status.

Acceptance: secret tidak plain text, delivery idempotent, dan retry query memiliki index.

## Phase 2 - Domain and DTO

### LAND-BE-001: Domain Models

Status: `in_progress`

- Page, section/template, CTA, form, field, submission, note, version/revision, media, menu, domain, branding/theme, lead integration, dan analytics.
- Status/value object validation.

Progress:

- Model dan enum awal page, section, form, field, dan submission sudah dibuat.
- Branding, domain, revision, media, menu, integration, dan analytics masih planned.

Acceptance: model selaras migration dan tidak bergantung pada Gin/SQL driver.

### LAND-BE-002: Admin DTO

- Page/access, section/template, CTA, form, submission, media, branding/theme, menu, domain, revision/schedule, lead integration, publish, analytics.
- Pagination/filter/sort.

Acceptance: request/response terpisah dan internal path/raw IP/secret tidak diekspos.

### LAND-BE-003: Public DTO

- Published page, access challenge, navigation, media URL, resolved branding, public form schema, submission, dan analytics event.

Acceptance: hanya published snapshot dan enabled section/form yang dikembalikan.

## Phase 3 - Repository

### LAND-REPO-001: Landing Page Repository

- CRUD/list/detail/soft delete/restore.
- Resolve organization/slug dan published host.
- Row lock untuk publish.

Acceptance: admin query tenant-scoped dan public query hanya published data.

### LAND-REPO-002: Section Repository

- CRUD, duplicate, dan bulk reorder atomik.

Acceptance: tidak ada duplicate order dan ownership page tervalidasi.

### LAND-REPO-003: Form Repository

- CRUD form/fields, reorder, dan active public lookup.

Acceptance: schema konsisten dan public lookup menolak unpublished page.

### LAND-REPO-004: Submission Repository

- Create, list/detail/status/note/delete.
- Idempotency lookup dan export cursor.

Acceptance: create transaction aman dan filter status/date/page/form tersedia.

### LAND-REPO-005: Version, Domain, and Branding Repository

- Version, slug redirect, verified organization domain binding, branding default/override.

Acceptance: published version immutable dan domain binding tidak dapat lintas organization.

### LAND-REPO-006: Analytics Repository

- Insert event, aggregate daily, summary, dan time series.

Acceptance: aggregate idempotent dan dashboard tenant-scoped.

### LAND-REPO-007: Reusable Content and Navigation Repository

- CRUD section template, CTA, menu, dan menu item.
- Resolve referenced resources saat membuat published snapshot.

Acceptance: nested menu tidak cycle, ordering atomik, dan tenant ownership selalu diperiksa.

### LAND-REPO-008: Media Repository

- Create/list/detail/update metadata.
- Usage lookup dan soft delete.
- Processing status update.

Acceptance: asset list tenant-scoped dan delete menolak asset aktif tanpa explicit force policy.

### LAND-REPO-009: Revision and Schedule Repository

- Create/list/get/compare source revision.
- Claim due publish/unpublish schedule.
- Mark schedule processed/failed.

Acceptance: claim aman untuk concurrent worker dan revision immutable.

### LAND-REPO-010: Lead Integration Repository

- CRUD integration config.
- Create/claim/update delivery log.
- Retry due delivery.

Acceptance: credential hanya melalui encrypted reference dan claim delivery concurrency-safe.

## Phase 4 - Page Builder Service

### LAND-BE-010: Landing Page CRUD Service

- Create/list/detail/update/duplicate/delete/restore/archive.
- Slug conflict dan audit metadata.

Acceptance: duplicate menjadi draft/slug baru dan draft edit tidak mengubah public snapshot.

### LAND-BE-011: Section Service

- Section registry dan content/style validation.
- CRUD, duplicate, reorder, toggle.
- Sanitize rich text/URL.

Acceptance: invalid schema menghasilkan field errors dan reorder atomik.

### LAND-BE-012: SEO Service

- Validate meta title/description/keywords, canonical URL, robots, social metadata, dan JSON-LD.
- Generate fallback metadata.

Acceptance: invalid URL/schema ditolak dan effective SEO deterministik.

### LAND-BE-013: Page Visibility and Access Service

- Visibility `public`, `private`, dan `password_protected`.
- Hash/replace/remove page password.
- Validate public access challenge dan issue short-lived access grant.
- Rate limit password attempts.

Acceptance: password plain tidak disimpan/dikembalikan, private page tidak dapat diakses public, dan access grant scoped ke page/version.

### LAND-BE-014: CTA Service

- Reusable CTA CRUD dan inline CTA validation.
- Type contact form, WhatsApp, internal/external link, dan document download.
- Target dan stable tracking key.

Acceptance: referenced form/page/asset satu tenant, URL aman, dan click dapat diatribusikan.

### LAND-BE-015: Reusable Section Template Service

- Save section as template, list/detail/update/delete, dan instantiate ke page.
- Copy-on-use untuk MVP.

Acceptance: perubahan template tidak mengubah existing page dan schema divalidasi sesuai section type.

### LAND-BE-016: Navigation Service

- Header/footer/sidebar menu CRUD.
- Nested item, reorder, toggle, internal/external link, anchor, dan button.

Acceptance: cycle/depth invalid ditolak dan broken internal reference masuk publish validation.

## Phase 5 - Branding

### LAND-BE-020: Branding Service

- Get/update organization default.
- Get/update/remove page override.
- Merge effective branding.
- Validate color, URL, font token, logo, dan social link.

Acceptance: tidak menerima script, override bersifat partial, dan update diaudit.

### LAND-BE-021: Branding Asset Integration

- Integrasi logo/favicon/social image dengan `internal/platform/storage`.
- Validate MIME, size, ownership, dan alt text.

Acceptance: public URL sesuai dan internal path tidak diekspos.

### LAND-BE-022: Theme and Layout Service

- Spacing scale, background style, layout width, header/footer style.
- Mode light/dark/system dan effective token resolution.
- Custom CSS tetap disabled pada MVP; jika diaktifkan wajib permission/sanitization terpisah.

Acceptance: token tervalidasi, contrast warning tersedia, dan arbitrary script tidak dapat diinjeksi.

### LAND-BE-023: Landing Media Service

- Upload/list/update/reuse/delete image, video, dan PDF/document.
- Folder/tag metadata, alt text, caption, focal point.
- Image optimization/thumbnail interface dan processing status.

Acceptance: MIME sniffing, size limit, ownership, random object key, usage guard, dan internal path protection diterapkan.

## Phase 6 - Form and Submission

### LAND-BE-030: Form Builder Service

- Form/field CRUD dan reorder.
- Stored-schema validation, consent, dan success behavior.

Acceptance: duplicate field key ditolak, redirect URL aman, dan file field memiliki MIME/size/count policy.

### LAND-BE-031: Public Submission Service

- Resolve published form dan validate payload.
- Normalize email/phone.
- Simpan file upload melalui platform storage dan kaitkan ke submission snapshot.
- Honeypot/rate limit/CAPTCHA hook.
- Idempotency, UTM/referrer/user-agent/IP hash.
- Publish `landing.submission_created`.

Acceptance: invalid payload tidak disimpan, notification failure tidak menghilangkan submission, dan response tidak membocorkan data internal.

### LAND-BE-032: Submission Admin Service

- List/detail/status/note/delete/export.

Acceptance: CSV mencegah formula injection dan export besar memakai streaming/cursor.

### LAND-BE-033: Lead Notification and Delivery Service

- Kirim email/WhatsApp/optional Discord melalui notification outbox.
- Generic signed webhook untuk N8N/automation.
- Optional adapter contract untuk CRM lead, Telegram, Google Sheets, dan sales assignment.
- Retry, timeout, idempotency, delivery log, dan dead-letter state.

Acceptance: submission commit lebih dulu, delivery failure tidak rollback submission, webhook mencegah SSRF, dan secret tidak masuk log.

## Phase 7 - Publish and Public Delivery

### LAND-BE-040: Publish Validation

- Validasi page, branding, SEO, section, form, dan domain.
- Return structured checklist.

Acceptance: invalid page tidak publish dan validation tidak mengubah state.

### LAND-BE-041: Publish Service

- Publish immutable snapshot, unpublish, restore version to draft, cache invalidation, event/audit.

Acceptance: transaction/row lock dipakai dan public reader tidak melihat partial publish.

### LAND-BE-042: Preview

- Authenticated preview dan optional expiring hashed preview token.

Acceptance: preview noindex; token scoped, expiring, revocable.

### LAND-BE-043: Public Page Resolver

- Resolve custom domain, platform subdomain, atau fallback slug.
- Canonical/old slug redirect.
- Return published snapshot, branding, SEO, sections, forms.

Acceptance: draft tidak bocor dan disabled resource ditangani konsisten.

### LAND-BE-044: Draft Autosave and Revision Service

- Autosave draft dengan optimistic concurrency.
- Buat revision snapshot berdasarkan debounce/meaningful change.
- List revision, compare dua revision, dan restore revision menjadi draft.
- Audit actor dan change note.

Acceptance: stale editor menerima conflict, autosave tidak membuat revision setiap keystroke, dan restore tidak langsung publish.

### LAND-BE-045: Scheduled Publishing Service

- Schedule/cancel publish dan unpublish berdasarkan timezone organization.
- Worker claim due schedule dan memanggil publish service idempotently.
- Catat success/failure dan publish event.

Acceptance: concurrent worker tidak double publish, invalid draft gagal dengan reason, dan schedule dapat di-retry.

## Phase 8 - Domain

### LAND-BE-050: Domain Management

- List verified organization domains yang tersedia.
- Bind/unbind domain ke Landing Page.
- Set primary/canonical page binding.
- Delegasikan add/verify/SSL domain ke organization domain service.

Acceptance: hanya verified domain organization aktif yang dapat di-bind dan duplicate active binding ditolak.

### LAND-BE-051: SSL Provisioning Integration

- Consume SSL/domain provisioning status dari multi-tenant organization domain capability.
- Invalidate public Landing cache saat status host berubah.

Acceptance: Landing tidak mengimplementasikan provider SSL kedua dan local development berjalan tanpa provider eksternal.

## Phase 9 - Analytics

### LAND-BE-060: Public Analytics Event

- Track allowlisted event.
- Validate page/section/form key.
- Bot/rate-limit hooks dan privacy-preserving visitor key.

Acceptance: arbitrary event/property dan PII ditolak.

### LAND-BE-061: Analytics Aggregate

- Daily aggregate, summary, time series, traffic source, device/browser, top-performing page, lead attribution, dan conversion funnel.

Acceptance: re-run tidak menggandakan count dan timezone organization dipakai.

## Phase 10 - HTTP API

### LAND-API-001: Admin Page and Section API

- Page, section, SEO, preview, publish, version endpoints.
- Permission middleware dan OpenAPI.

### LAND-API-002: Admin Form and Submission API

- Form/field/submission/note/export endpoints.
- Pagination, filter, dan permission terpisah.

### LAND-API-003: Branding, Domain, and Analytics API

- Branding, theme, domain, analytics endpoints.
- Audit mutation dan response stabil untuk Vue.

### LAND-API-004: Public API

- Public resolve/access challenge, submit/file upload, analytics event.
- Temporary scoped upload untuk field form bertipe file.
- Cache headers, ETag/version, rate limit.

Acceptance: tidak memerlukan admin auth dan tidak menerima tenant ID bebas.

### LAND-API-005: CTA, Template, Media, and Navigation API

- Reusable CTA dan section template endpoints.
- Landing media upload/list/update/delete endpoints.
- Menu dan menu item CRUD/reorder endpoints.

Acceptance: contract Vue memiliki schema stabil dan permission granular.

### LAND-API-006: Revision, Schedule, and Lead Integration API

- Revision list/detail/compare/restore.
- Schedule/cancel publish/unpublish.
- Lead integration CRUD, test delivery, delivery log, dan retry.

Acceptance: secret di-mask, mutation diaudit, dan endpoint test tidak dapat dipakai untuk SSRF.

## Phase 11 - Seed

### LAND-SEED-001: Landing Permissions

- Seed permission catalog dan assign ke `super_admin`.

Acceptance: idempotent dan rollback aman.

### LAND-SEED-002: Default Presets

- Seed Hero, About, Feature, CTA, Contact Form, section template, dan branding preset yang benar-benar diperlukan.
- Hindari demo tenant pada production migration.

### LAND-SEED-003: Default Role Permission Matrix

- Dokumentasikan dan bila sesuai seed assignment untuk Marketing Manager, Content Editor, Sales, dan Viewer.
- Jangan membuat duplicate system role jika role ekuivalen sudah tersedia.

Acceptance: Content Editor tidak dapat publish, Sales hanya mengakses submission yang diperlukan, dan Viewer read-only.

## Phase 12 - Testing and Documentation

### LAND-TEST-001: Unit Tests

- Slug/domain, tenant guard, section/form validation, branding merge.
- Visibility/password, CTA/reference, menu cycle, media validation.
- Publish/revision/schedule, submission idempotency, lead delivery, analytics validation.

### LAND-TEST-002: Repository Integration Tests

- CRUD/tenant isolation, publish transaction, unique slug/domain.
- Reusable content, navigation, media usage, revision/schedule claim.
- Submission/lead delivery dan analytics aggregate.

### LAND-TEST-003: Handler and Public Flow Tests

- Permission, validation, resolve/access, submit/upload, preview, schedule, integration secret masking, error envelope.

### LAND-DOC-001: Documentation Sync

- Update traceability, API contract, OpenAPI, migration guide, dan README.

## Definition of Done

- Code mengikuti module boundary.
- Migration up/down diverifikasi.
- Unit dan relevant integration test lulus.
- Tenant isolation, permission, audit, dan public abuse controls diuji.
- OpenAPI dan Vue contract sinkron.
- Traceability dan development history diperbarui.

## Deferred Roadmap Tasks

Task berikut dicatat agar requirement source tidak hilang, tetapi status awalnya `deferred` dan tidak masuk MVP:

### LAND-ADV-001: Visual Page Builder

- Drag-and-drop visual canvas, responsive preview, undo/redo, dan component inspector.

### LAND-ADV-002: Template Marketplace

- Template catalog, import/export, version compatibility, dan tenant-safe install.

### LAND-ADV-003: Experiment and Personalization

- A/B testing, audience rule, experiment allocation, dan conversion attribution.

### LAND-ADV-004: Multi-Language Page

- Locale variants, translation workflow, hreflang, dan locale fallback.

### LAND-ADV-005: AI Content Assistance

- AI copywriting dan page generation dengan approval, audit, quota, dan safe output.

### LAND-ADV-006: Advanced Delivery

- Static page generation, edge cache/CDN, heatmap integration, dan data warehouse export.

### LAND-ADV-007: Custom CSS

- Restricted custom CSS editor, sanitization, CSP compatibility, preview, dan permission khusus.
