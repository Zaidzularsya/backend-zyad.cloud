# Landing Page Public API Contract

Kontrak target untuk Frontend Vue, admin dashboard, dan public landing page renderer.

Source of truth final adalah `api/openapi.yaml`. Selama endpoint belum dibangun, seluruh endpoint berstatus planned dan dapat disempurnakan sebelum compatibility produksi dijanjikan.

## Base and Authentication

```txt
{APP_URL}/api/v1
```

Admin endpoint memakai:

```http
Authorization: Bearer <access_token>
```

Public endpoint menentukan tenant melalui host/domain atau public key yang diterbitkan server, bukan tenant ID bebas dari browser.

## Standard Response

```json
{
  "success": true,
  "message": "Request processed successfully",
  "data": {},
  "meta": null
}
```

```json
{
  "success": false,
  "message": "Validation failed",
  "error": {
    "code": "VALIDATION_ERROR",
    "details": {
      "slug": ["slug is already used"]
    }
  }
}
```

Pagination memakai `page`, `per_page`, `total`, dan `total_pages`.

## Error Codes

| Code | HTTP | Meaning |
| --- | ---: | --- |
| `VALIDATION_ERROR` | 422 | Request tidak valid |
| `UNAUTHORIZED` | 401 | Token tidak valid |
| `FORBIDDEN` | 403 | Permission tidak cukup |
| `NOT_FOUND` | 404 | Resource tidak ditemukan dalam tenant scope |
| `CONFLICT` | 409 | Slug/domain/state conflict |
| `RATE_LIMITED` | 429 | Public request terlalu sering |
| `LANDING_NOT_PUBLISHED` | 404 | Page tidak published |
| `LANDING_INVALID_STATE` | 409 | Operasi tidak valid untuk status saat ini |
| `LANDING_PUBLISH_VALIDATION_FAILED` | 422 | Publish checklist gagal |
| `LANDING_DOMAIN_NOT_VERIFIED` | 409 | Domain belum verified |
| `LANDING_FORM_INACTIVE` | 422 | Form tidak aktif |
| `LANDING_SUBMISSION_DUPLICATE` | 409 | Idempotency key sudah dipakai |
| `LANDING_PREVIEW_TOKEN_INVALID` | 404 | Preview token invalid/expired |

## Shared Data

Page summary:

```json
{
  "id": "uuid",
  "name": "Ramadan Campaign",
  "title": "Internet Cepat untuk Keluarga",
  "slug": "ramadan-2026",
  "page_type": "campaign",
  "status": "draft",
  "visibility": "public",
  "locale": "id-ID",
  "is_homepage": false,
  "publish_at": null,
  "unpublish_at": null,
  "published_version": 3,
  "published_at": "2026-06-13T10:00:00Z",
  "updated_at": "2026-06-13T11:00:00Z"
}
```

Section:

```json
{
  "id": "uuid",
  "key": "hero-main",
  "type": "hero",
  "name": "Main Hero",
  "sort_order": 10,
  "is_enabled": true,
  "content": {
    "heading": "Internet lebih cepat",
    "description": "Paket rumah mulai Rp250.000"
  },
  "style": {
    "variant": "image_right",
    "background_color": "surface"
  }
}
```

Branding:

```json
{
  "company_name": "Zyad Cloud",
  "tagline": "Connected business platform",
  "logo_light_url": "https://cdn.example.com/logo-light.svg",
  "logo_dark_url": "https://cdn.example.com/logo-dark.svg",
  "favicon_url": "https://cdn.example.com/favicon.png",
  "social_image_url": "https://cdn.example.com/social.jpg",
  "colors": {
    "primary": "#2563EB",
    "secondary": "#0F172A",
    "accent": "#F59E0B",
    "background": "#FFFFFF",
    "surface": "#F8FAFC",
    "text": "#0F172A",
    "muted": "#64748B"
  },
  "typography": {
    "heading_font": "Inter",
    "body_font": "Inter"
  },
  "shape": {
    "button_radius": "8px",
    "card_radius": "12px"
  },
  "layout": {
    "width": "wide",
    "spacing": "comfortable",
    "background_style": "solid",
    "color_mode": "system",
    "header_style": "default",
    "footer_style": "default"
  },
  "contact": {
    "email": "hello@example.com",
    "phone": "+628123456789",
    "address": "Jakarta, Indonesia"
  },
  "social_links": []
}
```

