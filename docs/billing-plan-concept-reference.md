# Billing Plan Concept Reference

> **Catatan riwayat (refactor domain-split)**: Dokumen ini adalah rujukan desain awal sebelum modul
> `billing` dipecah menjadi 3 domain eksplisit (Product/Catalog, Subscription, Billing). Tabel
> `billing_plans`, `billing_plan_prices`, `billing_features`, `billing_plan_entitlements` di dokumen ini
> sudah digantikan oleh `product_plans`, `product_plan_prices`, `product_features`,
> `product_plan_entitlements`; tabel `billing_subscriptions`/`billing_subscription_events` digantikan oleh
> `customer_subscriptions`/`subscription_events`. Struktur final ada di
> [product-subscription-billing-concept.md](product-subscription-billing-concept.md). Dokumen ini
> dipertahankan sebagai riwayat/rujukan konsep bisnis awal (masih valid untuk requirement/flow bisnis),
> bukan untuk struktur tabel/kode saat ini.

Dokumen ini menetapkan konsep Plan, Billing, Subscription, Entitlement,
Quota, Invoice, dan Payment untuk Zyad Cloud.

Sumber konteks:

- `README.md`
- `AGENTS.md`
- `docs/reference-plan-billing-subscribe.md`
- `docs/reference-multi-tenant.md`
- `docs/multi-tenant-development-tasks.md`
- `docs/multi-tenant-traceability-index.md`
- `docs/migration-guide.md`
- Implementasi existing `internal/modules/organization`
- Implementasi existing `internal/modules/landing`

## Overview

Module ini mengatur monetisasi SaaS berbasis organization. Dalam konteks
Zyad Cloud, `organization` adalah billing account. Semua subscription,
invoice, payment, entitlement runtime, dan quota harus dikaitkan dengan
`organization_id` yang valid.

Core concept:

| Concept | Meaning |
| --- | --- |
| Organization | Billing account dan tenant bisnis canonical. |
| Plan | Paket bisnis seperti `free`, `starter`, `growth`, `business`, atau `enterprise`. |
| Feature | Kemampuan platform yang bisa dikontrol per paket. |
| Plan entitlement | Default hak fitur dan limit dari sebuah plan. |
| Organization entitlement | Snapshot runtime hak fitur milik organization. |
| Subscription | Kontrak aktif organization terhadap sebuah plan. |
| Invoice | Tagihan atas subscription, add-on, adjustment, tax, atau discount. |
| Payment | Catatan transaksi pembayaran invoice. |
| Usage counter | Pencatatan pemakaian terhadap quota periodik. |

Billing logic tidak boleh hardcode berdasarkan nama plan:

```go
if plan == "starter" {
}
```

Runtime access control harus berbasis entitlement dan quota:

```go
guard.RequireFeature(ctx, organizationID, "landing.enabled")
guard.CheckQuota(ctx, organizationID, "landing.max_pages", 1)
```

## Existing Repository Alignment

Repo ini sudah memiliki foundation multi-tenant yang penting:

| Existing capability | Location | Billing implication |
| --- | --- | --- |
| Organization sebagai tenant canonical | `internal/modules/organization` | Billing wajib memakai `organization_id`, bukan `tenant_id`. |
| Runtime entitlement | `organization_entitlements` pada migration `000016` | Jangan membuat runtime entitlement source kedua tanpa migration deprecation. |
| Usage counter | `organization_usage_counters` pada migration `000016` | Quota periodik dapat memakai counter existing. |
| Feature/usage API tenant | `/api/v1/organization/features`, `/api/v1/organization/usage` | Tenant billing dashboard dapat reuse atau memberi agregasi baru. |
| Platform organization dan customer organization | `docs/reference-multi-tenant.md` | Platform admin billing harus dijaga dengan platform tenant/permission. |
| Landing access policy | `internal/modules/landing/service/access_policy.go` | Billing guard harus bisa diintegrasikan tanpa mencampur RBAC dan entitlement. |
| Route wiring aktif | `internal/app/router.go` | Billing handler protected harus di-wire seperti organization/landing, bukan route public placeholder. |

Keputusan desain MVP:

- `internal/modules/billing` menjadi owner untuk plan catalog, subscription,
  invoice, payment, dan billing event.
