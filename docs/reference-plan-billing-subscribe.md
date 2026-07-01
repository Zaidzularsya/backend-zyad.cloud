# MASTER PROMPT — Generate Development Documentation for Plan, Billing, Subscription, Entitlement & Quota Module

Kamu adalah senior software architect dan technical writer yang membantu saya menyusun dokumentasi development untuk platform multi-tenant SaaS.

Saya sedang membangun platform bernama `zyad.cloud` dengan backend Go, PostgreSQL, dan arsitektur modular. Platform ini mendukung multi-tenant organization, landing page management, CRM, POS, membership, media manager, permission/RBAC, custom domain, dan modul-modul SaaS lain.

Saya ingin membuat dokumentasi matang untuk modul:

* Plan Management
* Billing Management
* Subscription Management
* Feature Entitlement
* Quota / Usage Limit
* Invoice
* Payment
* Migration & Seed
* Development Traceability

Dokumentasi harus bisa langsung dipakai sebagai referensi development oleh AI coding agent maupun developer manusia.

## Target Output Dokumen

Buatkan 3 dokumen utama berikut:

1. `docs/billing-plan-concept-reference.md`
2. `docs/billing-plan-development-tasks.md`
3. `docs/billing-plan-development-traceability.md`

Selain itu, sertakan juga rekomendasi file migration dan seed yang perlu dibuat.

---

# 1. Document: `docs/billing-plan-concept-reference.md`

Buat dokumen konsep dan reference yang menjelaskan keseluruhan desain modul secara matang.

Dokumen harus berisi:

## 1.1 Overview

Jelaskan bahwa modul ini bertujuan untuk mengatur paket SaaS berbasis organization/tenant.

Core concept:

* Organization adalah billing account.
* Plan adalah paket bisnis.
* Feature adalah kemampuan/fungsi platform.
* Entitlement adalah izin fitur yang dimiliki organization.
* Quota adalah batas penggunaan fitur.
* Subscription adalah kontrak aktif organization terhadap plan.
* Invoice adalah tagihan.
* Payment adalah transaksi pembayaran.
* Usage counter adalah pencatatan penggunaan terhadap limit.

Tekankan bahwa billing logic tidak boleh hardcode berdasarkan nama plan seperti:

```go
if plan == "starter" {}
```

Melainkan harus berbasis entitlement dan quota.

## 1.2 Business Goals

Jelaskan tujuan bisnis:

* Mendukung monetisasi SaaS.
* Mendukung plan Free, Trial, Starter, Growth, Business, Enterprise.
* Mendukung fitur modular berdasarkan paket.
* Mendukung upgrade/downgrade.
* Mendukung add-on di masa depan.
* Mendukung billing, lalu payment gateway.
* Mendukung penggunaan tenant-level entitlement agar fleksibel.

## 1.3 Scope

Masukkan scope awal MVP:

* CRUD plans
* CRUD features
* Plan entitlements
* Organization subscription
* Organization entitlement snapshot
* Usage counters
* invoice
* payment record
* Subscription status check
* Feature access check
* Quota check
* Migration
* Seed default plans/features/entitlements

Out of scope untuk MVP:

* Full recurring payment otomatis
* Proration kompleks
* Usage-based overage billing
* Tax engine kompleks
* Payment gateway production integration
* Coupon/discount advanced

Namun desain database harus tetap siap dikembangkan ke arah itu.

## 1.4 Core Domain Model

Jelaskan model utama:

### Plan

Plan adalah template paket bisnis. Contoh:

* free
* starter
* growth
* business
* enterprise

Plan tidak boleh langsung menjadi satu-satunya sumber runtime access control.

### Feature

Feature adalah definisi kemampuan platform. Contoh:

* landing_page.enabled
* landing_page.max_pages
* landing_page.custom_domain
* crm.enabled
* crm.max_contacts
* users.max_users
* media.max_storage_mb
* whatsapp.enabled
* whatsapp.max_messages_per_month

### Plan Entitlement

Plan entitlement adalah default entitlement dari sebuah plan.

Contoh:

* growth memiliki landing_page.max_pages = 10
* growth memiliki landing_page.custom_domain = true
* growth memiliki users.max_users = 10

### Organization Entitlement

Organization entitlement adalah snapshot entitlement aktif milik tenant.

Jelaskan kenapa perlu snapshot:

* Plan adalah template.
* Organization entitlement adalah runtime source of truth.
* Jika harga/paket berubah, tenant lama tidak otomatis rusak.
* Bisa mendukung custom override untuk enterprise.
* Runtime check lebih ringan.

### Subscription

