# Billing Plan Development Traceability

> **Catatan riwayat (refactor domain-split)**: Migration, permission slug, dan API path yang dirujuk di
> dokumen ini (`billing_plans`, `billing_subscriptions`, `platform.billing.plan.*`, dst,
> `/platform/billing/plans`) sudah digantikan hasil refactor domain-split. Traceability final ada di
> [product-subscription-billing-traceability.md](product-subscription-billing-traceability.md). Dokumen ini
> dipertahankan sebagai riwayat pelacakan requirement→implementasi versi awal.

Dokumen ini menghubungkan requirement Plan, Billing, Subscription, Entitlement,
Quota, Invoice, dan Payment ke task, database, service, API, migration, seed,
test, dan status implementasi.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `blocked`: tertahan dependency/keputusan.
- `deferred`: sengaja ditunda.

## Source Documents

| Source | Purpose |
| --- | --- |
| `README.md` | Konteks platform multi-tenant dan module billing. |
| `AGENTS.md` | Instruksi layer, migration, dan gaya kerja repo. |
| `docs/reference-plan-billing-subscribe.md` | Source requirement awal dari user. |
| `docs/billing-plan-concept-reference.md` | Konsep dan desain domain billing. |
| `docs/billing-plan-development-tasks.md` | Breakdown task development. |
| `docs/reference-multi-tenant.md` | Organization sebagai tenant canonical. |
| `docs/multi-tenant-traceability-index.md` | Status entitlement/usage existing. |
| `docs/migration-guide.md` | Aturan migration. |

## Module Mapping

| Capability | Path/Table | Responsibility | Status |
| --- | --- | --- | --- |
| Billing module | `internal/modules/billing` | Plan, subscription, invoice, payment | done MVP |
| Organization module | `internal/modules/organization` | Organization, entitlement runtime, usage runtime | done foundation |
| Permission core | `internal/core/permission` | RBAC/action authorization | done foundation |
| Tenant middleware | `internal/core/middleware` | Active organization context | done foundation |
| Landing module | `internal/modules/landing` | Landing feature consumer | integrated with billing guards |
| Payment platform adapter | `internal/platform/xendit` | Technical gateway adapter | placeholder |

## Existing State

| Existing Capability | Location | Status/Gap |
| --- | --- | --- |
| Organization account | `organizations` | done |
| Organization entitlement runtime | `organization_entitlements` | done; billing must sync into this table |
| Organization usage runtime | `organization_usage_counters` | done; billing must reuse |
| Billing module | `internal/modules/billing` | done MVP for plan, subscription, invoice, payment, and tenant self-service |
| Payment module skeleton | `internal/modules/payment` | placeholder |
| Product module skeleton | `internal/modules/product` | placeholder |
| Order module skeleton | `internal/modules/order` | placeholder |
| Billing permissions | permission seed | done |
| Billing migrations | `migrations/000050+` | done |

## Requirement Traceability

