# Landing Page Module Reference

Dokumen ini menyimpan source requirement, batas tanggung jawab, dan kapasitas target module Landing Page.

## Source Requirement

- ChatGPT share: `https://chatgpt.com/share/6a2d5a38-20f8-83ec-a846-9af55c5aa381`
- Salinan lengkap percakapan yang diberikan user pada 2026-06-13.
- Judul share: `Breakdown Feature Management`.
- Requirement eksplisit dari user pada 2026-06-13.
- `README.md`, khususnya Landing Page Generator, multi-tenancy, custom domain, SEO, dan lead capture.
- Baseline schema arsip di `docs/archive/migrations/monolithic_baseline/000001_public_schema_baseline.up.sql`.

## Source Access Note

Percakapan lengkap awalnya tidak dapat diekstrak dari halaman share karena endpoint anonim mengembalikan HTTP `403`. Pada 2026-06-13 user memberikan salinan lengkap percakapan, sehingga audit requirement sekarang memakai salinan tersebut sebagai source utama bersama instruksi eksplisit user.

## Objective

Membangun satu module Landing Page multi-tenant yang menangani:

- Landing page dan public marketing page.
- Page builder berbasis section.
- Form builder dan public submission.
- Lead capture tanpa memecah business capability ke module CRM.
- Branding tenant dan branding per page.
- Theme, navigation, reusable section/template, dan CTA.
- SEO dan social sharing metadata.
- Draft, autosave, preview, publish, unpublish, scheduled publish, archive, dan version/revision snapshot.
- Public, private, dan password-protected page.
- Slug, subdomain, dan custom domain mapping.
- Analytics dasar landing page.
- Media image/video/document melalui platform storage tanpa module media terpisah.
- Lead delivery ke notification, webhook, automation, dan optional CRM integration.
- Event notification melalui core notification.
- API admin untuk Vue dan API public untuk renderer/storefront.

## Architecture Decision

Landing Page dipertahankan sebagai satu bounded module:

```txt
internal/modules/landing
```

Struktur target:

```txt
internal/modules/landing/
├── domain/
│   ├── landing_page.go
│   ├── landing_section.go
│   ├── landing_form.go
│   └── landing_submission.go
├── dto/
│   ├── request.go
│   └── response.go
├── repository/
│   ├── landing_page_repository.go
│   ├── section_repository.go
│   └── form_repository.go
├── service/
│   ├── landing_page_service.go
│   ├── publish_service.go
│   ├── form_service.go
│   └── analytics_service.go
├── handler/
│   ├── admin_handler.go
│   └── public_handler.go
├── routes/
│   └── routes.go
└── module.go
```

File tambahan boleh dibuat di dalam struktur tersebut jika satu file menjadi terlalu besar, tetapi capability tidak dipindah menjadi module bisnis terpisah.

## Rejected Module Split

Jangan memecah Landing Page menjadi:

```txt
internal/modules/landing
internal/modules/media
internal/modules/forms
internal/modules/crm
internal/modules/notification
internal/modules/analytics
```

Keputusan boundary:

| Capability | Ownership |
| --- | --- |
| Page, section, form, submission, lead state, analytics | `internal/modules/landing` |
| CTA, theme, navigation, reusable template, media metadata | `internal/modules/landing` |
| Binary file persistence dan signed URL | `internal/platform/storage` |
| Notification template, outbox, dan delivery | `internal/core/notification` |
| Auth, permission, tenant context, validation, response | `internal/core` |
| Cross-module customer/CRM sync di masa depan | Integrasi service/event, bukan pemindahan ownership landing submission |

## Naming Conflict

Kondisi awal repo:

- README dan placeholder code memakai `internal/modules/landingpage`.
- Requirement user menetapkan `internal/modules/landing`.

Keputusan dan status:

- Target canonical adalah `internal/modules/landing`.
- Placeholder `internal/modules/landingpage` sudah dipindahkan pada 2026-06-13.
- Jangan mempertahankan dua package aktif karena akan membingungkan route registration dan ownership.

## Tenant Boundary