Subscription menyatakan organization sedang memakai plan tertentu.

Status subscription minimal:

* trialing
* active
* past_due
* grace_period
* suspended
* canceled
* expired

### Invoice

Invoice adalah tagihan atas subscription atau add-on.

Status invoice:

* draft
* open
* paid
* void
* expired
* failed

### Payment

Payment adalah pencatatan pembayaran terhadap invoice.

Provider awal:

* manual
* xendit
* midtrans

Status payment:

* pending
* paid
* failed
* expired
* refunded

### Usage Counter

Usage counter mencatat penggunaan fitur yang memiliki limit.

Contoh:

* jumlah landing page
* jumlah user
* jumlah contact CRM
* storage media
* jumlah pesan WhatsApp per bulan

## 1.5 Access Control Flow

Jelaskan urutan validasi saat user menjalankan aksi:

1. Resolve organization dari token, host, atau header.
2. Cek subscription organization.
3. Cek entitlement organization.
4. Cek quota/usage.
5. Cek role permission user.
6. Jalankan action.
7. Update usage counter jika perlu.

Tekankan perbedaan:

Plan entitlement menjawab:

> Apakah organization ini punya akses fitur tersebut?

Role permission menjawab:

> Apakah user ini boleh melakukan aksi tersebut dalam organization?

Keduanya tidak boleh dicampur.

## 1.6 Example Feature Keys

Buat daftar feature key awal:

### Core Users

* users.max_users
* users.invite_user
* roles.custom_roles

### Landing Page

* landing_page.enabled
* landing_page.max_pages
* landing_page.max_sections_per_page
* landing_page.custom_domain
* landing_page.remove_branding
* landing_page.analytics

### CRM

* crm.enabled
* crm.max_contacts
* crm.import_export
* crm.pipeline
* crm.lead_form

### Media

* media.enabled
* media.max_storage_mb
* media.max_file_size_mb

### Domain

* domain.enabled
* domain.max_custom_domains
* domain.ssl_auto_provision

### WhatsApp

* whatsapp.enabled
* whatsapp.max_messages_per_month
* whatsapp.template_message

### POS

* pos.enabled
* pos.max_products
* pos.max_transactions_per_month

### Membership

* membership.enabled
* membership.max_members

### Automation

* automation.enabled
* automation.max_runs_per_month

## 1.7 Database Design Reference

Buat desain tabel berikut secara detail:

* billing_plans
* billing_plan_prices
* billing_features
* billing_plan_entitlements
* billing_subscriptions
* billing_organization_entitlements
* billing_invoices
* billing_invoice_items
* billing_payments
* billing_usage_counters
* billing_subscription_events
* billing_payment_events

Untuk setiap tabel, jelaskan:

* purpose
* key columns
* relationship
* indexes
* unique constraints
* check constraints
* soft delete jika perlu
* audit fields

Gunakan PostgreSQL.

Semua ID sebaiknya UUID.

Gunakan naming konsisten dengan snake_case.

Gunakan `organization_id` sebagai tenant ownership key.

## 1.8 Suggested Schema Detail

Gunakan desain berikut sebagai baseline dan boleh ditingkatkan:

### billing_plans

* id uuid primary key
* code varchar unique not null
* name varchar not null
* description text
* plan_type varchar not null check in free, paid, enterprise
* is_public boolean default true
* is_active boolean default true
* sort_order integer default 0
* created_at timestamp
* updated_at timestamp
* deleted_at timestamp nullable

### billing_plan_prices

* id uuid primary key
* plan_id uuid not null references billing_plans(id)
* billing_interval varchar not null check in monthly, yearly, one_time
* currency varchar not null default IDR
* amount numeric(14,2) not null default 0
* is_active boolean default true
* created_at timestamp
* updated_at timestamp
* deleted_at timestamp nullable

Unique:

* plan_id + billing_interval + currency where deleted_at is null

### billing_features

* id uuid primary key
* feature_key varchar unique not null
* module varchar not null
* name varchar not null
* description text
* value_type varchar not null check in boolean, integer, string, decimal
* unit varchar nullable
* is_active boolean default true
* created_at timestamp
* updated_at timestamp

### billing_plan_entitlements

* id uuid primary key
* plan_id uuid not null
* feature_id uuid not null
* value_bool boolean nullable
* value_int integer nullable
* value_decimal numeric(14,2) nullable
* value_string text nullable
* created_at timestamp
* updated_at timestamp

Unique:

* plan_id + feature_id

Validasi:

* hanya satu value column yang boleh digunakan sesuai feature.value_type

### billing_subscriptions