- Runtime entitlement tetap menggunakan `organization_entitlements` existing.
- Runtime usage counter tetap menggunakan `organization_usage_counters` existing.
- `billing_plan_entitlements` adalah template entitlement plan.
- Saat subscription dibuat atau plan berubah, billing service melakukan sync ke
  `organization_entitlements` dengan `source = 'plan'` dan
  `source_reference = subscription_id`.
- Jangan membuat tabel `billing_organization_entitlements` pada MVP kecuali ada
  migration eksplisit untuk memindahkan/deprecate `organization_entitlements`.
- Jangan membuat tabel `billing_usage_counters` pada MVP kecuali ada kebutuhan
  metered billing event yang berbeda dari runtime quota counter.

## Business Goals

- Mendukung monetisasi SaaS multi-tenant.
- Mendukung plan `free`, `trial`, `starter`, `growth`, `business`, dan
  `enterprise`.
- Mendukung fitur modular berdasarkan paket.
- Mendukung upgrade dan downgrade secara aman.
- Mendukung add-on di fase berikutnya.
- Mendukung billing manual MVP dan integrasi payment gateway setelah kontrak
  domain stabil.
- Mendukung tenant-level entitlement agar paket fleksibel dan tidak hardcoded.
- Menjaga tenant lama tetap stabil saat isi paket berubah.

## Scope

Scope MVP:

- CRUD plan.
- CRUD feature catalog.
- CRUD plan prices.
- Manage plan entitlements.
- Create/update organization subscription.
- Sync organization entitlement snapshot ke `organization_entitlements`.
- Check subscription status.
- Check feature access.
- Check quota.
- Invoice dan invoice items.
- Manual payment record.
- Payment event log untuk kesiapan webhook.
- Subscription event log.
- Migration dan seed default plan/features/entitlements.
- OpenAPI contract untuk platform admin dan tenant dashboard.

Out of scope MVP:

- Full recurring payment otomatis.
- Proration kompleks.
- Usage-based overage billing.
- Tax engine kompleks.
- Payment gateway production integration.
- Coupon/discount advanced.
- Add-on marketplace.
- Reseller billing.

Walau out of scope, schema tidak boleh menutup kemungkinan extension tersebut.

## Domain Boundary

| Concern | Owner |
| --- | --- |
| Plan catalog, prices, plan entitlements | `internal/modules/billing` |
| Subscription lifecycle | `internal/modules/billing` |
| Invoice dan invoice items | `internal/modules/billing` |
| Payment record dan provider event | `internal/modules/billing` untuk record, `internal/platform/xendit` atau provider adapter untuk integrasi teknis |
| Runtime organization entitlement | `internal/modules/organization` table/service existing, disinkronkan oleh billing |
| Runtime organization usage counter | `internal/modules/organization` table/service existing |
| RBAC permission | `internal/core/permission` |
| Tenant resolution | `internal/core/middleware`, `internal/core/tenant` |
| Landing page feature enforcement | `internal/modules/landing` memakai billing/entitlement guard |

Billing tidak boleh mengambil alih authorization user-level. Billing hanya
menjawab apakah organization berhak memakai fitur dan masih punya quota.
Permission/RBAC menjawab apakah user boleh menjalankan aksi.

## Core Domain Model

### Plan

Plan adalah template paket bisnis.

Contoh:

- `free`
- `starter`
- `growth`
- `business`
- `enterprise`

Plan tidak menjadi satu-satunya sumber runtime access control. Service lain
tidak boleh bertanya "organization ini plan apa?" untuk membuka fitur. Service
lain harus bertanya "organization ini punya entitlement fitur apa dan limitnya
berapa?"

### Plan Price

Plan price adalah opsi harga plan per interval dan currency.

Contoh:

- Starter monthly IDR 99000.
- Starter yearly IDR 990000.
- Enterprise custom/manual.

### Feature

Feature adalah definisi kemampuan platform yang stabil.

Contoh:

- `landing.enabled`
- `landing.max_pages`
- `landing.custom_domain`
- `crm.enabled`
- `crm.max_contacts`
- `users.max_users`
- `media.max_storage_mb`
- `whatsapp.max_messages_per_month`

Feature key harus stabil, lowercase, dan tidak diganti sembarangan karena akan
dipakai lintas module, seed, entitlement, UI, dan audit.

### Plan Entitlement

Plan entitlement adalah default value fitur dari sebuah plan.

Contoh:

- Growth memiliki `landing.max_pages = 10`.
- Growth memiliki `landing.custom_domain = true`.
- Growth memiliki `users.max_users = 10`.

### Organization Entitlement

