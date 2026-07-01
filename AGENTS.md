# Agent Working Guide

## Bahasa dan gaya kerja
- Jelaskan hasil kerja dalam Bahasa Indonesia.
- Gunakan Bahasa Inggris untuk code, command, file name, branch name, dan commit message.
- Buat perubahan kecil dan bertahap.
- Jangan menambah dependency production tanpa konfirmasi.
- Jangan mengubah arsitektur utama tanpa konfirmasi.

## Scope repo
- Bacalah dokumen `README.md` terlebih dahulu untuk memahami konteks dari repo.
- Dokumentasi development berada di `docs/`.
- 

## Development pattern
- Setiap module berada di `internal/modules/<module_name>`.
- Setiap module memakai layer `handler`, `service`, `repository`, `dto`, dan `model`.
- Handler hanya menangani HTTP binding, auth context, validasi request, dan response.
- Service memegang business rule, transaction boundary, idempotency, dan integrasi antar module.
- Repository hanya mengakses database dan tidak boleh berisi business rule.
- Platform integration ditempatkan di `internal/platform`.
- Shared core seperti auth, middleware, validation, response, permission, dan crypto ditempatkan di `internal/core`.

## Database
- Schema database langsung memakai `public`.
- Cleanup schema harus dicatat di migration terpisah agar traceable.
- Hindari TypeORM-style auto sync; semua perubahan database harus melalui migration.


--------------------

# Zyad Cloud Backend Go

Platform Multi Tenant

Backend go untuk 

# Stack
Berikut adalah rekomendasi susunan teknologi (tech stack) 
- **Bahasa Pemrograman:** Go – Dikenal dengan performanya yang sangat cepat dan efisien.
- **Web Framework:** Gin Web Framework – Pilihan tepat untuk membangun REST API yang cepat dengan fitur seperti routing dan middleware.
- **Basis Data Utama:** PostgreSQL – Sistem basis data relasional (RDBMS) yang sangat andal dan kaya fitur.
- **Cache / In-Memory DB:** Redis – Sangat optimal untuk caching, manajemen sesi (session management), dan antrean (queueing).

## Arsitektur

Struktur berikut dipertahankan sebagai arsitektur target:

```bash
backend-go/
├── cmd/
│   └── api/
│       └── main.go
│
├── api/
│   └── openapi.yaml
│
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   ├── dependency.go
│   │   ├── router.go
│   │   └── server.go
│   │
│   ├── config/
│   │   ├── config.go
│   │   ├── app.go
│   │   ├── auth.go
│   │   ├── database.go
│   │   ├── redis.go
│   │   ├── mikrotik.go
│   │   ├── xendit.go
│   │   └── whatsapp.go
│   │
│   ├── platform/
│   │   ├── database/
│   │   ├── logger/
│   │   ├── mail/
│   │   ├── mikrotik/
│   │   ├── redis/
│   │   ├── storage/
│   │   ├── whatsapp/
│   │   └── xendit/
│   │
│   ├── core/
│   │   ├── auth/
│   │   ├── crypto/
│   │   ├── errors/
│   │   ├── http/
│   │   ├── idempotency/
│   │   ├── middleware/
│   │   ├── permission/
│   │   └── validation/
│   │
│   ├── modules/
│   │   ├── account/
│   │   ├── asset/
│   │   ├── billing/
│   │   ├── contract/
│   │   ├── dashboard/
│   │   ├── landingpage/
│   │   ├── mikrotik/
│   │   ├── newsaggregator/
│   │   ├── order/
│   │   ├── organization/
│   │   ├── payment/
│   │   ├── product/
│   │   ├── provisioning/
│   │   ├── radius/
│   │   ├── resource/
│   │   └── user/
│   │
│   └── shared/
│       ├── pagination/
│       ├── response/
│       └── utils/
├── docs/
├── scripts/
├── tests/
├── .env.example
├── go.mod
├── go.sum
├── AGENTS.md
└── README.md
```

### Struktur didalam module
```bash
internal/modules/module_name/
├── handler/
│   └── module_name_handler.go
├── service/
│   └── module_name_service.go
├── repository/
│   └── module_name_repository.go
├── dto/
│   ├── create_module_name_request.go
│   └── module_name_response.go
├── model/
│   └── module_name.go
├── routes.go
└── errors.go
```


## Dokumentasi development

Dokumen revamp dan traceability ada di:

- `docs/migration-guide.md` untuk urutan dan cara menjalankan migration.
- `docs/auth-user-development-tasks.md` untuk breakdown task development module `internal/core/auth` dan `internal/modules/user`.
- `docs/auth-user-traceability-index.md` untuk index traceability requirement, API, migration, seed, dan status Auth/User.
- `docs/auth-user-public-api-contract.md` untuk kontrak API publik yang dipakai Frontend dan aplikasi eksternal.
- `docs/auth-user-migration-seed-plan.md` untuk rencana migration dan seed super admin Auth/User.
- `docs/notification-development-tasks.md` untuk breakdown task development core notification dan provider adapter.
- `docs/notification-traceability-index.md` untuk traceability requirement, config, API, migration, dan status Notification.
- `docs/billing-plan-concept-reference.md` untuk konsep domain Plan, Billing, Subscription, Entitlement, Quota, Invoice, dan Payment.
- `docs/billing-plan-development-tasks.md` untuk breakdown pekerjaan module `internal/modules/billing`.
- `docs/billing-plan-development-traceability.md` untuk traceability requirement, migration, seed, API, permission, test, dan status Billing Plan.

## Auth dan User Workflow

Saat mengerjakan module Auth dan User, jadikan urutan baca berikut sebagai acuan:

1. `README.md` untuk konteks repo.
2. `docs/reference-auth-user.md` untuk source requirement.
3. `docs/auth-user-development-tasks.md` untuk breakdown pekerjaan.
4. `docs/auth-user-traceability-index.md` untuk melacak kaitan requirement, API, migration, dan seed.
5. `docs/auth-user-public-api-contract.md` untuk kontrak response dan endpoint publik.
6. `docs/auth-user-migration-seed-plan.md` untuk urutan migration dan seed env.

## Notification Workflow

Saat mengerjakan core notification, jadikan urutan baca berikut sebagai acuan:

1. `README.md` untuk konteks repo.
2. `docs/reference-notification.md` untuk source requirement.
3. `docs/notification-development-tasks.md` untuk breakdown pekerjaan.
4. `docs/notification-traceability-index.md` untuk melacak requirement, config, API, migration, dan status.
5. `docs/migration-guide.md` sebelum membuat migration notification.

## Billing Plan Workflow

Saat mengerjakan Plan, Billing, Subscription, Entitlement, Quota, Invoice, dan Payment, jadikan urutan baca berikut sebagai acuan:

1. `README.md` untuk konteks platform multi-tenant.
2. `docs/reference-plan-billing-subscribe.md` untuk source requirement awal.
3. `docs/billing-plan-concept-reference.md` untuk konsep domain dan integrasi dengan repository existing.
4. `docs/billing-plan-development-tasks.md` untuk breakdown pekerjaan.
5. `docs/billing-plan-development-traceability.md` untuk melacak requirement, migration, seed, API, permission, dan test.
6. `docs/migration-guide.md` sebelum membuat migration billing.

Catatan penting: runtime entitlement dan usage quota sudah ada di `organization_entitlements` dan `organization_usage_counters`. Jangan membuat source of truth kedua tanpa migration deprecation yang eksplisit.