* id uuid primary key
* organization_id uuid not null
* plan_id uuid not null
* status varchar not null check in trialing, active, past_due, grace_period, suspended, canceled, expired
* billing_interval varchar not null check in monthly, yearly, custom
* current_period_start timestamp nullable
* current_period_end timestamp nullable
* trial_start timestamp nullable
* trial_end timestamp nullable
* cancel_at_period_end boolean default false
* canceled_at timestamp nullable
* suspended_at timestamp nullable
* created_at timestamp
* updated_at timestamp

Constraint:

* satu organization hanya boleh punya satu active/trialing/grace_period subscription.

### billing_organization_entitlements

* id uuid primary key
* organization_id uuid not null
* subscription_id uuid nullable
* feature_id uuid not null
* source varchar not null check in plan, override, addon, system
* value_bool boolean nullable
* value_int integer nullable
* value_decimal numeric(14,2) nullable
* value_string text nullable
* effective_from timestamp not null
* effective_until timestamp nullable
* created_at timestamp
* updated_at timestamp

Unique:

* organization_id + feature_id + source where effective_until is null

### billing_invoices

* id uuid primary key
* organization_id uuid not null
* subscription_id uuid nullable
* invoice_number varchar unique not null
* status varchar not null check in draft, open, paid, void, expired, failed
* currency varchar not null default IDR
* subtotal_amount numeric(14,2) default 0
* discount_amount numeric(14,2) default 0
* tax_amount numeric(14,2) default 0
* total_amount numeric(14,2) default 0
* due_date timestamp nullable
* paid_at timestamp nullable
* created_at timestamp
* updated_at timestamp

### billing_invoice_items

* id uuid primary key
* invoice_id uuid not null
* item_type varchar not null check in subscription, addon, adjustment, tax, discount
* description text not null
* quantity numeric(14,2) default 1
* unit_amount numeric(14,2) default 0
* total_amount numeric(14,2) default 0
* metadata jsonb default '{}'::jsonb
* created_at timestamp
* updated_at timestamp

### billing_payments

* id uuid primary key
* invoice_id uuid not null
* organization_id uuid not null
* provider varchar not null check in manual, xendit, midtrans
* provider_reference varchar nullable
* payment_method varchar nullable
* status varchar not null check in pending, paid, failed, expired, refunded
* amount numeric(14,2) not null
* currency varchar default IDR
* paid_at timestamp nullable
* raw_payload jsonb default '{}'::jsonb
* created_at timestamp
* updated_at timestamp

### billing_usage_counters

* id uuid primary key
* organization_id uuid not null
* feature_id uuid not null
* period_start timestamp nullable
* period_end timestamp nullable
* used_value numeric(14,2) default 0
* limit_value numeric(14,2) nullable
* reset_strategy varchar check in never, monthly, yearly, custom
* created_at timestamp
* updated_at timestamp

Unique:

* organization_id + feature_id + period_start + period_end

### billing_subscription_events

* id uuid primary key
* subscription_id uuid not null
* organization_id uuid not null
* event_type varchar not null
* old_status varchar nullable
* new_status varchar nullable
* metadata jsonb default '{}'::jsonb
* created_at timestamp

### billing_payment_events

* id uuid primary key
* payment_id uuid nullable
* invoice_id uuid nullable
* provider varchar not null
* event_type varchar not null
* provider_event_id varchar nullable
* payload jsonb default '{}'::jsonb
* processed_at timestamp nullable
* created_at timestamp

## 1.9 Runtime Service Rules

Jelaskan service yang dibutuhkan:

* PlanService
* FeatureService
* EntitlementService
* SubscriptionService
* InvoiceService
* PaymentService
* UsageService
* BillingGuardService

Fungsi penting:

* GetActiveSubscription(organizationID)
* EnsureSubscriptionUsable(organizationID)
* HasFeature(organizationID, featureKey)
* GetLimit(organizationID, featureKey)
* CheckQuota(organizationID, featureKey, requestedIncrement)
* IncrementUsage(organizationID, featureKey, amount)
* SyncEntitlementsFromPlan(organizationID, subscriptionID, planID)
* CreateInvoiceForSubscription(subscriptionID)
* MarkInvoicePaid(invoiceID)
* ActivateSubscription(subscriptionID)
* SuspendSubscription(subscriptionID)

## 1.10 API Reference Draft

Buat draft API endpoint untuk platform admin:

* GET /api/v1/platform/billing/plans

* POST /api/v1/platform/billing/plans

* GET /api/v1/platform/billing/plans/{id}

* PATCH /api/v1/platform/billing/plans/{id}

* DELETE /api/v1/platform/billing/plans/{id}