## Admin Page API

### GET /admin/landing-pages

Permission: `landing.page.read`.

Query: `page`, `per_page`, `search`, `status`, `page_type`, `sort_by`, `sort_order`, `include_deleted`.

### POST /admin/landing-pages

Permission: `landing.page.create`.

```json
{
  "name": "Ramadan Campaign",
  "title": "Internet Cepat untuk Keluarga",
  "slug": "ramadan-2026",
  "page_type": "campaign",
  "visibility": "public",
  "locale": "id-ID",
  "is_homepage": false
}
```

Page baru selalu `draft`.

Allowed `visibility`:

```txt
public
private
password_protected
```

### GET /admin/landing-pages/:id

Permission: `landing.page.read`.

Mencakup settings, SEO draft, sections, forms summary, branding, domain, dan publish state.

### PATCH /admin/landing-pages/:id

Permission: `landing.page.update`. Request partial.

### DELETE /admin/landing-pages/:id

Permission: `landing.page.delete`. Soft delete.

### POST /admin/landing-pages/:id/restore

Permission: `landing.page.restore`.

### POST /admin/landing-pages/:id/duplicate

Permission: `landing.page.create`.

```json
{
  "name": "Ramadan Campaign Copy",
  "slug": "ramadan-2026-copy",
  "include_forms": true
}
```

Submission dan analytics tidak diduplicate.

### POST /admin/landing-pages/:id/archive

Permission: `landing.page.archive`.

Archive hanya berlaku untuk page yang tidak sedang published. Page archived tetap dapat dilihat admin tetapi tidak dapat dipublish sebelum dikembalikan menjadi draft.

### PUT /admin/landing-pages/:id/access

Permission: `landing.page.update`.

```json
{
  "visibility": "password_protected",
  "password": "one-time-plain-input"
}
```

Password hanya diterima untuk di-hash dan tidak pernah dikembalikan. Mengubah ke `public` atau `private` menghapus password hash yang tidak lagi diperlukan.

## SEO API

### PATCH /admin/landing-pages/:id/seo

Permission: `landing.seo.manage`.

```json
{
  "meta_title": "Internet Cepat untuk Keluarga",
  "meta_description": "Pilih paket internet rumah terbaik.",
  "meta_keywords": ["internet rumah", "promo internet"],
  "canonical_url": null,
  "robots": {
    "index": true,
    "follow": true
  },
  "open_graph": {
    "title": "Internet Cepat",
    "description": "Promo terbaru.",
    "image_asset_id": "uuid"
  },
  "twitter_card": "summary_large_image",
  "schema_markup": {
    "@context": "https://schema.org",
    "@type": "Organization",
    "name": "Example"
  },
  "sitemap": {
    "included": true,
    "priority": 0.8
  }
}
```

## Section API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing-pages/:id/sections` | `landing.page.read` |
| POST | `/admin/landing-pages/:id/sections` | `landing.section.manage` |
| PATCH | `/admin/landing-pages/:id/sections/:sectionId` | `landing.section.manage` |
| POST | `/admin/landing-pages/:id/sections/:sectionId/duplicate` | `landing.section.manage` |
| DELETE | `/admin/landing-pages/:id/sections/:sectionId` | `landing.section.manage` |
| PUT | `/admin/landing-pages/:id/sections/reorder` | `landing.section.manage` |

Create request mengikuti shared Section tanpa `id`.

Reorder:

```json
{
  "items": [
    {"id": "uuid-1", "sort_order": 10},
    {"id": "uuid-2", "sort_order": 20}
  ]
}
```

## Form API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing-pages/:id/forms` | `landing.page.read` |
| POST | `/admin/landing-pages/:id/forms` | `landing.form.manage` |
| PATCH | `/admin/landing-pages/:id/forms/:formId` | `landing.form.manage` |
| DELETE | `/admin/landing-pages/:id/forms/:formId` | `landing.form.manage` |
| PUT | `/admin/landing-pages/:id/forms/:formId/fields` | `landing.form.manage` |

Create form:

```json
{
  "name": "Lead Form",
  "key": "lead-form",
  "is_active": true,
  "submit_label": "Kirim",
  "success_message": "Terima kasih, kami akan menghubungi Anda.",
  "redirect_url": null,
  "consent": {
    "required": true,
    "label": "Saya menyetujui kebijakan privasi",
    "privacy_url": "https://example.com/privacy"
  }
}
```