| Trace ID | Requirement | Business reason | Database tables | Migration file | Seed file | Domain/Service | API endpoint | Test coverage | Status | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| BILL-REQ-001 | Platform admin can create plan | Plan menjadi paket SaaS | `billing_plans` | `000050_create_billing_plan_catalog` | `000055_seed_billing_features_plans` | `PlanService` | `POST /platform/billing/plans` | `TestCreatePlan` | done | Platform permission required |
| BILL-REQ-002 | Plan has prices monthly/yearly | Mendukung billing interval | `billing_plan_prices` | `000050_create_billing_plan_catalog` | `000055_seed_billing_features_plans` | `PlanService` | `GET /platform/billing/plans` | `TestPlanPrice` | done | Currency IDR first |
| BILL-REQ-003 | Feature catalog is configurable | Fitur tidak hardcoded | `billing_features` | `000050_create_billing_plan_catalog` | `000055_seed_billing_features_plans` | `FeatureService` | `GET /platform/billing/features` | `TestFeatureList` | done | Feature key stable |
| BILL-REQ-004 | Plan has entitlement values | Paket menentukan akses fitur | `billing_plan_entitlements` | `000050_create_billing_plan_catalog` | `000056_seed_billing_plan_entitlements` | `PlanEntitlementService` | `PUT /platform/billing/plans/:id/entitlements` | `TestPlanEntitlement` | done | Validate value type |
| BILL-REQ-005 | Organization has active subscription | Tenant harus punya status billing | `billing_subscriptions` | `000051_create_billing_subscriptions` | optional demo seed | `SubscriptionService` | `GET /app/billing/current-plan` | `TestSubscriptionServiceCreateActiveSyncsEntitlements` | done | One usable subscription per organization is enforced and tenant billing can resolve current plan from it. |
| BILL-REQ-006 | Organization entitlement snapshot exists | Runtime access tidak bergantung langsung ke plan | `organization_entitlements` existing | `000016_create_organization_entitlements` | generated from billing plan | `SubscriptionService`, `EntitlementSink` | internal service | `billing_repository_integration_test`, `TestSubscriptionServiceCreateActiveSyncsEntitlements` | done | Runtime entitlement snapshot is synced from plan entitlements without creating a second source of truth. |
| BILL-REQ-007 | Feature access can be checked | Modul bisa dikunci per paket | `organization_entitlements` existing | `000016_create_organization_entitlements` | `000056_seed_billing_plan_entitlements` | `BillingGuardService` | internal service | `billing_guard_service_test` | done | Guard service reuses organization entitlement evaluation and blocks feature access when entitlement is missing or subscription is not usable. |
| BILL-REQ-008 | Quota can be checked | Limit penggunaan paket | `organization_usage_counters`, module owner tables | `000016_create_organization_entitlements` | optional usage seed | `UsageService`, `BillingGuardService`, `TenantBillingService` | `GET /app/billing/usage`, `GET /app/billing/current-plan` | `TestTenantBillingServiceCurrentPlanIncludesUsageSummary` | done | Tenant billing now exposes single-metric usage lookup and monthly aggregate usage summary for usage-counter based features on the current-plan payload, while landing/domain/member quota guards continue using snapshot/runtime checks. |
| BILL-REQ-009 | Invoice can be generated | Tenant dapat tagihan | `billing_invoices`, `billing_invoice_items` | `000052_create_billing_invoices_payments` | none | `InvoiceService` | `POST /platform/billing/invoices` | `TestCreateInvoice` | done | Service calculates total |
| BILL-REQ-010 | Payment can be recorded manually | MVP belum wajib gateway | `billing_payments` | `000052_create_billing_invoices_payments` | none | `PaymentService` | `POST /platform/billing/invoices/:id/mark-paid` | `TestMarkInvoicePaid` | done | Idempotent |
| BILL-REQ-011 | Subscription status changes are audited | Billing harus traceable | `billing_subscription_events` | `000053_create_billing_events` | none | `SubscriptionService` | internal | `TestSubscriptionServiceChangeStatusPropagatesActorToEventAndExpire` | done | Subscription event audit now stores `actor_user_id` when the caller provides it, including status change, scheduled cancellation, and upgrade activation flows. |
| BILL-REQ-012 | Payment provider events are idempotent | Webhook aman dari duplicate | `billing_payment_events` | `000053_create_billing_events` | none | `PaymentService` | future webhook | `TestPaymentServiceRecordProviderEventReturnsExistingOnDuplicate` | in_progress | Billing core now has idempotent provider-event recording by `(provider, provider_event_id)`; webhook endpoint, provider signature verification, and gateway-specific adapters are still pending. |
| BILL-REQ-013 | Landing page create respects quota | Mencegah abuse free/starter | `organization_entitlements`, landing tables | existing + billing seeds | `000056_seed_billing_plan_entitlements` | `BillingGuardService`, Landing service | Landing create endpoint | `TestCreateLandingPageQuota` | done | Use existing `landing.*` prefix |
| BILL-REQ-014 | Custom domain respects entitlement | Custom domain hanya paket tertentu | `organization_entitlements`, `organization_domains` | existing + billing seeds | `000056_seed_billing_plan_entitlements` | `BillingGuardService`, Domain services | Domain create/bind endpoints | `TestCustomDomainEntitlement` | done | Organization custom domain create checks `domain.enabled` and `domain.max_custom_domains`; Landing binding checks `landing.custom_domain` |
| BILL-REQ-015 | Suspended tenant cannot use paid features | Revenue protection | `billing_subscriptions` | `000051_create_billing_subscriptions` | none | `BillingGuardService` | protected feature endpoints | `TestSuspendedSubscriptionBlocked` | done | Feature guard blocks suspended subscription, while tenant billing screen falls back to latest subscription for recovery flows. |
| BILL-REQ-016 | Tenant can view current plan and invoice | Self-service billing dashboard | `billing_subscriptions`, `billing_invoices` | `000051`, `000052` | none | `TenantBillingService` | `GET /app/billing/current-plan`, `GET /app/billing/invoices` | `TestTenantBillingScoped` | done | Tenant scoped current-plan dan invoice list sudah tersedia |
| BILL-REQ-017 | Platform admin can manage subscription | Support support/admin operations | `billing_subscriptions`, `billing_subscription_events` | `000051`, `000053` | `000054_seed_billing_permissions` | `SubscriptionService` | `POST /platform/billing/subscriptions` | `TestPlatformCreateSubscription` | done | Platform tenant required |
| BILL-REQ-018 | Upgrade activates entitlement after payment | Paid upgrade changes runtime access | subscriptions, invoices, payments, org entitlements | `000051`, `000052`, `000016` | plan entitlement seed | `SubscriptionService`, `PaymentService` | upgrade/payment endpoints | `TestUpgradeAfterPayment` | done | `mark-paid` now detects `upgrade_request` invoice metadata, reactivates the subscription onto the target plan, and resyncs runtime entitlements. |
| BILL-REQ-019 | Downgrade is scheduled safely | Data tidak langsung hilang | `billing_subscriptions`, event table | `000051`, `000053` | none | `SubscriptionService` | `POST /app/billing/cancel` or downgrade endpoint | `TestSubscriptionServiceScheduleCancellationUsesPeriodEndFlag` | done | Tenant cancel now sets `cancel_at_period_end` while keeping the current subscription active until the billing period ends. |
| BILL-REQ-020 | Billing docs stay discoverable | Developer/AI context connected | docs | none | none | documentation | none | doc review | done | README/AGENTS/migration-guide link this workflow |