* GET /api/v1/platform/billing/features

* POST /api/v1/platform/billing/features

* PATCH /api/v1/platform/billing/features/{id}

* GET /api/v1/platform/billing/plans/{id}/entitlements

* PUT /api/v1/platform/billing/plans/{id}/entitlements

* GET /api/v1/platform/billing/subscriptions

* POST /api/v1/platform/billing/subscriptions

* PATCH /api/v1/platform/billing/subscriptions/{id}

* GET /api/v1/platform/billing/invoices

* POST /api/v1/platform/billing/invoices

* POST /api/v1/platform/billing/invoices/{id}/mark-paid

Tenant dashboard API:

* GET /api/v1/app/billing/current-plan
* GET /api/v1/app/billing/usage
* GET /api/v1/app/billing/invoices
* POST /api/v1/app/billing/upgrade
* POST /api/v1/app/billing/cancel

Internal guard API/service:

* GET /api/v1/internal/billing/organizations/{organization_id}/entitlements
* GET /api/v1/internal/billing/organizations/{organization_id}/features/{feature_key}/check

## 1.11 Error Response Standard

Buat standard error code:

* SUBSCRIPTION_NOT_FOUND
* SUBSCRIPTION_INACTIVE
* SUBSCRIPTION_SUSPENDED
* FEATURE_NOT_ENABLED
* QUOTA_EXCEEDED
* PLAN_NOT_FOUND
* FEATURE_NOT_FOUND
* INVOICE_NOT_FOUND
* PAYMENT_ALREADY_PROCESSED
* INVALID_BILLING_STATUS

Contoh response:

```json
{
  "code": "QUOTA_EXCEEDED",
  "message": "Limit landing page pada paket Anda sudah tercapai.",
  "feature": "landing_page.max_pages",
  "used": 10,
  "limit": 10,
  "upgrade_available": true
}
```

## 1.12 Upgrade & Downgrade Policy

Jelaskan:

Upgrade:

* entitlement baru bisa langsung aktif
* invoice dibuat
* untuk MVP tidak perlu prorate
* setelah payment paid, subscription active dengan plan baru

Downgrade:

* aktif next billing cycle
* data tidak dihapus
* item berlebih masuk read-only/locked
* tenant harus memilih data aktif jika limit baru lebih kecil

## 1.13 Grace Period & Suspension

Jelaskan lifecycle:

* invoice due date lewat
* subscription jadi past_due
* masih bisa akses normal beberapa hari
* masuk grace_period dengan warning
* setelah grace habis menjadi suspended
* saat suspended dashboard hanya bisa akses billing
* public landing page bisa diputuskan tetap hidup sementara atau dinonaktifkan sesuai business rule

## 1.14 Security & Audit

Jelaskan:

* Platform admin hanya role tertentu yang boleh manage plan/billing.
* Tenant hanya boleh melihat invoice/subscription organization-nya sendiri.
* Payment webhook harus idempotent.
* Provider event id harus unique jika tersedia.
* Jangan percaya amount dari client.
* Payment status harus diverifikasi dari provider/webhook.
* Semua perubahan subscription harus dicatat di billing_subscription_events.
* Semua event payment gateway harus dicatat di billing_payment_events.

## 1.15 Future Extension

Sertakan extension:

* add-on
* coupon
* tax
* recurring payment
* prorated billing
* usage-based billing
* metered billing
* dunning notification
* payment gateway integration
* enterprise custom contract
* white label
* reseller billing

---

# 2. Document: `docs/billing-plan-development-tasks.md`

Buat dokumen task development yang actionable dan bisa dieksekusi bertahap.

Gunakan format:

* Phase
* Task ID
* Task name
* Description
* Files impacted
* Implementation notes
* Acceptance criteria
* Dependencies
* Test scenario

## Required Phases

### Phase 0 — Review Existing Architecture

Task:

* Review existing organization table.
* Review existing permission/RBAC module.
* Review existing landing page module.
* Review existing OpenAPI pattern.
* Review migration folder convention.
* Review seed folder convention.
* Review repository/service/handler pattern.
* Review middleware organization resolver.
* Review current docs.

Acceptance:

* Developer memahami struktur existing.
* Tidak membuat module yang bentrok dengan naming existing.
* Semua naming mengikuti pattern repo.

### Phase 1 — Database Migration

Buat task untuk membuat migration file:

* create billing_plans
* create billing_plan_prices
* create billing_features
* create billing_plan_entitlements
* create billing_subscriptions
* create billing_organization_entitlements
* create billing_invoices
* create billing_invoice_items
* create billing_payments
* create billing_usage_counters
* create billing_subscription_events
* create billing_payment_events