Replace fields atomically:

```json
{
  "fields": [
    {
      "key": "full_name",
      "type": "text",
      "label": "Nama lengkap",
      "required": true,
      "sort_order": 10,
      "validation": {
        "min_length": 2,
        "max_length": 150
      }
    },
    {
      "key": "email",
      "type": "email",
      "label": "Email",
      "required": true,
      "sort_order": 20,
      "validation": {}
    },
    {
      "key": "proposal",
      "type": "file",
      "label": "Proposal",
      "required": false,
      "sort_order": 30,
      "validation": {
        "allowed_mime_types": ["application/pdf"],
        "max_size_bytes": 5242880,
        "max_files": 1
      }
    }
  ]
}
```

## Publish and Preview API

### POST /admin/landing-pages/:id/validate-publish

Permission: `landing.page.publish`.

```json
{
  "valid": false,
  "errors": [
    {
      "path": "sections.hero-main.content.heading",
      "code": "REQUIRED",
      "message": "Hero heading is required"
    }
  ],
  "warnings": []
}
```

### POST /admin/landing-pages/:id/publish

Permission: `landing.page.publish`.

```json
{
  "change_note": "Publish campaign June",
  "expected_updated_at": "2026-06-13T11:00:00Z"
}
```

### POST /admin/landing-pages/:id/unpublish

Permission: `landing.page.publish`.

### GET /admin/landing-pages/:id/versions

Permission: `landing.page.read`.

### GET /admin/landing-pages/:id/versions/:version

Permission: `landing.page.read`.

### POST /admin/landing-pages/:id/versions/:version/restore

Permission: `landing.page.publish`. Restore membuat draft, bukan langsung publish.

### POST /admin/landing-pages/:id/preview-token

Permission: `landing.preview`.

Response hanya mengembalikan preview URL/token sekali dan expiry.

## Branding API

| Method | Endpoint | Purpose | Permission |
| --- | --- | --- | --- |
| GET | `/admin/landing/branding` | Organization default | `landing.branding.read` |
| PATCH | `/admin/landing/branding` | Update default | `landing.branding.update` |
| GET | `/admin/landing-pages/:id/branding` | Override dan effective | `landing.branding.read` |
| PATCH | `/admin/landing-pages/:id/branding` | Update page override | `landing.branding.update` |
| DELETE | `/admin/landing-pages/:id/branding` | Remove override | `landing.branding.update` |

PATCH bersifat partial dan mengikuti shared Branding type. Arbitrary script/CSS tidak diterima pada MVP.

### GET /admin/landing/theme

Permission: `landing.branding.read`.

Mengembalikan effective theme token organization untuk editor Vue.

### PATCH /admin/landing/theme

Permission: `landing.theme.manage`.

Mengubah color mode, spacing scale, background style, layout width, button/card style, serta header/footer style. Custom CSS tidak termasuk endpoint MVP.

## CTA API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/ctas` | `landing.cta.manage` |
| POST | `/admin/landing/ctas` | `landing.cta.manage` |
| PATCH | `/admin/landing/ctas/:id` | `landing.cta.manage` |
| DELETE | `/admin/landing/ctas/:id` | `landing.cta.manage` |

```json
{
  "name": "Primary WhatsApp CTA",
  "label": "Konsultasi Gratis",
  "type": "whatsapp",
  "target": "new_tab",
  "destination": "https://wa.me/628123456789",
  "tracking_key": "primary-whatsapp"
}
```

Allowed type: `contact_form`, `whatsapp`, `external_link`, `internal_page`, dan `document_download`.

## Section Template API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/section-templates` | `landing.section_template.manage` |
| POST | `/admin/landing/section-templates` | `landing.section_template.manage` |
| PATCH | `/admin/landing/section-templates/:id` | `landing.section_template.manage` |
| DELETE | `/admin/landing/section-templates/:id` | `landing.section_template.manage` |
| POST | `/admin/landing-pages/:id/sections/from-template` | `landing.section.manage` |

Instantiation menggunakan copy-on-use; response section baru tidak tetap terikat ke perubahan template.

## Media API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/media` | `landing.media.manage` |
| POST | `/admin/landing/media` | `landing.media.manage` |
| PATCH | `/admin/landing/media/:id` | `landing.media.manage` |
| DELETE | `/admin/landing/media/:id` | `landing.media.manage` |

