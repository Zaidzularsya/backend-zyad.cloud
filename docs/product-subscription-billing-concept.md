# Konsep Domain: Product/Catalog, Subscription, Billing

> Dokumen ini adalah **konsep final** hasil refactor pemisahan domain dari modul `internal/modules/billing/`
> tunggal (dibuat 2026-06-29/30) menjadi 3 domain eksplisit. Dokumen requirement lama
> (`docs/billing-plan-concept-reference.md`, `docs/billing-plan-development-tasks.md`,
> `docs/billing-plan-development-traceability.md`, `docs/reference-plan-billing-subscribe.md`) tetap
> dipertahankan sebagai riwayat/rujukan desain awal — dokumen ini adalah struktur final yang berlaku.

## 1. Mengapa Dipisah

Tabel dan kode awal memaksa 3 konsep bisnis berbeda ke satu prefix `billing_`:

| Pertanyaan | Domain |
|---|---|
| Apa yang dijual platform? | **Product/Catalog** |
| Siapa yang berlangganan dan bagaimana status kontraknya? | **Subscription** |
| Bagaimana tagihan dan pembayaran diproses? | **Billing** |

Mencampur ketiganya di satu nama (`billing_plans`, `billing_subscriptions`, dll) membuat:
- Nama tabel tidak mencerminkan tanggung jawab sebenarnya (plan/feature bukan aktivitas "billing").
- Satu Go package (`internal/modules/billing`) menangani logika yang seharusnya independen (katalog produk
  vs status langganan vs tagihan), menyulitkan perubahan salah satu tanpa risiko menyentuh yang lain.
- Permission dan route API ikut membingungkan (`platform.billing.plan.manage` untuk mengelola katalog produk).

## 2. Pemisahan Domain

### Product/Catalog Domain
Master data yang **dijual** platform, tidak terikat organisasi mana pun (data global/platform-level).

| Tabel | Fungsi |
|---|---|
| `product_features` | Master fitur/kapabilitas platform (mis. `landing.max_pages`, `crm.enabled`) |
| `product_plans` | Paket yang dijual (Free, Starter, Growth, Business, Enterprise) |
| `product_plan_prices` | Variasi harga per plan (monthly/yearly/one_time/custom, per currency) |
| `product_plan_entitlements` | Nilai/limit tiap fitur untuk tiap plan (template, bukan runtime) |

### Subscription Domain
Kontrak/langganan aktif milik organization tertentu — **menghubungkan** organization ke Product/Catalog.

| Tabel | Fungsi |
|---|---|
| `customer_subscriptions` | Langganan aktif organization ke sebuah plan, dengan status lifecycle |
| `subscription_events` | Audit trail perubahan status subscription (siapa, kapan, dari status apa ke apa) |

**Tidak ada `subscription_entitlements` baru** — lihat bagian 6.

### Billing Domain
Tagihan dan pembayaran — hasil dari subscription yang aktif diproses menjadi kewajiban bayar.

| Tabel | Fungsi |
|---|---|
| `billing_invoices` | Tagihan (nomor invoice, subtotal/diskon/pajak/total, status) |
| `billing_invoice_items` | Rincian item dalam satu invoice |
| `billing_payments` | Pembayaran atas satu invoice (provider, status, jumlah) |
| `billing_payment_events` | Event/webhook dari payment provider (Xendit/Midtrans), untuk idempotency & audit |

## 3. Mapping Tabel Lama → Baru