Organization entitlement adalah snapshot runtime entitlement aktif milik tenant.
Repo ini sudah memiliki table existing:

```txt
organization_entitlements
```

Alasan snapshot dibutuhkan:

- Plan adalah template.
- Organization entitlement adalah runtime source of truth.
- Jika harga/paket berubah, tenant lama tidak otomatis rusak.
- Enterprise dapat diberi override.
- Runtime check lebih ringan dan tidak perlu join subscription/plan terus.

Source precedence existing dari multi-tenant adalah:

1. `platform_override`
2. `addon`
3. `trial`
4. `plan`

Billing sync untuk subscription memakai:

```txt
source = plan
source_reference = <billing_subscriptions.id>
```

### Subscription

Subscription menyatakan organization sedang memakai plan tertentu.

Status minimal:

- `trialing`
- `active`
- `past_due`
- `grace_period`
- `suspended`
- `canceled`
- `expired`

Satu organization hanya boleh punya satu subscription usable pada waktu yang
sama. Usable status untuk runtime guard:

- `trialing`
- `active`
- `grace_period`

Status `past_due` dapat diperlakukan usable atau non-usable sesuai policy
dunning. MVP merekomendasikan `past_due` masih usable terbatas sampai
`grace_period` berakhir.

### Invoice

Invoice adalah tagihan atas subscription atau item billing lain.

Status invoice:

- `draft`
- `open`
- `paid`
- `void`
- `expired`
- `failed`

Invoice tidak boleh dianggap paid hanya dari request client. Payment manual
harus dilakukan oleh platform admin. Payment gateway harus diverifikasi dari
provider/webhook.

### Payment

Payment adalah pencatatan pembayaran terhadap invoice.

Provider awal:

- `manual`
- `xendit`
- `midtrans`

Status payment:

- `pending`
- `paid`
- `failed`
- `expired`
- `refunded`

Payment dan webhook harus idempotent. Mark invoice paid pada invoice yang sudah
paid harus aman dan tidak menggandakan event.

### Usage Counter

Usage counter mencatat pemakaian fitur yang memiliki limit periodik.

Repo ini sudah memiliki table existing:

```txt
organization_usage_counters
```

Contoh usage:

- Jumlah pesan WhatsApp per bulan.
- Jumlah automation run per bulan.
- Jumlah transaksi POS per bulan.

Untuk snapshot count quota seperti landing page count, custom domain count, dan
user count, MVP sebaiknya menghitung langsung dari table sumber agar tidak ada
counter stale.

## Feature Key Reference

### Core Users

| Feature key | Value type | Unit |
| --- | --- | --- |
| `users.max_users` | integer | user |
| `users.invite_user` | boolean | - |
| `roles.custom_roles` | boolean | - |

### Landing Page

| Feature key | Value type | Unit |
| --- | --- | --- |
| `landing.enabled` | boolean | - |
| `landing.max_pages` | integer | page |
| `landing.max_sections_per_page` | integer | section |
| `landing.custom_domain` | boolean | - |
| `landing.remove_branding` | boolean | - |
| `landing.analytics` | boolean | - |

Catatan existing: `internal/modules/landing/domain/feature.go` saat ini sudah
memakai `landing.enabled`. Billing MVP mempertahankan prefix existing
`landing.*` agar implementasi tidak perlu transisi feature key pada module
Landing.

### CRM

| Feature key | Value type | Unit |
| --- | --- | --- |
| `crm.enabled` | boolean | - |
| `crm.max_contacts` | integer | contact |
| `crm.import_export` | boolean | - |
| `crm.pipeline` | boolean | - |
| `crm.lead_form` | boolean | - |

### Media

| Feature key | Value type | Unit |
| --- | --- | --- |
| `media.enabled` | boolean | - |
| `media.max_storage_mb` | integer | MB |
| `media.max_file_size_mb` | integer | MB |

### Domain

| Feature key | Value type | Unit |
| --- | --- | --- |
| `domain.enabled` | boolean | - |
| `domain.max_custom_domains` | integer | domain |
| `domain.ssl_auto_provision` | boolean | - |

### WhatsApp

| Feature key | Value type | Unit |
| --- | --- | --- |
| `whatsapp.enabled` | boolean | - |
| `whatsapp.max_messages_per_month` | integer | message/month |
| `whatsapp.template_message` | boolean | - |

### POS, Membership, Automation