## Task Index

| Phase | Task ID | Status |
| --- | --- | --- |
| Review | BILL-0001 | done |
| Review | BILL-0002 | done |
| Review | BILL-0003 | done |
| Database | BILL-DB-001 | done |
| Database | BILL-DB-002 | done |
| Database | BILL-DB-003 | done |
| Database | BILL-DB-004 | done |
| Database | BILL-DB-005 | done |
| Seed | BILL-SEED-001 | done |
| Seed | BILL-SEED-002 | done |
| Seed | BILL-SEED-003 | done |
| Domain/DTO | BILL-BE-001 | done |
| Domain/DTO | BILL-BE-002 | done |
| Domain/DTO | BILL-BE-003 | done |
| Repository | BILL-REPO-001 | done |
| Repository | BILL-REPO-002 | done |
| Repository | BILL-REPO-003 | done |
| Repository | BILL-REPO-004 | done |
| Repository | BILL-REPO-005 | done |
| Service | BILL-SVC-001 | done |
| Service | BILL-SVC-002 | done |
| Service | BILL-SVC-003 | done |
| Service | BILL-SVC-004 | done |
| Service | BILL-SVC-005 | done |
| API | BILL-API-001 | done |
| API | BILL-API-002 | done |
| API | BILL-API-003 | done |
| Guard | BILL-GUARD-001 | done |
| Guard | BILL-GUARD-002 | done |
| Guard | BILL-GUARD-003 | done |
| Docs | BILL-DOC-001 | done |
| Tests | BILL-TEST-001 | done |
| Tests | BILL-TEST-002 | done |
| Tests | BILL-TEST-003 | done |
| Docs | BILL-DOC-002 | done |