| Existing Table | New Table | Domain | Catatan |
|---|---|---|---|
| `billing_features` | `product_features` | Product/Catalog | Master fitur platform |
| `billing_plans` | `product_plans` | Product/Catalog | Paket yang dijual |
| `billing_plan_prices` | `product_plan_prices` | Product/Catalog | Harga plan |
| `billing_plan_entitlements` | `product_plan_entitlements` | Product/Catalog | Isi/limit fitur per plan |
| `billing_subscriptions` | `customer_subscriptions` | Subscription | Langganan aktif organization |
| `billing_subscription_events` | `subscription_events` | Subscription | Audit trail lifecycle subscription |
| `billing_invoices` | `billing_invoices` | Billing | Tetap |
| `billing_invoice_items` | `billing_invoice_items` | Billing | Tetap |
| `billing_payments` | `billing_payments` | Billing | Tetap |
| `billing_payment_events` | `billing_payment_events` | Billing | Tetap |

Semua kolom, CHECK constraint, UNIQUE constraint, FK, dan index dari tabel lama **dipertahankan persis**,
hanya nama tabel/constraint yang menyesuaikan penamaan domain baru. Tidak ada perubahan bentuk data.

## 4. ERD Konseptual

```
┌─────────────────────────┐
│   PRODUCT / CATALOG      │
│                          │
│  product_features        │
│  product_plans           │───┐
│  product_plan_prices      │◄──┤ plan_id
│  product_plan_entitlements│◄──┤ plan_id, feature_id
└─────────────────────────┘   │
                                │ plan_id
┌─────────────────────────┐   │
│      SUBSCRIPTION         │   │
│                          │   │
│  customer_subscriptions  │◄──┘
│    - organization_id ────┼──► organizations (existing)
│  subscription_events     │◄─── subscription_id, organization_id
└───────────┬─────────────┘
            │ subscription_id (nullable, RESTRICT)
            │ organization_id
┌───────────▼─────────────┐
│         BILLING           │
│                          │
│  billing_invoices         │◄── organization_id, subscription_id
│  billing_invoice_items    │◄── invoice_id
│  billing_payments         │◄── invoice_id, organization_id
│  billing_payment_events   │◄── payment_id, invoice_id
└─────────────────────────┘

Sync runtime entitlement (tidak digambar sebagai tabel baru):
customer_subscriptions --(sync saat create/activate/expire)--> organization_entitlements (existing, migration 000016)
                                                                  source='plan', source_reference=<subscription_id>
```

## 5. Flow Bisnis

```
1. Admin platform membuat product_features (mis. "landing.max_pages")
2. Admin platform membuat product_plans (mis. "growth")
3. Admin platform membuat product_plan_prices untuk plan tsb (mis. growth/monthly/IDR)
4. Admin platform mengatur product_plan_entitlements: growth -> landing.max_pages = 10
5. Customer/organization subscribe ke plan growth
   -> customer_subscriptions dibuat (status: trialing/active)
   -> subscription_events dicatat (event: subscription_created)
   -> entitlement plan disinkron ke organization_entitlements (source='plan', source_reference=subscription.id)
6. billing_invoices dibuat untuk periode tagihan subscription tsb
   -> billing_invoice_items diisi (mis. "Growth Plan - Monthly")
7. Customer membayar -> billing_payments dibuat
8. Webhook payment provider (Xendit) diterima -> billing_payment_events dicatat (idempotent by provider_event_id)
9. Invoice status berubah jadi 'paid' -> subscription tetap aktif untuk periode berikutnya
   (atau, jika ini upgrade: customer_subscriptions.plan_id berubah + entitlement disinkron ulang)
10. Jika subscription dibatalkan/expired -> entitlement di organization_entitlements di-expire
    (subscription_events mencatat event: subscription_canceled/expired)
```

## 6. Keputusan: Tidak Membuat `subscription_entitlements`

**Keputusan: TIDAK dibuat.**

Alasan: `organization_entitlements` (tabel existing, migration 000016, milik modul `organization`) **sudah**
berfungsi persis sebagai snapshot runtime yang dimaksud:
- Setiap kali subscription dibuat/diaktifkan, entitlement dari `product_plan_entitlements` disinkron ke
  `organization_entitlements` dengan `source = 'plan'` dan `source_reference = <customer_subscriptions.id>`.