| Feature key | Value type | Unit |
| --- | --- | --- |
| `pos.enabled` | boolean | - |
| `pos.max_products` | integer | product |
| `pos.max_transactions_per_month` | integer | transaction/month |
| `membership.enabled` | boolean | - |
| `membership.max_members` | integer | member |
| `automation.enabled` | boolean | - |
| `automation.max_runs_per_month` | integer | run/month |

## Database Design Reference

Gunakan PostgreSQL, UUID, `snake_case`, schema `public`, dan migration biasa.
Jangan memakai PostgreSQL native enum kecuali repo sudah memutuskan standar itu.
Gunakan `varchar` plus `check constraint` agar status mudah berkembang.

### `billing_plans`

Purpose: catalog paket bisnis.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `code varchar(80) not null`
- `name varchar(150) not null`
- `description text`
- `plan_type varchar(30) not null`
- `is_public boolean not null default true`
- `is_active boolean not null default true`
- `sort_order integer not null default 0`
- `metadata jsonb not null default '{}'::jsonb`
- `created_at timestamp without time zone not null default now()`
- `updated_at timestamp without time zone not null default now()`
- `deleted_at timestamp without time zone`

Constraints and indexes:

- Unique `code` where `deleted_at is null`.
- Check `plan_type in ('free', 'trial', 'paid', 'enterprise')`.
- Check `code = lower(code)` and stable slug pattern.
- Index `(is_public, is_active, sort_order)`.

### `billing_plan_prices`

Purpose: harga plan per interval dan currency.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `plan_id uuid not null references billing_plans(id)`
- `billing_interval varchar(30) not null`
- `currency varchar(3) not null default 'IDR'`
- `amount numeric(14,2) not null default 0`
- `is_active boolean not null default true`
- `metadata jsonb not null default '{}'::jsonb`
- timestamps dan `deleted_at`

Constraints and indexes:

- Unique `(plan_id, billing_interval, currency)` where `deleted_at is null`.
- Check `billing_interval in ('monthly', 'yearly', 'one_time', 'custom')`.
- Check `amount >= 0`.
- Check `currency = upper(currency)`.
- Index `(plan_id, is_active)`.

### `billing_features`

Purpose: catalog feature key yang bisa diberi entitlement.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `feature_key varchar(150) not null`
- `module varchar(80) not null`
- `name varchar(150) not null`
- `description text`
- `value_type varchar(30) not null`
- `unit varchar(50)`
- `reset_strategy varchar(30) not null default 'never'`
- `is_active boolean not null default true`
- timestamps

Constraints and indexes:

- Unique `feature_key`.
- Check `value_type in ('boolean', 'integer', 'string', 'decimal')`.
- Check `reset_strategy in ('never', 'monthly', 'yearly', 'custom')`.
- Check feature key lowercase dotted pattern.
- Index `(module, is_active)`.

### `billing_plan_entitlements`

Purpose: default entitlement value untuk plan.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `plan_id uuid not null references billing_plans(id)`
- `feature_id uuid not null references billing_features(id)`
- `value_bool boolean`
- `value_int bigint`
- `value_decimal numeric(14,2)`
- `value_string text`
- `limits jsonb not null default '{}'::jsonb`
- timestamps

Constraints and indexes:

- Unique `(plan_id, feature_id)`.
- Hanya satu value column boleh terisi sesuai `billing_features.value_type`.
- Check `limits` adalah object.
- Index `(feature_id)`.

Catatan: `limits` dipakai untuk kompatibilitas dengan existing
`organization_entitlements.limits`. Contoh:

```json
{
  "enabled": true,
  "max_pages": 10
}
```

Untuk feature boolean sederhana, `value_bool` cukup. Untuk limit spesifik,
gunakan `value_int` dan/atau `limits` sesuai kebutuhan service guard.

### `billing_subscriptions`

Purpose: kontrak plan aktif organization.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `organization_id uuid not null references organizations(id)`
- `plan_id uuid not null references billing_plans(id)`
- `status varchar(30) not null`
- `billing_interval varchar(30) not null`
- `current_period_start timestamp without time zone`
- `current_period_end timestamp without time zone`
- `trial_start timestamp without time zone`
- `trial_end timestamp without time zone`
- `cancel_at_period_end boolean not null default false`
- `canceled_at timestamp without time zone`
- `suspended_at timestamp without time zone`
- `metadata jsonb not null default '{}'::jsonb`
- timestamps

Constraints and indexes:

- Check `status in ('trialing', 'active', 'past_due', 'grace_period', 'suspended', 'canceled', 'expired')`.
- Check `billing_interval in ('monthly', 'yearly', 'custom')`.
- Check period end after start if both present.
- Partial unique: one usable subscription per organization where status in
  `('trialing', 'active', 'past_due', 'grace_period')`.
- Index `(organization_id, status)`.
- Index `(plan_id)`.
- Consider RLS because table is organization operational data.

### Runtime Entitlement Snapshot

Use existing table:

```txt
organization_entitlements
```

Billing sync rules:

- Insert/update rows with `source = 'plan'`.
- Set `source_reference = billing_subscriptions.id`.
- Set `effective_from = subscription.current_period_start` or `now()`.
- Set `effective_until = subscription.current_period_end` only when ending
  entitlement. Active plan entitlement should generally keep it null.
- Keep `limits` as structured JSONB.
- Do not overwrite `platform_override`, `addon`, or `trial` sources.

Do not create `billing_organization_entitlements` during MVP.

### Runtime Usage Counter

Use existing table:

```txt
organization_usage_counters
```

Billing usage policy:

- Periodic/metered quota uses this table.
- Snapshot resources can be counted from owner tables.
- Billing dashboard can expose aggregated usage through billing service, but
  source of truth remains existing organization service/table.

Do not create `billing_usage_counters` during MVP.

### `billing_invoices`

Purpose: tagihan organization.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `organization_id uuid not null references organizations(id)`
- `subscription_id uuid references billing_subscriptions(id)`
- `invoice_number varchar(80) not null`
- `status varchar(30) not null`
- `currency varchar(3) not null default 'IDR'`
- `subtotal_amount numeric(14,2) not null default 0`
- `discount_amount numeric(14,2) not null default 0`
- `tax_amount numeric(14,2) not null default 0`
- `total_amount numeric(14,2) not null default 0`
- `due_date timestamp without time zone`
- `paid_at timestamp without time zone`
- `metadata jsonb not null default '{}'::jsonb`
- timestamps

Constraints and indexes:

- Unique `invoice_number`.
- Check status in `draft`, `open`, `paid`, `void`, `expired`, `failed`.
- Check amounts are non-negative.
- Check `total_amount = subtotal_amount - discount_amount + tax_amount` can be
  enforced in service or generated/constraint after design is stable.
- Index `(organization_id, status, created_at desc)`.
- Index `(subscription_id)`.
- RLS recommended.

### `billing_invoice_items`

Purpose: item invoice yang membentuk total.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `invoice_id uuid not null references billing_invoices(id) on delete cascade`
- `item_type varchar(30) not null`
- `description text not null`
- `quantity numeric(14,2) not null default 1`
- `unit_amount numeric(14,2) not null default 0`
- `total_amount numeric(14,2) not null default 0`
- `metadata jsonb not null default '{}'::jsonb`
- timestamps

Constraints and indexes:

- Check item type in `subscription`, `addon`, `adjustment`, `tax`, `discount`.
- Check quantity and amount validity.
- Index `(invoice_id)`.

### `billing_payments`

Purpose: catatan pembayaran invoice.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `invoice_id uuid not null references billing_invoices(id)`
- `organization_id uuid not null references organizations(id)`
- `provider varchar(30) not null`
- `provider_reference varchar(150)`
- `payment_method varchar(80)`
- `status varchar(30) not null`
- `amount numeric(14,2) not null`
- `currency varchar(3) not null default 'IDR'`
- `paid_at timestamp without time zone`
- `raw_payload jsonb not null default '{}'::jsonb`
- timestamps

Constraints and indexes:

- Check provider in `manual`, `xendit`, `midtrans`.
- Check status in `pending`, `paid`, `failed`, `expired`, `refunded`.
- Check amount >= 0.
- Unique `(provider, provider_reference)` where `provider_reference is not null`.
- Index `(organization_id, status, created_at desc)`.
- Index `(invoice_id)`.
- RLS recommended.

### `billing_subscription_events`

Purpose: audit perubahan subscription.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `subscription_id uuid not null references billing_subscriptions(id)`
- `organization_id uuid not null references organizations(id)`
- `event_type varchar(80) not null`
- `old_status varchar(30)`
- `new_status varchar(30)`
- `actor_user_id uuid references users(id)`
- `metadata jsonb not null default '{}'::jsonb`
- `created_at timestamp without time zone not null default now()`

Indexes:

- `(subscription_id, created_at desc)`.
- `(organization_id, created_at desc)`.
- `(event_type, created_at desc)`.

### `billing_payment_events`

Purpose: audit provider event/webhook dan idempotency.

Key columns:

- `id uuid primary key default gen_random_uuid()`
- `payment_id uuid references billing_payments(id)`
- `invoice_id uuid references billing_invoices(id)`
- `provider varchar(30) not null`
- `event_type varchar(80) not null`
- `provider_event_id varchar(150)`
- `payload jsonb not null default '{}'::jsonb`
- `processed_at timestamp without time zone`
- `created_at timestamp without time zone not null default now()`

Constraints and indexes:

- Unique `(provider, provider_event_id)` where `provider_event_id is not null`.
- Index `(invoice_id, created_at desc)`.
- Index `(payment_id, created_at desc)`.
- Index `(provider, event_type, created_at desc)`.

## Service Design

Recommended service types:

| Service | Responsibility |
| --- | --- |
| `PlanService` | CRUD plan, price, activation, soft delete. |
| `FeatureService` | CRUD feature catalog and validation. |
| `PlanEntitlementService` | Manage plan entitlement template. |
| `SubscriptionService` | Create, activate, cancel, suspend, upgrade, downgrade. |
| `EntitlementSyncService` | Copy plan entitlement to `organization_entitlements`. |
| `InvoiceService` | Create invoice, calculate totals, list tenant invoices. |
| `PaymentService` | Manual mark-paid, payment record, provider event idempotency. |
| `UsageService` | Read usage summary and coordinate quota checks. |
| `BillingGuardService` | Runtime subscription, feature, and quota guard. |

Important methods:

- `GetActiveSubscription(ctx, organizationID)`
- `EnsureSubscriptionUsable(ctx, organizationID)`
- `HasFeature(ctx, organizationID, featureKey)`
- `RequireFeature(ctx, organizationID, featureKey)`
- `GetLimit(ctx, organizationID, featureKey, limitKey)`
- `CheckQuota(ctx, organizationID, featureKey, requestedIncrement)`
- `IncrementUsage(ctx, organizationID, featureKey, amount)`
- `SyncEntitlementsFromPlan(ctx, organizationID, subscriptionID, planID)`
- `CreateInvoiceForSubscription(ctx, subscriptionID)`
- `MarkInvoicePaid(ctx, invoiceID, actorUserID)`
- `ActivateSubscription(ctx, subscriptionID)`
- `SuspendSubscription(ctx, subscriptionID)`

Service rules:

- Service owns transaction boundary.
- Repository only performs database access.
- Payment mark-paid is idempotent.
- Subscription status transition must write `billing_subscription_events`.
- Payment provider event must write `billing_payment_events`.
- Entitlement sync must not overwrite non-plan source rows.
- Error codes must be stable for frontend.

## Access Control Flow

Saat user menjalankan action:

1. Resolve organization dari token, host, atau header oleh middleware existing.
2. Pastikan organization aktif.
3. Cek subscription organization.
4. Cek entitlement organization.
5. Cek quota/usage.
6. Cek role permission user.
7. Jalankan action.
8. Update usage counter jika action mengonsumsi quota periodik.

Untuk action yang sangat sensitif, RBAC dapat dicek sebelum quota agar tidak
membocorkan informasi paket kepada user yang tidak berhak. Namun secara domain,
RBAC dan entitlement tetap dua concern berbeda.

Perbedaan utama:

| Concern | Question |
| --- | --- |
| Plan entitlement | Apakah organization ini punya akses fitur tersebut? |
| Quota | Apakah organization ini masih punya limit? |
| RBAC permission | Apakah user ini boleh melakukan aksi tersebut dalam organization? |

## Quota Strategy

### Snapshot Count Quota

Quota dihitung dari table utama.

Contoh:

- `users.max_users` dihitung dari active organization membership/user.
- `landing.max_pages` dihitung dari active landing pages.
- `domain.max_custom_domains` dihitung dari verified/active organization domains.

Kelebihan:

- Tidak mudah stale.
- Cocok untuk resource yang jumlahnya relatif kecil.

### Periodic Usage Quota

Quota dicatat di `organization_usage_counters`.

Contoh:

- `whatsapp.max_messages_per_month`.
- `automation.max_runs_per_month`.
- `pos.max_transactions_per_month`.

Kelebihan:

- Cocok untuk event volume tinggi.
- Bisa reset per bulan/tahun/custom period.