Sertakan catatan:

* semua table pakai UUID
* created_at dan updated_at
* deleted_at untuk master data tertentu
* check constraints untuk enum-like fields
* indexes untuk organization_id, plan_id, feature_key, status
* unique constraints penting
* down migration harus drop table dengan urutan FK yang benar

### Phase 2 — Seed Data

Buat task seed:

* seed default features
* seed default plans
* seed default plan prices
* seed default plan entitlements
* optional seed trial subscription untuk organization demo jika tersedia

Default plans:

* free
* starter
* growth
* business
* enterprise

Default feature keys:

* users.max_users
* roles.custom_roles
* landing_page.enabled
* landing_page.max_pages
* landing_page.max_sections_per_page
* landing_page.custom_domain
* landing_page.remove_branding
* landing_page.analytics
* crm.enabled
* crm.max_contacts
* crm.import_export
* crm.pipeline
* media.enabled
* media.max_storage_mb
* media.max_file_size_mb
* domain.enabled
* domain.max_custom_domains
* domain.ssl_auto_provision
* whatsapp.enabled
* whatsapp.max_messages_per_month
* pos.enabled
* pos.max_products
* membership.enabled
* membership.max_members
* automation.enabled
* automation.max_runs_per_month

Default entitlement sample:

Free:

* users.max_users = 1
* landing_page.enabled = true
* landing_page.max_pages = 1
* landing_page.custom_domain = false
* crm.enabled = false
* media.max_storage_mb = 100

Starter:

* users.max_users = 3
* landing_page.enabled = true
* landing_page.max_pages = 3
* landing_page.custom_domain = false
* crm.enabled = true
* crm.max_contacts = 500
* media.max_storage_mb = 1000

Growth:

* users.max_users = 10
* landing_page.max_pages = 10
* landing_page.custom_domain = true
* domain.max_custom_domains = 1
* crm.max_contacts = 5000
* media.max_storage_mb = 5000
* whatsapp.enabled = true
* whatsapp.max_messages_per_month = 1000

Business:

* users.max_users = 25
* landing_page.max_pages = 50
* domain.max_custom_domains = 5
* crm.max_contacts = 25000
* pos.enabled = true
* membership.enabled = true
* automation.enabled = true

Enterprise:

* customizable
* values can be high or null depending design

Seed harus idempotent menggunakan upsert.

### Phase 3 — Domain Layer

Task:

* create domain models
* create DTOs
* create repository interfaces
* create service interfaces
* create error constants

Expected folder:

```text
internal/modules/billing/
  domain/
  dto/
  repository/
  service/
  handler/
```

Jika repo existing punya pattern berbeda, ikuti pattern existing.

### Phase 4 — Repository Implementation

Task:

* implement plan repository
* implement feature repository
* implement entitlement repository
* implement subscription repository
* implement invoice repository
* implement payment repository
* implement usage repository

Acceptance:

* Semua query pakai context.
* Semua query filter deleted_at untuk soft deleted data.
* Semua query tenant-scoped untuk organization data.
* Repository tidak berisi business logic kompleks.

### Phase 5 — Service Implementation

Task:

* PlanService CRUD
* FeatureService CRUD
* EntitlementService
* SubscriptionService
* InvoiceService
* PaymentService
* UsageService
* BillingGuardService

Business methods:

* SyncEntitlementsFromPlan
* EnsureSubscriptionUsable
* HasFeature
* GetEntitlementValue
* CheckQuota
* IncrementUsage
* CreateInvoiceForSubscription
* MarkInvoicePaid
* RecordPaymentEvent

Acceptance:

* Entitlement snapshot dibuat saat subscription dibuat/diaktifkan.
* Subscription status dicek sebelum feature access.
* Quota check mengembalikan error standar.
* Payment mark paid idempotent.
* Semua status transition tercatat di subscription events.

### Phase 6 — HTTP Handler / API

Task:

Platform admin endpoints:

* manage plans
* manage features
* manage plan entitlements
* manage organization subscriptions
* manage invoices
* mark invoice paid

Tenant endpoints:

* current plan
* usage
* invoices
* upgrade request
* cancel request

Acceptance:

* Platform API butuh platform admin permission.
* Tenant API harus scoped by current organization.
* Tidak boleh tenant melihat invoice organization lain.
* Response format mengikuti standard repo existing.
* Error format konsisten.

### Phase 7 — Middleware / Guard Integration

Task:

* buat helper/middleware untuk require feature
* buat helper/middleware untuk require quota
* integrasikan minimal ke landing page create
* integrasikan minimal ke custom domain create
* integrasikan minimal ke user invitation jika ada