- Semua resource admin memiliki `organization_id`.
- Query admin selalu difilter oleh organization dari auth/tenant context.
- Client tidak boleh menentukan organization milik tenant lain.
- Slug unik minimal dalam scope organization.
- Custom domain unik global setelah canonicalization.
- Public resolution memakai host/domain atau public slug, bukan `X-Tenant-ID` bebas dari browser.
- Cross-tenant access membutuhkan permission khusus dan audit log.

## Capability Catalog

### Landing Page Management

- List, detail, create, update, duplicate, soft delete, restore, dan archive.
- Status: `draft`, `published`, `unpublished`, `archived`.
- Page type extensible: `homepage`, `company_profile`, `product_service`, `campaign`, `pricing`, `contact`, `lead_capture`, `promo_event`, dan `portfolio_case_study`.
- Visibility: `public`, `private`, dan `password_protected`.
- Password page disimpan sebagai hash, tidak pernah plain text.
- Internal name terpisah dari public title.
- Slug, locale, timezone, `publish_at`, dan `unpublish_at`.
- Homepage marker per domain/site scope.
- Page settings tervalidasi, bukan satu JSON bebas.

### Section Builder

- Section ordered per page.
- Section type registry: `hero`, `about`, `features`, `services`, `product_showcase`, `content`, `gallery`, `portfolio`, `testimonial`, `pricing`, `faq`, `cta`, `form`, `contact`, `newsletter`, `partner_logos`, `statistics`, dan `footer`.
- `custom_html` deferred dan dibatasi.
- Create, update, reorder, duplicate, enable/disable, dan delete section.
- Content/style JSONB divalidasi berdasarkan section type.
- Stable section key untuk anchor dan analytics.
- Sanitization untuk rich text dan embed.
- Section dapat disimpan sebagai reusable section atau section template dalam tenant yang sama.
- Reusable section memakai copy-on-use pada MVP agar perubahan template tidak mengubah page published secara tak terduga.

### Hero and CTA

- Hero mendukung headline, subheadline, badge, background image/video, variant, primary CTA, dan secondary CTA.
- CTA dapat disimpan inline pada section atau sebagai reusable CTA tenant.
- CTA type: `contact_form`, `whatsapp`, `external_link`, `internal_page`, dan `document_download`.
- CTA target: `same_tab` atau `new_tab`.
- CTA memiliki stable tracking key.
- Internal page dan form reference divalidasi dalam tenant yang sama.
- External URL, WhatsApp URL, dan download asset divalidasi sebelum publish.

### Form Builder

- Satu page dapat memiliki beberapa form.
- Field minimum: `text`, `textarea`, `email`, `phone`, `number`, `select`, `radio`, `checkbox`, `file`, `date`, dan `hidden`.
- Required, label, placeholder, options, validation rules, dan sort order.
- Success message dan optional redirect URL.
- Consent checkbox dan privacy policy reference.
- Form active/inactive.
- Server-side validation dibangun dari stored schema.

### Submission and Lead Capture

- Public submit tanpa authentication.
- Simpan normalized contact fields dan payload yang sudah difilter.
- Status: `new`, `contacted`, `qualified`, `converted`, `rejected`, `spam`, `archived`.
- Admin list, detail, status update, note, export, dan soft delete.
- Source metadata: page, form, referrer, UTM, IP hash, user agent, submitted time.
- Idempotency atau duplicate suppression untuk browser retry.
- Notification event setelah submission tersimpan.
- CAPTCHA/rate limit/honeypot.
- Submission tetap dimiliki Landing walaupun nanti disinkronkan ke CRM.
- File submission disimpan melalui platform storage dengan MIME/size allowlist dan malware scanning hook.

### Lead Delivery Integration

- Submission disimpan terlebih dahulu sebelum integrasi eksternal dijalankan.
- Notification minimum: email, WhatsApp, dan optional Discord melalui core notification.
- Outbound integration: generic webhook sebagai foundation untuk N8N dan automation lain.
- Optional adapters: CRM lead, Telegram, Google Sheets, dan sales assignment.
- Delivery memakai outbox/retry, idempotency key, timeout, status log, dan dead-letter state.
- Secret webhook/provider tidak disimpan plain text dalam landing content.
- Kegagalan integration tidak menghapus atau membatalkan submission.
- Follow-up detail tetap dicatat sebagai submission status/note sampai module CRM eksternal benar-benar tersedia.

### Branding

Branding adalah feature wajib tambahan.