## API Reference Draft

Semua path berikut berada di bawah prefix `/api/v1`.

### Platform Admin API

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/platform/billing/plans` | `platform.billing.plan.read` |
| POST | `/platform/billing/plans` | `platform.billing.plan.manage` |
| GET | `/platform/billing/plans/:id` | `platform.billing.plan.read` |
| PATCH | `/platform/billing/plans/:id` | `platform.billing.plan.manage` |
| DELETE | `/platform/billing/plans/:id` | `platform.billing.plan.manage` |
| GET | `/platform/billing/plans/:id/prices` | `platform.billing.plan_price.read` |
| POST | `/platform/billing/plans/:id/prices` | `platform.billing.plan_price.manage` |
| PATCH | `/platform/billing/plans/:id/prices/:priceId` | `platform.billing.plan_price.manage` |
| DELETE | `/platform/billing/plans/:id/prices/:priceId` | `platform.billing.plan_price.manage` |
| GET | `/platform/billing/features` | `platform.billing.feature.read` |
| POST | `/platform/billing/features` | `platform.billing.feature.manage` |
| PATCH | `/platform/billing/features/:id` | `platform.billing.feature.manage` |
| GET | `/platform/billing/plans/:id/entitlements` | `platform.billing.entitlement.read` |
| PUT | `/platform/billing/plans/:id/entitlements` | `platform.billing.entitlement.manage` |
| GET | `/platform/billing/subscriptions` | `platform.billing.subscription.read` |
| POST | `/platform/billing/subscriptions` | `platform.billing.subscription.manage` |
| PATCH | `/platform/billing/subscriptions/:id` | `platform.billing.subscription.manage` |
| POST | `/platform/billing/subscriptions/:id/cancel` | `platform.billing.subscription.manage` |
| POST | `/platform/billing/subscriptions/:id/suspend` | `platform.billing.subscription.manage` |
| GET | `/platform/billing/invoices` | `platform.billing.invoice.read` |
| POST | `/platform/billing/invoices` | `platform.billing.invoice.manage` |
| POST | `/platform/billing/invoices/:id/mark-paid` | `platform.billing.payment.manage` |

### Tenant Dashboard API

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/app/billing/current-plan` | `organization.billing.read` |
| GET | `/app/billing/usage` | `organization.billing.read` |
| GET | `/app/billing/invoices` | `organization.billing.read` |
| POST | `/app/billing/upgrade` | `organization.billing.manage` |
| POST | `/app/billing/cancel` | `organization.billing.manage` |

### Internal Service Contract

Internal guard sebaiknya interface Go, bukan endpoint publik. Endpoint internal
hanya dibuat jika benar-benar dibutuhkan untuk service boundary terpisah.

Suggested internal methods:

- `RequireUsableSubscription`
- `RequireFeature`
- `CheckQuota`
- `ConsumeUsage`
- `GetBillingSnapshot`

## Error Response Standard

Gunakan `internal/core/errors.AppError` dan response helper existing.

Error code:

- `SUBSCRIPTION_NOT_FOUND`
- `SUBSCRIPTION_INACTIVE`
- `SUBSCRIPTION_SUSPENDED`
- `FEATURE_NOT_ENABLED`
- `QUOTA_EXCEEDED`
- `PLAN_NOT_FOUND`
- `FEATURE_NOT_FOUND`
- `INVOICE_NOT_FOUND`
- `PAYMENT_ALREADY_PROCESSED`
- `INVALID_BILLING_STATUS`
- `BILLING_ENTITLEMENT_SYNC_FAILED`
- `PAYMENT_EVENT_ALREADY_PROCESSED`

Example payload:

```json
{
  "code": "QUOTA_EXCEEDED",
  "message": "Limit landing page pada paket Anda sudah tercapai.",
  "feature": "landing.max_pages",
  "used": 10,
  "limit": 10,
  "upgrade_available": true
}
```

## Upgrade and Downgrade Policy

### Upgrade

- Request upgrade membuat invoice.
- Untuk MVP tidak perlu prorate.
- Setelah invoice paid, subscription pindah ke plan baru.
- Entitlement baru disync dan aktif.
- Event `subscription_upgraded` dicatat.

### Downgrade

- Downgrade berlaku next billing cycle.
- Data tenant tidak dihapus.
- Resource yang melebihi limit baru menjadi read-only/locked.
- Tenant harus memilih data aktif jika limit baru lebih kecil.
- Event `subscription_downgrade_scheduled` dicatat.