Acceptance:

* Create landing page gagal jika subscription suspended.
* Create landing page gagal jika feature landing_page.enabled false.
* Create landing page gagal jika quota max_pages habis.
* Create custom domain gagal jika custom domain entitlement false.
* Error code jelas dan bisa dipakai frontend.

### Phase 8 — OpenAPI Documentation

Task:

* update api/openapi.yaml
* tambahkan schemas billing
* tambahkan endpoints platform billing
* tambahkan endpoints tenant billing
* tambahkan error response schemas
* tambahkan examples

Acceptance:

* OpenAPI valid.
* Endpoint punya request/response example.
* Error cases terdokumentasi.
* Tidak merusak existing OpenAPI.

### Phase 9 — Testing

Task:

Unit test:

* plan service
* entitlement service
* subscription service
* usage service
* invoice/payment service

Integration test:

* migration up/down
* seed idempotent
* create subscription
* sync entitlement
* quota check
* mark invoice paid
* suspended subscription blocks feature

Acceptance:

* Tests pass.
* Seed bisa dijalankan berulang tanpa duplicate.
* Migration down membersihkan table dengan benar.
* Critical flow subscription aktif berjalan.

### Phase 10 — Documentation Finalization

Task:

* finalisasi concept reference
* finalisasi task development
* finalisasi traceability index
* tambahkan checklist implementation
* tambahkan known limitations
* tambahkan next phase recommendation

Acceptance:

* Dokumentasi bisa dipakai sebagai development guide.
* Task punya status tracking.
* Traceability menghubungkan business requirement ke DB, API, service, test.

---

# 3. Document: `docs/billing-plan-development-traceability.md`

Buat dokumen traceability index yang menghubungkan requirement, database, service, API, migration, seed, dan test.

Gunakan format tabel.

Kolom minimal:

* Trace ID
* Requirement
* Business Reason
* Database Tables
* Migration File
* Seed File
* Domain/Service
* API Endpoint
* Test Coverage
* Status
* Notes

Contoh traceability:

| Trace ID     | Requirement                               | Business Reason                                  | Database Tables                                           | Migration File                                   | Seed File                | Domain/Service                           | API Endpoint                                   | Test Coverage                    | Status  | Notes       |
| ------------ | ----------------------------------------- | ------------------------------------------------ | --------------------------------------------------------- | ------------------------------------------------ | ------------------------ | ---------------------------------------- | ---------------------------------------------- | -------------------------------- | ------- | ----------- |
| BILL-REQ-001 | Platform admin can create plan            | Plan menjadi paket SaaS                          | billing_plans                                             | 0000xx_create_billing_plans.up.sql               | seed_billing_plans       | PlanService                              | POST /platform/billing/plans                   | TestCreatePlan                   | Planned | -           |
| BILL-REQ-002 | Plan has prices monthly/yearly            | Mendukung billing interval                       | billing_plan_prices                                       | 0000xx_create_billing_plan_prices.up.sql         | seed_billing_plan_prices | PlanPriceService                         | GET /platform/billing/plans                    | TestPlanPrice                    | Planned | -           |
| BILL-REQ-003 | Feature catalog is configurable           | Fitur tidak hardcoded                            | billing_features                                          | 0000xx_create_billing_features.up.sql            | seed_billing_features    | FeatureService                           | GET /platform/billing/features                 | TestFeatureList                  | Planned | -           |
| BILL-REQ-004 | Plan has entitlement values               | Paket menentukan akses fitur                     | billing_plan_entitlements                                 | 0000xx_create_billing_plan_entitlements.up.sql   | seed_plan_entitlements   | EntitlementService                       | PUT /platform/billing/plans/{id}/entitlements  | TestPlanEntitlement              | Planned | -           |
| BILL-REQ-005 | Organization has active subscription      | Tenant harus punya status billing                | billing_subscriptions                                     | 0000xx_create_billing_subscriptions.up.sql       | optional demo seed       | SubscriptionService                      | GET /app/billing/current-plan                  | TestActiveSubscription           | Planned | -           |
| BILL-REQ-006 | Organization entitlement snapshot exists  | Runtime access tidak bergantung langsung ke plan | billing_organization_entitlements                         | 0000xx_create_organization_entitlements.up.sql   | generated from plan      | EntitlementService                       | internal service                               | TestSyncEntitlementFromPlan      | Planned | -           |
| BILL-REQ-007 | Feature access can be checked             | Modul bisa dikunci per paket                     | billing_organization_entitlements                         | same                                             | same                     | BillingGuardService                      | internal service                               | TestHasFeature                   | Planned | -           |
| BILL-REQ-008 | Quota can be checked                      | Limit penggunaan paket                           | billing_usage_counters                                    | 0000xx_create_billing_usage_counters.up.sql      | usage seed optional      | UsageService                             | GET /app/billing/usage                         | TestQuotaExceeded                | Planned | -           |
| BILL-REQ-009 | Invoice can be generated                  | Tenant dapat tagihan                             | billing_invoices, billing_invoice_items                   | 0000xx_create_billing_invoices.up.sql            | none                     | InvoiceService                           | POST /platform/billing/invoices                | TestCreateInvoice                | Planned | -           |
| BILL-REQ-010 | Payment can be recorded manually          | MVP belum wajib gateway                          | billing_payments                                          | 0000xx_create_billing_payments.up.sql            | none                     | PaymentService                           | POST /platform/billing/invoices/{id}/mark-paid | TestMarkInvoicePaid              | Planned | -           |
| BILL-REQ-011 | Subscription status changes are audited   | Billing harus traceable                          | billing_subscription_events                               | 0000xx_create_billing_subscription_events.up.sql | none                     | SubscriptionService                      | internal                                       | TestSubscriptionEventCreated     | Planned | -           |
| BILL-REQ-012 | Payment provider events are idempotent    | Webhook aman dari duplicate                      | billing_payment_events                                    | 0000xx_create_billing_payment_events.up.sql      | none                     | PaymentService                           | webhook future                                 | TestPaymentEventIdempotency      | Planned | Future      |
| BILL-REQ-013 | Landing page create respects quota        | Mencegah abuse free/starter plan                 | billing_usage_counters, billing_organization_entitlements | same                                             | seed features            | BillingGuardService + LandingPageService | POST landing page endpoint existing            | TestCreateLandingPageQuota       | Planned | Integration |
| BILL-REQ-014 | Custom domain respects entitlement        | Custom domain hanya paket tertentu               | billing_organization_entitlements                         | same                                             | seed features            | BillingGuardService + DomainService      | POST domain endpoint existing                  | TestCustomDomainEntitlement      | Planned | Integration |
| BILL-REQ-015 | Suspended tenant cannot use paid features | Revenue protection                               | billing_subscriptions                                     | same                                             | none                     | BillingGuardService                      | all protected endpoints                        | TestSuspendedSubscriptionBlocked | Planned | Critical    |