## API Index

Endpoint path berikut relatif terhadap `/api/v1`.

| Endpoint | Task | Permission | Status |
| --- | --- | --- | --- |
| `GET /platform/billing/plans` | BILL-API-001 | `platform.billing.plan.read` | done |
| `POST /platform/billing/plans` | BILL-API-001 | `platform.billing.plan.manage` | done |
| `GET /platform/billing/plans/:id` | BILL-API-001 | `platform.billing.plan.read` | done |
| `PATCH /platform/billing/plans/:id` | BILL-API-001 | `platform.billing.plan.manage` | done |
| `DELETE /platform/billing/plans/:id` | BILL-API-001 | `platform.billing.plan.manage` | done |
| `GET /platform/billing/plans/:id/prices` | BILL-API-001 | `platform.billing.plan_price.read` | done |
| `POST /platform/billing/plans/:id/prices` | BILL-API-001 | `platform.billing.plan_price.manage` | done |
| `PATCH /platform/billing/plans/:id/prices/:priceId` | BILL-API-001 | `platform.billing.plan_price.manage` | done |
| `DELETE /platform/billing/plans/:id/prices/:priceId` | BILL-API-001 | `platform.billing.plan_price.manage` | done |
| `GET /platform/billing/features` | BILL-API-001 | `platform.billing.feature.read` | done |
| `POST /platform/billing/features` | BILL-API-001 | `platform.billing.feature.manage` | done |
| `PATCH /platform/billing/features/:id` | BILL-API-001 | `platform.billing.feature.manage` | done |
| `GET /platform/billing/plans/:id/entitlements` | BILL-API-001 | `platform.billing.entitlement.read` | done |
| `PUT /platform/billing/plans/:id/entitlements` | BILL-API-001 | `platform.billing.entitlement.manage` | done |
| `GET /platform/billing/subscriptions` | BILL-API-002 | `platform.billing.subscription.read` | done |
| `POST /platform/billing/subscriptions` | BILL-API-002 | `platform.billing.subscription.manage` | done |
| `PATCH /platform/billing/subscriptions/:id` | BILL-API-002 | `platform.billing.subscription.manage` | done |
| `POST /platform/billing/subscriptions/:id/cancel` | BILL-API-002 | `platform.billing.subscription.manage` | done |
| `POST /platform/billing/subscriptions/:id/suspend` | BILL-API-002 | `platform.billing.subscription.manage` | done |
| `GET /platform/billing/invoices` | BILL-API-002 | `platform.billing.invoice.read` | done |
| `POST /platform/billing/invoices` | BILL-API-002 | `platform.billing.invoice.manage` | done |
| `POST /platform/billing/invoices/:id/mark-paid` | BILL-API-002 | `platform.billing.payment.manage` | done |
| `GET /app/billing/current-plan` | BILL-API-003 | `organization.billing.read` | done |
| `GET /app/billing/usage` | BILL-API-003 | `organization.billing.read` | done |
| `GET /app/billing/invoices` | BILL-API-003 | `organization.billing.read` | done |
| `POST /app/billing/upgrade` | BILL-API-003 | `organization.billing.manage` | done |
| `POST /app/billing/cancel` | BILL-API-003 | `organization.billing.manage` | done |

## Migration Index

| Migration | Task | Status | Notes |
| --- | --- | --- | --- |
| `000016_create_organization_entitlements` | existing dependency | done | Runtime entitlement and usage counter source. |
| `000050_create_billing_plan_catalog` | BILL-DB-001 | done | Down/up verified. |
| `000051_create_billing_subscriptions` | BILL-DB-002 | done | Down/up verified. |
| `000052_create_billing_invoices_payments` | BILL-DB-003 | done | Down/up verified. |
| `000053_create_billing_events` | BILL-DB-004 | done | Down/up verified. |
| `000054_seed_billing_permissions` | BILL-SEED-001 | done | Idempotency verified by direct SQL rerun. |
| `000055_seed_billing_features_plans` | BILL-SEED-002 | done | Idempotency verified by direct SQL rerun. |
| `000056_seed_billing_plan_entitlements` | BILL-SEED-003 | done | Idempotency verified by direct SQL rerun. |
| `000057_seed_billing_plan_price_permissions` | follow-up | done | Adds granular plan price permissions and grants them to `super_admin`. |