- Organization branding default dan optional override per page.
- Logo utama, dark/light, favicon, dan social image.
- Primary, secondary, accent, background, surface, text, dan muted colors.
- Heading/body font melalui allowlist atau token.
- Button radius, card radius, dan style token terbatas.
- Spacing scale, background style, layout width, header style, dan footer style.
- Mode `light`, `dark`, atau `system` dengan token warna yang tervalidasi.
- Company name, tagline, contact email, phone, address, dan social links.
- Custom CSS deferred dan memerlukan sanitization serta permission khusus.
- Public response mengembalikan resolved branding hasil merge default dan override.

Table `brands` pada baseline arsip wajib diaudit. Jangan reuse table hanya karena namanya sama jika ownership dan field tidak sesuai.

### Navigation and Menu

- Menu location: `header`, `footer`, dan optional `sidebar`.
- Nested menu memakai `parent_id` dengan maximum depth yang dikonfigurasi.
- Item type: internal page, external link, anchor, dan button.
- Target `same_tab` atau `new_tab`.
- Item active/inactive dan sortable.
- Internal page/anchor divalidasi dalam published snapshot.
- Navigation ikut masuk published version agar draft menu tidak bocor ke public page.

### SEO and Sharing

- Meta title/description/keywords, canonical URL, robots index/follow.
- Open Graph, Twitter card, dan social image.
- JSON-LD/schema markup tervalidasi.
- Sitemap inclusion dan priority.
- Redirect slug lama.
- Preview metadata untuk Vue.

### Publish Lifecycle

- Draft dapat diedit tanpa mengubah public version.
- Autosave draft memakai optimistic concurrency dan interval/rate limit.
- Revision history menyimpan snapshot perubahan bermakna, bukan setiap keystroke.
- Admin dapat membandingkan dua revision.
- Preview memakai auth atau token terbatas waktu.
- Publish membuat immutable snapshot/version.
- Unpublish menghentikan public serving tanpa menghapus draft.
- Rollback membuat draft dari version lama.
- Scheduled publish/unpublish diproses worker secara idempotent.
- Publish invalidates cache.

### Domain Resolution

- Platform subdomain, fallback path/slug, dan custom domain.
- Canonicalization: lowercase, trim trailing dot, tanpa scheme/path.
- Verification: `pending`, `verified`, `failed`, `disabled`.
- SSL provisioning berada di infrastructure integration.
- Canonical domain dapat memiliki redirect aliases.
- `organization_domains` dari capability multi-tenant adalah source of truth ownership, verification, dan SSL state.
- Landing hanya mengikat domain organization yang sudah verified ke page melalui `landing_domain_bindings`.
- Platform primary host terikat ke platform organization dan dapat diarahkan ke Landing Page marketing platform.

### Analytics

- Page view dan unique visitor approximation.
- CTA click, form view, form start, dan form submission.
- Conversion rate.
- UTM dan referrer.
- Device/browser category dan region bila tersedia dari trusted proxy.
- Top-performing page dan lead source attribution.
- Daily aggregate.

Raw event retention, bot filtering, consent, dan anonymization wajib dikonfigurasi. Full product analytics/data warehouse deferred.

### Media Reference

- Landing menyimpan media metadata: `asset_id`, type, folder, alt text, caption, dimensions/duration, variant, dan focal point.
- Upload/delete binary memakai `internal/platform/storage`.
- Tidak membuat `internal/modules/media`.
- Mendukung image, video, dan PDF/document sesuai allowlist.
- Validasi MIME, size, ownership, filename, dan usage reference.
- Image optimization/thumbnail memakai platform storage/image processor interface.
- Media dapat digunakan ulang dalam tenant dan dikelompokkan dengan folder/tag metadata.
- Public response tidak expose internal storage path.

### Notification Integration

Event minimum:

```txt
landing.submission_created
landing.page_published
landing.domain_verification_failed
landing.lead_delivery_failed
```

Landing hanya publish event/outbox. Template dan delivery tetap dimiliki `internal/core/notification`.

## Data Capacity Target

Target awal per organization, bukan hard limit permanen:

| Resource | Target |
| --- | ---: |
| Active landing pages | 100 |
| Sections per page | 100 |
| Forms per page | 20 |
| Fields per form | 100 |
| Published versions retained per page | 50 |
| Submissions per form | 1,000,000 dengan pagination/indexing |
| Raw analytics events | 10,000,000 sebelum archival strategy wajib |
| Custom domains | 20 |
| Branding social links | 20 |

API list wajib pagination. Export besar memakai streaming atau async job, bukan memuat seluruh data ke memory.

## Suggested Data Model

```txt
landing_pages
landing_page_sections
landing_section_templates
landing_ctas
landing_forms
landing_form_fields
landing_submissions
landing_submission_notes
landing_page_versions
landing_page_revisions
landing_domain_bindings
landing_brandings
landing_menus
landing_menu_items
landing_media_assets
landing_lead_integrations
landing_lead_delivery_logs
landing_analytics_events
landing_analytics_daily
landing_slug_redirects
```

JSONB dipakai untuk content/style/schema dinamis. Identity, ownership, status, ordering, searchable fields, timestamps, dan relational integrity tetap column biasa.

## Existing Schema Audit

Baseline arsip mempunyai:

- `landing_pages` dengan owner, code, domain, status, `config`, `seo`, `cta`, dan version.
- `brands` dengan integer ID dan field minimal.

Sebelum migration:

1. Periksa schema database aktual.
2. Tentukan apakah table lama benar-benar diterapkan.
3. Buat compatibility/cleanup migration terpisah.
4. Jangan mengedit migration lama.
5. Catat mapping dan data backfill.

## Permission Catalog

```txt
landing.page.read
landing.page.create
landing.page.update
landing.page.delete
landing.page.restore
landing.page.publish
landing.page.archive
landing.section.manage
landing.section_template.manage
landing.cta.manage
landing.media.manage
landing.form.manage
landing.submission.read
landing.submission.update
landing.submission.delete
landing.submission.export
landing.seo.manage
landing.branding.read
landing.branding.update
landing.theme.manage
landing.menu.manage
landing.domain.read
landing.domain.manage
landing.analytics.read
landing.preview
landing.integration.read
landing.integration.manage
```

## Security Baseline

- Tenant scope enforced di repository/service.
- Public form memakai rate limit, payload limit, bot mitigation, dan server validation.
- Rich text, URL, redirect, schema markup, dan optional HTML disanitasi.
- CSV export mencegah spreadsheet formula injection.
- PII tidak masuk application log.
- IP disimpan sebagai hash/anonymized value bila memungkinkan.
- Preview token di-hash, scoped, expired, dan revocable.
- Password-protected page memakai password hash, rate limit, dan short-lived access grant.
- File upload memakai MIME sniffing, size limit, random object key, dan malware scan hook.
- Webhook target mencegah SSRF ke private/link-local network dan secret disimpan terenkripsi.
- Domain verification tidak dapat di-bypass client.
- Publish dan branding update membuat audit log.
- Cache key memasukkan tenant/domain/page version.

## MVP Boundary

MVP:

- Page CRUD, section builder, form builder, public page, dan submission.
- Hero/About/Feature/CTA/Contact Form section.
- Branding/theme update dan basic SEO.
- Publish/unpublish, published snapshot, dan preview.
- Submission admin.
- Lead notification melalui core notification (email/WhatsApp/optional Discord) dan generic webhook.
- Basic page view, CTA click, dan conversion analytics.
- Permission, audit, migration, test, OpenAPI, dan frontend contract.

Post-MVP planned:

- Private/password page, scheduled publish, autosave/compare revision.
- Reusable section/template, reusable CTA, media folder/optimization, dan navigation builder.
- Advanced theme mode/layout.
- N8N preset, Telegram, Google Sheets, sales assignment, dan external CRM sync.

Deferred advanced:

- WYSIWYG penuh, template marketplace, arbitrary JavaScript, A/B testing, personalization.
- Translation workflow penuh, AI copywriting, dan AI page generator.
- Async large export worker.
- Custom domain automation, static generation, edge cache/CDN, heatmap, dan data warehouse integration.

## Non-Goals

- General-purpose CMS untuk seluruh domain bisnis.
- Menyimpan binary file di database Landing.
- Mengimplementasikan notification provider di Landing.
- Menyimpan arbitrary executable JavaScript.
- Memecah form, submission, branding, atau analytics menjadi module bisnis terpisah.