Upload memakai `multipart/form-data` dan menerima image, video, atau PDF sesuai allowlist backend. Response mengembalikan opaque `asset_id`, metadata, processing status, dan URL yang aman, bukan storage path.

## Navigation API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/menus` | `landing.menu.manage` |
| POST | `/admin/landing/menus` | `landing.menu.manage` |
| PATCH | `/admin/landing/menus/:id` | `landing.menu.manage` |
| DELETE | `/admin/landing/menus/:id` | `landing.menu.manage` |
| PUT | `/admin/landing/menus/:id/items` | `landing.menu.manage` |

Menu item mendukung `parent_id`, internal page, external link, anchor, button, target, active state, dan sort order. Backend menolak cycle serta depth melebihi batas.

## Submission Admin API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing-submissions` | `landing.submission.read` |
| GET | `/admin/landing-submissions/:id` | `landing.submission.read` |
| PATCH | `/admin/landing-submissions/:id/status` | `landing.submission.update` |
| POST | `/admin/landing-submissions/:id/notes` | `landing.submission.update` |
| DELETE | `/admin/landing-submissions/:id` | `landing.submission.delete` |
| GET | `/admin/landing-submissions/export` | `landing.submission.export` |

List/export filter: page, form, status, date range, UTM source/campaign, search, dan sort.

Status request:

```json
{
  "status": "contacted",
  "reason": "Contacted by sales"
}
```

Export menggunakan streamed CSV; formula spreadsheet harus di-escape.

## Domain API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/domains/available` | `landing.domain.read` |
| GET | `/admin/landing/domain-bindings` | `landing.domain.read` |
| POST | `/admin/landing/domain-bindings` | `landing.domain.manage` |
| PATCH | `/admin/landing/domain-bindings/:id` | `landing.domain.manage` |
| DELETE | `/admin/landing/domain-bindings/:id` | `landing.domain.manage` |

Registration dan verification domain dilakukan melalui API Organization multi-tenant. Landing hanya dapat memilih domain berstatus verified milik active organization.

Create binding:

```json
{
  "organization_domain_id": "uuid",
  "landing_page_id": "uuid",
  "is_primary": true
}
```

Response menyertakan canonical host serta domain/SSL status dari Organization capability tanpa mengekspos provider credential.

## Analytics Admin API

### GET /admin/landing-analytics/summary

Permission: `landing.analytics.read`.

Query: `landing_page_id`, `date_from`, `date_to`, `timezone`.

```json
{
  "page_views": 12500,
  "unique_visitors": 8200,
  "cta_clicks": 2100,
  "form_starts": 850,
  "submissions": 420,
  "conversion_rate": 3.36
}
```

### GET /admin/landing-analytics/timeseries

Permission: `landing.analytics.read`.

Query tambahan: `interval=day` dan metric allowlist.

### GET /admin/landing-analytics/sources

Permission: `landing.analytics.read`. Return UTM/referrer breakdown.

### GET /admin/landing-analytics/top-pages

Permission: `landing.analytics.read`. Return page view, visitor, submission, dan conversion ranking.

### GET /admin/landing-analytics/devices

Permission: `landing.analytics.read`. Return device/browser breakdown tanpa raw fingerprint.

## Revision and Schedule API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing-pages/:id/revisions` | `landing.page.read` |
| GET | `/admin/landing-pages/:id/revisions/compare` | `landing.page.read` |
| POST | `/admin/landing-pages/:id/revisions/:revision/restore` | `landing.page.update` |
| PUT | `/admin/landing-pages/:id/schedule` | `landing.page.publish` |
| DELETE | `/admin/landing-pages/:id/schedule` | `landing.page.publish` |

Schedule:

```json
{
  "publish_at": "2026-06-20T09:00:00+07:00",
  "unpublish_at": "2026-07-01T00:00:00+07:00",
  "timezone": "Asia/Jakarta"
}
```

Autosave memakai endpoint update page/section/form yang sama dengan optimistic concurrency. Backend membuat revision berdasarkan meaningful change dan debounce policy.