## Permission Traceability

| Permission | Capability | Status |
| --- | --- | --- |
| `platform.billing.plan.read` | Read plan catalog | done |
| `platform.billing.plan.manage` | Create/update/delete plan | done |
| `platform.billing.plan_price.read` | Read plan prices | done |
| `platform.billing.plan_price.manage` | Create/update/delete plan prices | done |
| `platform.billing.feature.read` | Read feature catalog | done |
| `platform.billing.feature.manage` | Create/update feature catalog | done |
| `platform.billing.entitlement.read` | Read plan entitlement template | done |
| `platform.billing.entitlement.manage` | Update plan entitlement template | done |
| `platform.billing.subscription.read` | Read organization subscriptions across tenants | done |
| `platform.billing.subscription.manage` | Create/update/cancel/suspend subscriptions | done |
| `platform.billing.invoice.read` | Read invoices across tenants | done |
| `platform.billing.invoice.manage` | Create/void invoices | done |
| `platform.billing.payment.manage` | Manual payment and payment event operations | done |
| `organization.billing.read` | Tenant reads current plan, usage, invoices | done |
| `organization.billing.manage` | Tenant requests upgrade/cancel | done |

## Feature Key Traceability

| Feature key | Consumer | Source seed | Status | Notes |
| --- | --- | --- | --- | --- |
| `landing.enabled` | Landing page access guard | billing feature seed | done | Billing guard and landing access policy already consume this entitlement before protected landing actions. |
| `landing.max_pages` | Landing page create quota | billing feature seed | done | Count from `landing_pages`. |
| `landing.max_sections_per_page` | Landing section and template/duplicate flows | billing feature seed | done | Count current sections per landing page before create; duplicate/template instantiation also enforce the same quota. |
| `landing.custom_domain` | Landing domain binding | billing feature seed | done | Separate from org domain registry limit. |
| `domain.max_custom_domains` | Organization domain service | billing feature seed | done | Count from verified/active custom domains in `organization_domains`. |
| `users.max_users` | Membership/invite service | billing feature seed | done | Count active organization members only. |
| `whatsapp.max_messages_per_month` | Notification/WhatsApp future | billing feature seed | planned | Use `organization_usage_counters`. |
| `automation.max_runs_per_month` | Automation future | billing feature seed | planned | Use `organization_usage_counters`. |

## Development Checklist

