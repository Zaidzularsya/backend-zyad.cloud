---
description: Reading-order checklist untuk kerja di Plan, Billing, Subscription, Entitlement, Quota, Invoice, atau Payment. Pakai kalau task menyentuh internal/modules/billing, internal/platform/doku, atau organization_entitlements/organization_usage_counters.
---

# Billing Plan Workflow

Baca dokumen berikut secara berurutan sebelum mengerjakan Plan/Billing/Subscription/Entitlement/Quota/Invoice/Payment:

1. `README.md` — konteks platform multi-tenant dan module billing.
2. `AGENTS.md` — batas layer, migration, dan gaya kerja repository.
3. `docs/reference-plan-billing-subscribe.md` — source requirement awal.
4. `docs/billing-plan-concept-reference.md` — konsep domain, boundary, table, service, API, dan integrasi existing.
5. `docs/billing-plan-development-tasks.md` — breakdown pekerjaan bertahap.
6. `docs/billing-plan-development-traceability.md` — traceability requirement, migration, seed, API, permission, dan test.
7. `docs/migration-guide.md` — sebelum membuat migration billing.

Target canonical module adalah `internal/modules/billing`. Runtime entitlement dan usage quota harus selaras
dengan `organization_entitlements` dan `organization_usage_counters` existing — **jangan** membuat source of
truth kedua tanpa migration deprecation eksplisit.

Payment gateway aktif adalah **DOKU** (`internal/platform/doku`), bukan Xendit — beberapa dokumen lama masih
menyebut Xendit, cek kode aktual kalau ragu.