## Grace Period and Suspension

Lifecycle:

1. Invoice due date lewat.
2. Subscription menjadi `past_due`.
3. Tenant masih bisa akses normal selama policy mengizinkan.
4. Subscription masuk `grace_period` dengan warning.
5. Setelah grace habis, subscription menjadi `suspended`.
6. Saat suspended, dashboard hanya boleh akses billing dan data recovery yang
   diperlukan.

Public landing page saat suspended harus diputuskan eksplisit. Rekomendasi MVP:

- Platform marketing organization tidak terpengaruh subscription customer.
- Customer public landing page tetap read-only selama `grace_period`.
- Saat `suspended`, public landing dapat menampilkan fallback unavailable page
  atau tetap hidup sesuai policy bisnis.

## Security and Audit

- Platform admin billing hanya role/permission tertentu.
- Tenant hanya boleh melihat invoice/subscription organization sendiri.
- Payment webhook harus idempotent.
- Provider event ID harus unique jika tersedia.
- Jangan percaya amount dari client.
- Payment status harus diverifikasi dari provider/webhook.
- Semua perubahan subscription dicatat di `billing_subscription_events`.
- Semua event payment gateway dicatat di `billing_payment_events`.
- Manual mark-paid harus menyimpan `actor_user_id`.
- Jangan log secret provider atau raw payment credential.
- Billing route protected wajib memakai tenant context existing.
- Tenant-owned table billing sebaiknya ikut RLS helper existing.

## Migration Recommendation

Migration terakhir saat dokumen ini dibuat adalah `000049`.

Rekomendasi urutan:

- `000050_create_billing_plan_catalog`
- `000051_create_billing_subscriptions`
- `000052_create_billing_invoices_payments`
- `000053_create_billing_events`
- `000054_seed_billing_permissions`
- `000055_seed_billing_features_plans`
- `000056_seed_billing_plan_entitlements`

Jangan membuat `billing_organization_entitlements` dan
`billing_usage_counters` pada MVP karena fungsi runtime sudah ada di migration
`000016_create_organization_entitlements`.

## Seed Recommendation

Seed harus idempotent memakai `INSERT ... ON CONFLICT ... DO UPDATE` dan tidak
boleh menghapus data custom user.

Default plans:

- `free`
- `starter`
- `growth`
- `business`
- `enterprise`

Default feature catalog minimal:

- `users.max_users`
- `users.invite_user`
- `roles.custom_roles`
- `landing.enabled`
- `landing.max_pages`
- `landing.max_sections_per_page`
- `landing.custom_domain`
- `landing.remove_branding`
- `landing.analytics`
- `crm.enabled`
- `crm.max_contacts`
- `crm.import_export`
- `crm.pipeline`
- `media.enabled`
- `media.max_storage_mb`
- `media.max_file_size_mb`
- `domain.enabled`
- `domain.max_custom_domains`
- `domain.ssl_auto_provision`
- `whatsapp.enabled`
- `whatsapp.max_messages_per_month`
- `pos.enabled`
- `pos.max_products`
- `membership.enabled`
- `membership.max_members`
- `automation.enabled`
- `automation.max_runs_per_month`

## Integration Notes

### Landing Page

- Create page checks `landing.enabled`.
- Create page checks `landing.max_pages`.
- Section create/reorder can check `landing.max_sections_per_page`.
- Analytics endpoint checks `landing.analytics`.
- Remove branding checks `landing.remove_branding`.

Billing MVP mempertahankan prefix existing `landing.*`, sehingga Landing guard
tidak membutuhkan compatibility alias untuk feature key.

### Custom Domain

- Organization domain create checks `domain.enabled`.
- Verified custom domain limit checks `domain.max_custom_domains`.
- Landing domain binding checks `landing.custom_domain`.

### User Invitation

- Invite user checks `users.invite_user`.
- Active member count checks `users.max_users`.
- Custom role management checks `roles.custom_roles`.

### Payment

- MVP supports manual provider.
- Xendit/Midtrans adapter remains future and belongs in `internal/platform`.
- Billing service owns business transition after provider verification.

## Future Extension

- Add-on.
- Coupon.
- Tax.
- Recurring payment.
- Prorated billing.
- Usage-based billing.
- Metered billing event ledger.
- Dunning notification.
- Payment gateway integration.
- Enterprise custom contract.
- White label.
- Reseller billing.