## Lead Integration API

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/admin/landing/lead-integrations` | `landing.integration.read` |
| POST | `/admin/landing/lead-integrations` | `landing.integration.manage` |
| PATCH | `/admin/landing/lead-integrations/:id` | `landing.integration.manage` |
| DELETE | `/admin/landing/lead-integrations/:id` | `landing.integration.manage` |
| POST | `/admin/landing/lead-integrations/:id/test` | `landing.integration.manage` |
| GET | `/admin/landing/lead-deliveries` | `landing.integration.read` |
| POST | `/admin/landing/lead-deliveries/:id/retry` | `landing.integration.manage` |

Integration type minimum adalah `notification` dan `webhook`. CRM, Telegram, Google Sheets, dan sales assignment adalah optional adapters. Secret hanya diterima saat create/rotate dan selalu di-mask pada response.

## Public API

### GET /public/landing/resolve

Host adalah sumber utama tenant/domain. Query `slug` hanya untuk fallback route.

Response:

```json
{
  "page": {
    "id": "public-opaque-id",
    "title": "Internet Cepat untuk Keluarga",
    "slug": "ramadan-2026",
    "locale": "id-ID",
    "version": 4
  },
  "branding": {},
  "seo": {},
  "sections": [],
  "forms": [],
  "canonical_url": "https://promo.example.com/",
  "published_at": "2026-06-13T10:00:00Z"
}
```

Response boleh memakai `ETag` dari published version. Draft update tidak mengubah public ETag.

### GET /public/landing/preview/:token

Wajib mengirim:

```http
X-Robots-Tag: noindex, nofollow
Cache-Control: no-store
```

### POST /public/landing/access/:publicPageId

Hanya untuk `password_protected`.

```json
{
  "password": "visitor-input"
}
```

Response mengembalikan short-lived access grant yang scoped ke page/version. Endpoint wajib rate limited dan tidak membedakan pesan antara page/password invalid.

### POST /public/landing/forms/:publicKey/uploads

Temporary upload untuk field form bertipe `file`. Endpoint memakai `multipart/form-data`, rate limit, CAPTCHA policy jika diperlukan, dan validasi schema form sebelum menerima binary.

Response:

```json
{
  "upload_token": "opaque-upload-token",
  "expires_at": "2026-06-13T12:30:00Z"
}
```

Upload yang tidak diklaim oleh submission dibersihkan setelah expiry.

### POST /public/landing/forms/:publicKey/submissions

Header:

```http
Idempotency-Key: client-generated-uuid
```

Request:

```json
{
  "fields": {
    "full_name": "Jane Doe",
    "email": "jane@example.com",
    "phone": "+628123456789"
  },
  "consent": true,
  "context": {
    "page_version": 4,
    "utm_source": "instagram",
    "utm_medium": "social",
    "utm_campaign": "ramadan-2026",
    "referrer": "https://instagram.com/"
  },
  "captcha_token": "provider-token",
  "website": "",
  "uploaded_files": {
    "proposal": ["opaque-upload-token"]
  }
}
```

`website` adalah honeypot dan harus kosong.
File di-upload melalui flow sementara yang dibatasi form/public key, kemudian diklaim saat submission berhasil.

Response:

```json
{
  "reference": "SUB-20260613-OPAQUE",
  "success_message": "Terima kasih, kami akan menghubungi Anda.",
  "redirect_url": null
}
```

### POST /public/landing/events

Allowlist: `page_view`, `cta_click`, `form_view`, `form_start`.

Submission dihitung server-side setelah submit berhasil.

```json
{
  "event": "cta_click",
  "page_id": "public-opaque-id",
  "page_version": 4,
  "section_key": "hero-main",
  "target_key": "primary-action",
  "session_id": "anonymous-client-session-id",
  "context": {
    "utm_source": "instagram",
    "utm_campaign": "ramadan-2026"
  }
}
```

## Vue Consumption Notes

- Gunakan `id` untuk mutation admin, bukan slug.
- Pisahkan draft editor state dari public resolved response.
- Kirim `expected_updated_at` atau version untuk optimistic concurrency.
- Render section hanya dari allowlisted `type`.
- Hindari `v-html` kecuali backend menandai payload sanitized dan renderer tetap memiliki policy.
- Gunakan public form schema dari published response.
- Jangan menyimpan PII submission lebih lama dari kebutuhan UI.
- Pakai `Idempotency-Key` stabil saat retry submit.
- Theme memakai effective branding dan accessible fallback.

## Contract Change Rules

- Breaking change dicatat di traceability index.
- Endpoint final wajib masuk `api/openapi.yaml`.
- Rename field membutuhkan migration strategy frontend.
- Public payload harus mempertimbangkan published page yang masih di-cache.