Tambahkan juga section:

## Status Legend

* Planned
* In Progress
* Done
* Blocked
* Deferred

## Development Checklist

Checklist:

* [ ] Existing architecture reviewed
* [ ] Migration created
* [ ] Migration down tested
* [ ] Seed created
* [ ] Seed idempotent tested
* [ ] Domain models created
* [ ] Repositories implemented
* [ ] Services implemented
* [ ] Handlers implemented
* [ ] Guard integrated
* [ ] OpenAPI updated
* [ ] Unit tests added
* [ ] Integration tests added
* [ ] Documentation finalized

---

# 4. Migration & Seed Requirements

Buat rekomendasi nama file migration.

Gunakan numbering mengikuti repo existing. Jika belum tahu nomor terakhir, gunakan placeholder:

```text
0000xx_create_billing_plans.up.sql
0000xx_create_billing_plans.down.sql
0000xx_create_billing_plan_prices.up.sql
0000xx_create_billing_plan_prices.down.sql
0000xx_create_billing_features.up.sql
0000xx_create_billing_features.down.sql
0000xx_create_billing_plan_entitlements.up.sql
0000xx_create_billing_plan_entitlements.down.sql
0000xx_create_billing_subscriptions.up.sql
0000xx_create_billing_subscriptions.down.sql
0000xx_create_billing_organization_entitlements.up.sql
0000xx_create_billing_organization_entitlements.down.sql
0000xx_create_billing_invoices.up.sql
0000xx_create_billing_invoices.down.sql
0000xx_create_billing_invoice_items.up.sql
0000xx_create_billing_invoice_items.down.sql
0000xx_create_billing_payments.up.sql
0000xx_create_billing_payments.down.sql
0000xx_create_billing_usage_counters.up.sql
0000xx_create_billing_usage_counters.down.sql
0000xx_create_billing_subscription_events.up.sql
0000xx_create_billing_subscription_events.down.sql
0000xx_create_billing_payment_events.up.sql
0000xx_create_billing_payment_events.down.sql
```

Namun jika repo lebih suka satu migration besar untuk satu module, boleh gunakan:

```text
0000xx_create_billing_module.up.sql
0000xx_create_billing_module.down.sql
```

Berikan rekomendasi mana yang lebih baik.