- `effective_from`/`effective_until` di `organization_entitlements` mengikuti periode subscription — jika
  plan berubah, tenant lama tidak otomatis rusak karena entitlement snapshot lama tetap berlaku sampai
  `effective_until`, baru digantikan entitlement plan baru saat sinkronisasi berikutnya.
- Kode yang menjalankan sinkronisasi ini sudah ada dan teruji: `EntitlementSink.SyncPlanEntitlements()` dan
  `ExpirePlanEntitlements()` (pindah dari `internal/modules/billing/repository/entitlement_sink.go` ke
  `internal/modules/subscription/repository/entitlement_sink.go`).

Membuat `subscription_entitlements` baru akan menciptakan **source of truth kedua** untuk entitlement
runtime — persis yang sudah eksplisit dilarang di `docs/billing-plan-concept-reference.md`: *"Jangan membuat
runtime entitlement table kedua. Gunakan `organization_entitlements` existing."* Runtime feature/quota check
(mis. saat membuat landing page baru, cek kuota `landing.max_pages`) tetap membaca `organization_entitlements`
— **bukan** `product_plan_entitlements` langsung (yang hanya berisi template plan, bukan entitlement aktif
tenant tertentu). Flow ini tidak berubah oleh refactor ini.

## 7. Aturan Status per Entity

| Entity | Kolom Status | Nilai Valid |
|---|---|---|
| `product_plans` | `plan_type` | `free`, `trial`, `paid`, `enterprise` |
| `product_plans` | `is_active`/`is_public` | boolean, independen dari `plan_type` |
| `product_plan_prices` | `is_active` | boolean |
| `product_features` | `is_active` | boolean |
| `customer_subscriptions` | `status` | `trialing` → `active` → (`past_due` → `grace_period` →) `suspended`/`canceled`/`expired`. Status *usable* untuk guard runtime: `trialing`, `active`, `grace_period`, `past_due` |
| `billing_invoices` | `status` | `draft` → `open` → `paid`/`void`/`expired`/`failed` |
| `billing_payments` | `status` | `pending` → `paid`/`failed`/`expired`/`refunded` |
| `subscription_events` / `billing_payment_events` | `event_type` | free-text snake_case (audit log, bukan enum tertutup) |

## 8. Risiko dan Backward Compatibility

- **Tidak ada data produksi yang harus dijaga** — migration 000050–000057 baru dibuat 2 hari sebelum
  refactor ini, masih fase development. Migration baru (Fase C) langsung `DROP` tabel lama setelah data
  seed dipindahkan ulang ke tabel baru — tidak perlu strategi migrasi data kompleks.
- **Breaking change API & permission disengaja**: route `/platform/billing/plans*` → `/platform/product/*`,
  `/platform/billing/subscriptions*` → `/platform/subscriptions*`, permission `platform.billing.plan.*` →
  `platform.product.plan.*`, dst. Route `/api/v1/app/billing/*` (customer-facing) **tidak berubah**.
- **Frontend (`frontend.zyad.cloud`) sudah punya integrasi nyata** ke route platform yang berubah
  (`platform-billing.api.ts`, halaman `PlatformBillingPlansPage.vue`) — **sengaja tidak diperbaiki di paket
  kerja ini** (keputusan eksplisit pemilik repo). Daftar file yang perlu diupdate ada di
  `docs/product-subscription-billing-refactor-plan.md` bagian Follow-up Frontend.
- **Role/permission assignment tidak putus**: migration rename permission memakai `UPDATE permissions SET
  ...` (bukan delete+insert), sehingga baris `role_permissions` yang sudah mengacu ke `permission_id` lama
  tetap valid tanpa perlu re-seed assignment role.
- **Dokumen requirement lama tidak dihapus** — tetap ada sebagai riwayat/rujukan desain awal, dengan catatan
  di bagian atas yang menunjuk ke 3 dokumen baru ini sebagai struktur final.