- [x] Existing architecture reviewed for documentation.
- [x] Entitlement source conflict identified.
- [x] Billing documentation created.
- [x] Migration created.
- [x] Migration down tested.
- [x] Seed created.
- [x] Seed idempotent tested.
- [x] Domain models created.
- [x] DTOs created.
- [x] Repositories implemented.
- [x] Services implemented.
- [x] Handlers implemented.
- [ ] Guard integrated.
- Guard integration saat ini selesai untuk create landing page (`BILL-GUARD-001`), custom domain flow (`BILL-GUARD-002`), dan member invite quota (`BILL-GUARD-003`).
- [x] OpenAPI updated.
- [x] Unit tests added.
- [x] Integration tests added.
- [ ] Documentation finalized after implementation.

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-29 | `organization` remains billing account | Repo multi-tenant contract already uses organization as canonical tenant. |
| 2026-06-29 | Billing MVP reuses `organization_entitlements` | Avoid duplicate runtime entitlement source and align migration `000016`. |
| 2026-06-29 | Billing MVP reuses `organization_usage_counters` | Avoid duplicate usage counter source and align existing quota service. |
| 2026-06-29 | `internal/modules/billing` owns subscription/invoice/payment | Keeps plan/billing/subscription cohesive without new module split. |
| 2026-06-29 | Manual payment first, gateway later | Avoid premature provider coupling while preserving event/idempotency schema. |
| 2026-06-29 | Billing handlers must be protected router wiring | Current module public route registration is no-op and not suitable for sensitive billing route. |
| 2026-06-30 | Platform billing update and mark-paid use unscoped lookup in billing repository | Platform endpoints operate lintas organization sehingga handler tidak aman bila bergantung pada `organization_id` dari client. |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-29 | Added billing plan documentation set | `docs/billing-plan-concept-reference.md`, `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md` | Documentation aligns user reference with README, AGENTS, multi-tenant, organization entitlement, and migration conventions. |
| 2026-06-29 | Added billing schema and seed migrations | `migrations/000050_create_billing_plan_catalog.*.sql`, `migrations/000051_create_billing_subscriptions.*.sql`, `migrations/000052_create_billing_invoices_payments.*.sql`, `migrations/000053_create_billing_events.*.sql`, `migrations/000054_seed_billing_permissions.*.sql`, `migrations/000055_seed_billing_features_plans.*.sql`, `migrations/000056_seed_billing_plan_entitlements.*.sql` | Down/up migration and seed idempotency verified. |
| 2026-06-29 | Added billing model, DTO, and error foundations | `internal/modules/billing/model`, `internal/modules/billing/dto`, `internal/modules/billing/errors.go` | Phase 3 foundation added and verified with `go test ./internal/modules/billing/...`. |
| 2026-06-29 | Added billing plan and feature repositories | `internal/modules/billing/repository/plan_repository.go`, `internal/modules/billing/repository/feature_repository.go`, `internal/modules/billing/repository/plan_entitlement_repository.go`, `internal/modules/billing/repository/billing_repository_integration_test.go` | Completed `BILL-REPO-001` and `BILL-REPO-002`; verified with billing repository integration test. |
| 2026-06-29 | Added billing subscription, invoice, payment, and event repositories | `internal/modules/billing/repository/subscription_repository.go`, `internal/modules/billing/repository/invoice_repository.go`, `internal/modules/billing/repository/payment_repository.go`, `internal/modules/billing/repository/billing_repository_integration_test.go` | Completed `BILL-REPO-003` and `BILL-REPO-004`; verified with billing repository integration test. |
| 2026-06-29 | Added billing entitlement sink adapter | `internal/modules/billing/repository/entitlement_sink.go`, `internal/modules/billing/repository/billing_repository_integration_test.go` | Completed `BILL-REPO-005`; plan entitlements sync into existing `organization_entitlements` with subscription ID as source reference. |
| 2026-06-29 | Added billing plan, feature, and plan entitlement services | `internal/modules/billing/service/plan_service.go`, `internal/modules/billing/service/feature_service.go`, `internal/modules/billing/service/plan_entitlement_service.go`, `internal/modules/billing/service/plan_entitlement_service_test.go` | Completed `BILL-SVC-001`; service validates enum and entitlement value type before repository write. |
| 2026-06-29 | Added billing subscription service | `internal/modules/billing/service/subscription_service.go`, `internal/modules/billing/service/subscription_service_test.go` | Completed `BILL-SVC-002`; create/update records subscription events, validates status transitions, and syncs/expires plan entitlement snapshots. |
| 2026-06-29 | Added billing invoice service | `internal/modules/billing/service/invoice_service.go`, `internal/modules/billing/service/invoice_service_test.go` | Completed `BILL-SVC-003`; service calculates item totals and invoice subtotal/tax/discount/total before repository write. |
| 2026-06-29 | Added billing payment service | `internal/modules/billing/service/payment_service.go`, `internal/modules/billing/service/payment_service_test.go` | Completed `BILL-SVC-004`; manual mark-paid creates payment/event, updates invoice paid status, and returns existing paid payment on repeat calls. |
| 2026-06-29 | Added billing guard service | `internal/modules/billing/service/billing_guard_service.go`, `internal/modules/billing/service/billing_guard_service_test.go` | Completed `BILL-SVC-005`; guard checks usable subscription, feature entitlement, and snapshot quota before module actions. |
| 2026-06-30 | Added protected billing platform and tenant handlers | `internal/modules/billing/handler`, `internal/modules/billing/service/tenant_billing_service.go`, `internal/app/app.go`, `internal/app/dependency.go`, `internal/app/router.go` | Completed `BILL-API-001`, `BILL-API-002`, and partial `BILL-API-003`; platform routes are protected under `/api/v1/platform/billing`, tenant self-service currently covers `current-plan` and `invoices`. |
| 2026-06-30 | Added tenant billing usage and cancel endpoints | `internal/modules/billing/dto/request.go`, `internal/modules/billing/handler/tenant_billing_handler.go`, `internal/modules/billing/handler/tenant_billing_handler_test.go`, `internal/modules/billing/service/tenant_billing_service.go`, `internal/modules/billing/service/tenant_billing_service_test.go`, `internal/app/app.go` | Extended `BILL-API-003`; tenant self-service now covers `current-plan`, `usage`, `invoices`, and `cancel`. |
| 2026-06-30 | Wired billing quota guard into Landing page creation | `internal/app/app.go`, `internal/modules/landing/service/page_service_test.go` | Completed `BILL-GUARD-001`; Landing page create now checks `landing.max_pages` through `BillingGuardService` before persisting non-template pages. |
| 2026-06-30 | Wired billing feature guard into Landing custom domain binding | `internal/app/app.go`, `internal/modules/landing/domain/feature.go`, `internal/modules/landing/service/domain_service.go`, `internal/modules/landing/service/domain_service_unit_test.go` | Landing domain binding now checks `landing.custom_domain` before binding verified organization domains to a page. |
| 2026-06-30 | Wired billing feature and quota guard into organization custom domain create | `internal/app/app.go`, `internal/modules/organization/service/domain_service.go`, `internal/modules/organization/service/domain_service_test.go` | Completed `BILL-GUARD-002`; organization custom domain create now checks `domain.enabled` and snapshot quota `domain.max_custom_domains` before persisting new custom domains. |
| 2026-06-30 | Wired billing feature and quota guard into membership invite flow | `internal/app/app.go`, `internal/modules/organization/service/membership_service.go`, `internal/modules/organization/service/membership_service_test.go` | Completed `BILL-GUARD-003`; organization member invite now checks `users.invite_user` and snapshot quota `users.max_users` before creating invitations. |
| 2026-06-30 | Added OpenAPI documentation for implemented billing endpoints | `api/openapi.yaml`, `docs/billing-plan-development-traceability.md` | Completed `BILL-DOC-001`; documented current platform billing CRUD/self-service endpoints and billing request/response schemas without including future `upgrade` flow. |
| 2026-06-30 | Wired billing quota guard into Landing section creation | `internal/app/app.go`, `internal/modules/landing/domain/feature.go`, `internal/modules/landing/service/section_service.go`, `internal/modules/landing/service/section_service_unit_test.go` | Landing section create now checks snapshot quota `landing.max_sections_per_page` before persisting a new section on a page. |
| 2026-06-30 | Extended landing section quota guard to duplicate and template instantiation flows | `internal/modules/landing/service/page_service.go`, `internal/modules/landing/service/page_service_test.go`, `internal/modules/landing/service/template_service.go`, `internal/modules/landing/service/template_service_unit_test.go`, `internal/modules/landing/service/access_policy_test.go`, `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md` | Follow-up coverage now closes section quota bypass through page duplicate/template instantiation and adds explicit suspended-subscription access policy test coverage. |
| 2026-06-30 | Added tenant upgrade invoice request and suspended current-plan fallback | `internal/modules/billing/dto/request.go`, `internal/modules/billing/service/subscription_service.go`, `internal/modules/billing/service/tenant_billing_service.go`, `internal/modules/billing/service/tenant_billing_service_test.go`, `internal/modules/billing/handler/tenant_billing_handler.go`, `internal/modules/billing/handler/tenant_billing_handler_test.go`, `api/openapi.yaml`, `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md` | Completed `BILL-API-003`; tenant billing now supports `/app/billing/upgrade` invoice creation and lets suspended tenants retrieve latest billing subscription context for billing recovery flows. |
| 2026-06-30 | Wired invoice mark-paid flow to activate paid upgrade request | `internal/modules/billing/service/payment_service.go`, `internal/modules/billing/service/payment_service_test.go`, `internal/modules/billing/service/subscription_service.go`, `internal/modules/billing/service/subscription_service_test.go`, `internal/app/app.go`, `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md` | Closed `BILL-REQ-018`; when a paid invoice carries `billing_action=upgrade_request`, payment flow now promotes the referenced subscription to the target plan and resyncs runtime entitlements. |
| 2026-06-30 | Finalized billing documentation status against implemented code paths | `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md`, `docs/migration-guide.md` | Closed `BILL-DOC-002`; stale `planned` statuses for completed billing migrations, APIs, permissions, and core subscription/entitlement requirements were aligned with the current implementation. |
| 2026-06-30 | Changed tenant cancel flow into scheduled cancellation at period end | `internal/modules/billing/service/subscription_service.go`, `internal/modules/billing/service/subscription_service_test.go`, `internal/modules/billing/service/tenant_billing_service.go`, `internal/modules/billing/service/tenant_billing_service_test.go`, `internal/modules/billing/handler/tenant_billing_handler.go`, `internal/modules/billing/handler/tenant_billing_handler_test.go`, `api/openapi.yaml`, `docs/billing-plan-development-traceability.md` | Closed `BILL-REQ-019`; self-service cancel now preserves access until the end of the active billing period by setting `cancel_at_period_end` instead of immediately expiring the subscription. |
| 2026-06-30 | Propagated actor user ID into subscription audit events and entitlement side effects | `internal/modules/billing/service/subscription_service.go`, `internal/modules/billing/service/subscription_service_test.go`, `docs/billing-plan-development-traceability.md` | Closed `BILL-REQ-011`; actor identity now flows into `billing_subscription_events` and entitlement sync/expire paths when the operation originates from a known user. |
| 2026-06-30 | Added monthly usage summary to tenant current-plan billing response | `internal/modules/billing/service/tenant_billing_service.go`, `internal/modules/billing/service/tenant_billing_service_test.go`, `docs/billing-plan-development-traceability.md` | Closed `BILL-REQ-008`; current-plan now includes aggregate monthly usage for usage-counter based billing features while preserving the existing detailed usage lookup endpoint. |
| 2026-06-30 | Corrected landing entitlement feature traceability status | `docs/billing-plan-development-traceability.md` | Marked `landing.enabled` as implemented to match the existing billing guard and landing access-policy integration already present in code and tests. |
| 2026-06-30 | Started idempotent payment-provider event foundation in billing core | `internal/modules/billing/dto/request.go`, `internal/modules/billing/dto/response.go`, `internal/modules/billing/repository/payment_repository.go`, `internal/modules/billing/service/payment_service.go`, `internal/modules/billing/service/payment_service_test.go`, `internal/modules/billing/service/response_mapper.go`, `docs/billing-plan-development-traceability.md` | Moved `BILL-REQ-012` to `in_progress`; provider events can now be recorded idempotently and duplicate webhook deliveries can resolve to the existing billing event record. |
| 2026-06-30 | Added platform billing plan price CRUD endpoints and granular permissions | `internal/modules/billing/dto/request.go`, `internal/modules/billing/handler/platform_billing_handler.go`, `internal/modules/billing/repository/plan_repository.go`, `internal/modules/billing/service/plan_service.go`, `internal/modules/billing/service/plan_service_test.go`, `internal/modules/billing/repository/billing_repository_integration_test.go`, `internal/modules/billing/repository/permission_seed_integration_test.go`, `migrations/000057_seed_billing_plan_price_permissions.*.sql`, `internal/modules/user/seeder/super_admin.go`, `api/openapi.yaml`, `docs/billing-plan-development-traceability.md` | Closed backend gap for separate price management, added `platform.billing.plan_price.read/manage`, verified unit and integration tests, and ensured `super_admin` gets the new permission grants. |