Untuk production-grade, prefer beberapa migration yang terurut dan mudah rollback.

## Seed File Recommendation

Gunakan:

```text
0000xx_seed_billing_features.up.sql
0000xx_seed_billing_features.down.sql
0000xx_seed_billing_plans.up.sql
0000xx_seed_billing_plans.down.sql
0000xx_seed_billing_plan_prices.up.sql
0000xx_seed_billing_plan_prices.down.sql
0000xx_seed_billing_plan_entitlements.up.sql
0000xx_seed_billing_plan_entitlements.down.sql
```

Seed harus idempotent menggunakan:

```sql
INSERT ... ON CONFLICT ... DO UPDATE
```

atau mengikuti pattern seed existing pada repo.

## Required Migration Quality

Migration harus:

* Bisa dijalankan dari kosong.
* Bisa rollback dengan benar.
* Tidak merusak table existing.
* FK jelas.
* Index jelas.
* Check constraint jelas.
* Unique constraint jelas.
* Tidak menggunakan enum PostgreSQL native kecuali repo memang sudah menggunakan enum.
* Lebih baik gunakan varchar + check constraint agar mudah diubah.
* Semua amount gunakan numeric, bukan float.
* Semua metadata gunakan jsonb.
* Semua timestamp konsisten dengan repo.
* Gunakan gen_random_uuid() jika extension pgcrypto sudah tersedia; jika belum, sertakan requirement extension atau gunakan UUID dari aplikasi.

## Required Seed Quality

Seed harus:

* Idempotent.
* Tidak membuat duplicate.
* Aman dijalankan ulang.
* Tidak menghapus data custom user.
* Tidak override enterprise custom tenant entitlement.
* Default plans harus inactive/active sesuai kebutuhan.
* Feature key harus stabil dan tidak mudah berubah.

---

# 5. Implementation Guidance

Dalam dokumen, tambahkan arahan implementasi berikut:

## 5.1 Do Not Mix Entitlement and RBAC

RBAC:

* user-level permission
* action-level permission
* contoh: user boleh create landing page

Entitlement:

* organization-level feature access
* package-level limitation
* contoh: organization boleh custom domain

Flow harus:

1. Subscription usable?
2. Feature entitled?
3. Quota available?
4. User permission allowed?

## 5.2 Use Organization Entitlement Snapshot

Saat subscription dibuat atau plan berubah:

* ambil semua plan_entitlements
* copy ke billing_organization_entitlements
* source = plan
* effective_from = now
* effective_until = null

Jika plan berubah, jangan langsung mengubah historical entitlement tanpa strategy.

## 5.3 Quota Strategy

Ada 2 tipe quota:

1. Snapshot count quota

   * users.max_users
   * landing_page.max_pages
   * domain.max_custom_domains

2. Periodic usage quota

   * whatsapp.max_messages_per_month
   * automation.max_runs_per_month
   * pos.max_transactions_per_month

Jelaskan perbedaan cara menghitungnya.

## 5.4 Usage Counter Strategy

Untuk count-based resources, bisa dihitung realtime dari table utama atau disimpan di usage counter.

MVP:

* count landing page dari landing_pages table
* count custom domain dari organization_domains table
* count users dari organization_users table

Tetapi billing_usage_counters tetap disiapkan untuk fitur periodic seperti WhatsApp, automation, dan transaction.

## 5.5 Idempotency

Payment dan webhook harus idempotent.

Mark invoice paid tidak boleh menggandakan event.

Jika invoice sudah paid, request mark-paid berikutnya harus aman.

## 5.6 Frontend Implication

Tenant dashboard harus bisa menampilkan:

* current plan
* subscription status
* next billing date
* usage list
* invoice list
* upgrade CTA

Platform dashboard harus bisa manage:

* plan
* feature
* entitlement
* subscription
* invoice
* payment

---

# 6. Expected Final Answer Format

Hasil akhir harus berisi:

1. File tree dokumen yang dibuat.
2. Isi lengkap untuk:

   * `billing-plan-concept-reference.md`
   * `billing-plan-development-tasks.md`
   * `billing-plan-development-traceability.md`
3. Rekomendasi migration files.
4. Rekomendasi seed files.
5. Development order yang paling aman.
6. Risiko dan mitigasi.
7. Checklist implementasi.
8. Catatan integrasi dengan landing page, custom domain, organization, dan permission existing.

Gunakan bahasa Indonesia teknis yang jelas, rapi, dan matang.

Jangan langsung coding fitur. Fokus pada dokumen konsep, reference, task development, dan traceability index yang akan menjadi dasar implementation.
