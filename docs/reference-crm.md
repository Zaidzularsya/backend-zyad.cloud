# CRM Module Reference

Dokumen ini menyimpan source requirement, keputusan arsitektur, dan batas tanggung jawab modul CRM sebelum
implementasi dimulai — mengikuti checklist wajib `docs/module-map.md` bagian "Modul Stub — Checklist Sebelum
Mulai Menggarap".

## Source Requirement

- `backend/SKILLS.md` bagian B (Modul Aplikasi) dan bagian 3 (Matriks Hak Akses) — rancangan bisnis dan
  matriks permission 10 resource CRM (status 🗺️ ROADMAP saat dokumen ini ditulis).
- `docs/reference-plan-billing-subscribe.md` — rancangan entitlement `crm.enabled`, `crm.max_contacts`,
  `crm.import_export`, `crm.pipeline`, `crm.lead_form`.
- Keputusan eksplisit user (2026-09-07): scope penuh 10 resource dikerjakan bertahap per fase, isolasi data
  pakai RLS PostgreSQL (pola modul `landing`), entitlement gating aktif sejak fase pertama, model data
  Customer digabung ke tabel Contact (bukan tabel terpisah) karena belum ada satupun tabel `customer`/
  `contact` existing yang perlu dipertahankan kompatibilitasnya.

## Objective

Membangun modul CRM tenant-only yang menangani:

- Lead management: capture, scoring, assignment, convert ke Contact/Company/Deal.
- Contact & Company management: data pelanggan/prospek tenant, termasuk status "customer" (bukan tabel
  terpisah — lihat Naming Conflict).
- Sales pipeline: tahapan penjualan dinamis per organisasi, Kanban board di FE.
- Deal: nilai transaksi, tahapan, win/loss, approval diskon.
- Activity log: interaksi (call/email/meeting/task/note) terhadap Lead/Contact/Company/Deal.
- Quotation & Invoice tenant-ke-customer (bukan invoice billing platform — lihat Naming Conflict).
- Integration: koneksi CRM ke sistem eksternal (webhook, WhatsApp, form capture, dsb).
- Entitlement/quota enforcement per plan tenant.

## Architecture Decision

Mengikuti pola modul `landing` (satu-satunya modul existing dengan RLS penuh dan banyak sub-entity dalam
satu bounded module):

```txt
internal/modules/crm/
├── domain/
│   ├── company.go, contact.go, lead.go, pipeline.go, deal.go
│   ├── activity.go, quotation.go, invoice.go, integration.go
│   └── feature.go        # konstanta feature key: FeatureCRMEnabled, FeatureCRMMaxContacts, dst
├── repository/            # satu file per sub-resource, withTx(ctx, scope, fn) — pola branding_repository.go
├── service/               # satu file per sub-resource, functional-option quota/feature guard
├── handler/                # satu file per sub-resource, RegisterRoutes(router, checker)
├── dto/
└── errors.go
```

Tidak ada `routes.go`/`module.go` wrapper — route registration langsung di `internal/app/router.go` per
handler, mengikuti keputusan yang sudah ditetapkan untuk modul `landing` (lihat catatan di
`internal/app/router.go` dan `docs/reference-landing-page.md` bagian Naming Conflict).

## Rejected Module Split

CRM tidak dipecah menjadi modul terpisah per resource (`internal/modules/crm-lead`,
`internal/modules/crm-deal`, dst) — semua 10 resource tetap dalam satu bounded module `internal/modules/crm`
karena saling terkait erat (Lead convert ke Contact+Deal, Deal butuh Pipeline+Contact+Company, Quotation/
Invoice butuh Deal), sama seperti alasan modul `landing` tidak dipecah jadi page/media/forms terpisah.

## Naming Conflict

### Customer vs Contact

Matriks permission (`SKILLS.md`) memisahkan resource `customer` dan `contact` sebagai dua permission
berbeda. Secara data model, satu tabel `crm_contacts` dipakai untuk keduanya:

- Kolom `is_customer boolean` dan `lifecycle_stage` (`lead`/`contact`/`customer`/`churned`) membedakan
  status kontak.
- Permission `contact.*` dan `customer.*` sama-sama menjaga tabel `crm_contacts`, dibedakan oleh scope
  operasi (mis. `customer.archive` hanya berlaku untuk row dengan `is_customer = true`).
- Alasan: mencegah duplikasi field dan logic "pindah data antar tabel" saat kontak naik status jadi
  customer. Tidak ada tabel `customers`/`contacts` existing di database yang perlu dipertahankan
  kompatibilitasnya (dicek 2026-09-07 — hanya ada `customer_subscriptions` milik modul `subscription`,
  representasi langganan tenant ke platform, tidak berkaitan dengan pelanggan tenant).

### `crm_invoices` vs `billing_invoices`

Dua entitas yang sama sekali berbeda pihak — **jangan tertukar**:

- `billing_invoices` (modul `billing`, sudah ada) = tagihan **platform Zyad ke tenant** untuk pembayaran
  langganan SaaS tenant tersebut.
- `crm_invoices` (modul CRM, baru) = tagihan **tenant ke customer miliknya sendiri**, dihasilkan dari
  Deal/Quotation yang closed-won.

Nama tabel sengaja eksplisit `crm_invoices` (bukan `invoices`) supaya grep/pencarian langsung menunjukkan
dua modul berbeda.

## Tenant Boundary

- Semua tabel `crm_*` punya kolom `organization_id UUID NOT NULL`, dilindungi RLS lewat
  `apply_organization_rls()` (fungsi dari `migrations/000019_add_rls_foundation.up.sql`) — pola yang sama
  dengan `landing_*`.
- Modul CRM **hanya bisa diakses organization tipe `customer`** — digate lewat
  `middleware.RequireCustomerTenant()` di root grup route `/app/crm`. Organization tipe `platform`
  (superadmin) mendapat `403 CUSTOMER_ORGANIZATION_REQUIRED` di semua endpoint CRM.
- Base path tenant-facing: `/api/v1/app/crm/*` — mengikuti pola self-service tenant billing
  (`/api/v1/app/billing/*`), bukan pola `/admin/*` yang dipakai landing (landing memakai `/admin/*` untuk
  panel milik tenant sendiri, bukan superadmin — penamaan itu tidak diikuti CRM untuk menghindari ambiguitas).

## Security Baseline

- **RLS**: setiap tabel `crm_*` di-`apply_organization_rls()` di migration pembuatannya. Repository tetap
  wajib set `app.organization_id` per transaksi lewat `set_config` (defense-in-depth, bukan satu-satunya
  lapisan proteksi) — pola persis `internal/modules/landing/repository/branding_repository.go` method
  `withTx`.
- **Tenant guard**: `middleware.RequireActiveTenant()` + `middleware.RequireCustomerTenant()` di root grup
  `/app/crm`.
- **Permission**: format `resource.action` (`lead.read`, `deal.close_won`, dst — bare, tanpa prefix `crm.`)
  sesuai matriks `SKILLS.md`, diperiksa lewat `permissionmiddleware.RequireOrganizationOrGlobal(checker,
  "<resource>.<action>")` per endpoint.
- **Entitlement**: `crm.enabled` di-gate di level middleware root grup (`middleware.RequireEntitlement`,
  baru ditambahkan di `internal/core/middleware/entitlement.go`). `crm.max_contacts` di-enforce di
  `contactService.Create` lewat `SubscriptionGuardService.RequireQuota`. `crm.import_export`,
  `crm.pipeline` (hanya untuk pipeline tambahan, bukan pipeline default pertama), `crm.lead_form`
  di-enforce di service layer masing-masing lewat `RequireFeature`.
- **Role default**: `organization_owner` mendapat seluruh permission CRM (scope `organization`); `member`
  mendapat subset read/create/update operasional tanpa aksi sensitif (`delete`, `merge`, `import`, `export`,
  `view_secret`, `view_amount`, `view_price`, `approve_discount`, `close_won`, `close_lost`).
  Role `super_admin` (platform) **tidak** diberi permission CRM sama sekali — deviasi eksplisit dari pola
  modul lain yang biasanya menyertakan `super_admin` di semua seed permission.
- **Integration secret**: `crm_integrations.secret_encrypted` harus terenkripsi at-rest; endpoint GET biasa
  tidak boleh mengembalikan secret mentah kecuali permission `integration.view_secret` di-check eksplisit di
  handler. Helper enkripsi existing di `internal/platform/` (jika ada) harus dipakai — jika belum ada,
  ini dicatat sebagai risiko terpisah yang harus diselesaikan sebelum Fase 5 (Integration) di-deploy ke
  production.

## Capability Catalog & Data Model

Lihat rencana implementasi (`~/.claude/plans/eventual-skipping-penguin.md` — file kerja internal, bukan
bagian repo) untuk skema tabel `crm_*` lengkap per resource dan relasi antar tabel. Ringkasan relasi:

```
Lead --convert--> Contact + Company + Deal (opsional)
Company 1--N Contact
Pipeline 1--N PipelineStage
Deal N--1 Pipeline, N--1 PipelineStage, N--1 Company, N--1 Contact
Activity N--1 (polymorphic: Lead|Contact|Company|Deal, diverifikasi di service layer)
Quotation N--1 Deal, N--1 Contact/Company; Quotation 1--N QuotationItem
Invoice N--1 Quotation (opsional), N--1 Deal, N--1 Contact/Company; Invoice 1--N InvoiceItem
Integration -- organization-level, tidak berelasi ke entity lain
```

## Fase Implementasi

- **Fase 0** (selesai): `reference-crm.md`, update `module-map.md`, migration `000074`
  (feature `crm.lead_form`), middleware `RequireEntitlement`.
- **Fase 1** (selesai — backend; FE menyusul): Company + Contact + Lead. Migration `000075`-`000077`
  (tabel) + `000078` (permission seed). Endpoint live: CRUD + restore untuk Company/Contact, CRUD + restore +
  assign + convert untuk Lead, di bawah `/api/v1/app/crm/*` dengan gate `crm.enabled` (middleware root grup)
  dan `crm.max_contacts` (guard di `ContactService.Create` dan `LeadService.Convert`).
  **Dengan sengaja ditunda** ke fase berikutnya (bukan bagian Fase 1): endpoint `import`/`export`/`merge`
  untuk Company/Contact/Lead, endpoint `archive` untuk Company, dan permission/endpoint `customer.*` yang
  terpisah dari `contact.*` (saat ini semua operasi terhadap baris `is_customer=true` memakai permission
  `contact.*` yang sama — belum ada pemisahan otorisasi berdasarkan `lifecycle_stage`). Catat ini sebagai
  utang teknis eksplisit, bukan silently dropped.
- **Fase 2** (selesai — backend; FE menyusul): Pipeline + Deal. Migration `000079` (pipelines+stages),
  `000080` (deals + FK `crm_leads.converted_deal_id`), `000081` (permission seed). Endpoint live: CRUD +
  restore + archive + `PUT .../stages` (replace-all, diff berbasis ID) untuk Pipeline; CRUD + restore +
  move-stage + close-won + close-lost + approve-discount untuk Deal, semua di bawah `/api/v1/app/crm/*`.
  Validasi `stage_id` harus milik `pipeline_id` yang sama diterapkan di service layer
  (`ErrDealStageNotInPipeline`, HTTP 422). Field uang/desimal (`crm_deals.value`,
  `crm_pipeline_stages.probability`, `crm_deals.discount_percent`) memakai representasi **string**
  (`::text` cast di query, bukan `float64`) mengikuti konvensi modul `billing` — hindari presisi float
  untuk nilai finansial.
  **Aturan `crm.pipeline` gating**: pipeline pertama sebuah organisasi selalu boleh dibuat gratis (Deal
  butuh minimal satu pipeline untuk beroperasi); hanya pipeline ke-2 dst yang di-gate oleh feature
  `crm.pipeline` (lihat `PipelineService.Create` di `internal/modules/crm/service/pipeline_service.go`).
- **Fase 3** (selesai — backend; FE menyusul): Activity. Migration `000082` (tabel `crm_activities`),
  `000083` (permission seed — 7 permission sesuai SKILLS.md, **tanpa** action `restore`, beda dari
  Company/Contact/Lead/Pipeline). Endpoint: CRUD (tanpa restore) + complete + cancel + assign di bawah
  `/api/v1/app/crm/activities`. `related_entity_id` polymorphic (lead/contact/company/deal) tanpa FK —
  eksistensi diverifikasi di `ActivityService.validateRelatedEntity` dengan memanggil repository resource
  terkait (`404 ACTIVITY_RELATED_ENTITY_NOT_FOUND` kalau tidak ada).
- **Fase 4** (selesai — backend; FE menyusul): Quotation + Invoice CRM. Migration `000084`
  (`crm_quotations`+`crm_quotation_items`), `000085` (`crm_invoices`+`crm_invoice_items`), `000086`
  (permission seed). Endpoint: CRUD (tanpa restore, sesuai matriks) + `send`/`approve`/`reject` untuk
  Quotation; CRUD (tanpa restore) + `send`/`mark-paid`/`cancel` untuk Invoice, di bawah
  `/api/v1/app/crm/{quotations,invoices}`. Invoice bisa dibuat dari nol (items manual) **atau** disalin dari
  Quotation yang sudah ada (`quotation_id` + items kosong → copy items & totals apa adanya, tidak dihitung
  ulang).
  **Simplifikasi yang disengaja dan perlu ditinjau ulang sebelum dipakai untuk kontrak bernilai tinggi**:
  total (`subtotal`/`discount_total`/`tax_total`/`grand_total`) dihitung di Go dengan `float64` biasa
  (`QuotationService.computeTotals`/`InvoiceService.computeInvoiceTotals`), BUKAN library desimal —
  berbeda dari konvensi modul lain di CRM/`billing` yang menghindari aritmatika uang di Go sama sekali
  (hanya `::text`-cast nilai yang sudah dihitung di SQL/domain lain). `tax_total` juga input manual per
  request, bukan dihitung dari tax rate. Line item **tidak bisa diedit setelah quotation/invoice dibuat**
  (hanya field header dan transisi status) — perlu iterasi terpisah kalau mau mendukung revisi item.
  Nomor quotation/invoice (`quotation_number`/`invoice_number`) **wajib diisi klien saat create**, backend
  tidak melakukan auto-numbering per-organisasi.
  `view_price`/`view_amount`/`approve_discount` (masking angka sensitif berdasarkan permission) dari matriks
  SKILLS.md **belum diimplementasikan** — siapa pun dengan permission `read` melihat semua angka.
- **Fase 5** (selesai — backend; FE menyusul): Integration. Migration `000087` (`crm_integrations`),
  `000088` (permission seed — 7 permission, `member` hanya dapat `read`; connect/edit/lihat secret
  owner-only). Endpoint: CRUD (tanpa restore) + `connect` + `GET/PUT .../secret` (permission terpisah
  `view_secret`/`update_secret`) di `/api/v1/app/crm/integrations`.
  **Enkripsi secret**: `secret_encrypted` dienkripsi AES-256-GCM lewat helper baru
  `internal/core/crypto.EncryptSecret`/`DecryptSecret` (kunci diturunkan dari `cfg.App.Secret` via
  SHA-256 — single-key, tanpa rotasi/KMS; lihat doc comment di `secretbox.go` untuk batasannya).
  Response normal (`IntegrationResponse`) **tidak pernah** menyertakan ciphertext, hanya `has_secret`
  boolean; plaintext hanya keluar lewat endpoint `GET .../secret` yang di-guard `integration.view_secret`.
  **Status akhir entitlement granular** yang disebut di rencana awal:
  - `crm.enabled` — LIVE sejak Fase 1 (middleware root grup `/app/crm`).
  - `crm.max_contacts` — LIVE sejak Fase 1 (`ContactService.Create`, `LeadService.Convert`).
  - `crm.pipeline` — LIVE sejak Fase 2 (pipeline pertama gratis, ke-2 dst di-gate).
  - `crm.import_export` — **tetap tidak ada endpoint untuk di-gate** (import/export Company/Contact/Lead
    masih ditunda dari Fase 1, lihat bagian itu).
  - `crm.lead_form` — **tetap tidak ada endpoint untuk di-gate** (form-capture publik lintas
    modul `landing`→CRM belum digarap, di luar scope 5 fase ini — lihat "Non-Goals").
  Dua feature key terakhir sudah di-seed di database (Fase 0) tapi belum ada kode yang memanggilnya —
  ini bukan bug, hanya menunggu endpoint yang relevan dibangun.

## Status Akhir Setelah Fase 0-5 (Backend)

Seluruh 10 resource dari matriks SKILLS.md sudah punya modul backend live (`internal/modules/crm`),
tabel (`crm_*`, migration `000074`-`000088`), dan permission (~90 permission total). **Belum dikerjakan**
sama sekali (di luar scope 5 fase ini, perlu iterasi terpisah):
- ~~Frontend untuk Integration~~ — sudah selesai juga (Fase 5). **Seluruh 10 resource kini punya FE
  lengkap** (Company/Contact/Lead/Pipeline/Deal/Activity/Quotation/Invoice/Integration).
- Unit/integration test otomatis untuk seluruh modul CRM (utang teknis sejak Fase 1, belum pernah dicicil).
- `import`/`export`/`merge` untuk Company/Contact/Lead, `archive` untuk Company, permission `customer.*`
  terpisah dari `contact.*`.
- `view_price`/`view_amount`/`approve_discount` field-level masking untuk Quotation/Invoice/Deal.
- Update `backend/api/openapi.yaml` — endpoint CRM belum didokumentasikan di spec OpenAPI sama sekali.
- Drag-and-drop untuk Kanban Deal (masih dropdown "pindah stage").
- Entity picker lintas-resource untuk Activity (`related_entity_id`) dan Invoice-dari-Quotation
  (`quotation_id`) — keduanya masih input ID manual.
- Library desimal untuk perhitungan quotation/invoice (saat ini `float64` biasa).

## Lead Detail (iterasi setelah Fase 5)

Mendukung halaman detail lead 3-panel di FE (`LeadDetailPage.vue`).

- **Migration `000120`**: kolom `crm_leads.job_title` (varchar 150), `annual_revenue` (numeric(18,2),
  `CHECK >= 0`, representasi string di API seperti `crm_deals.value`), `address` (jsonb, default `{}`,
  bentuk sama dengan `crm_contacts.address`). `job_title` dan `address` ikut dibawa ke contact saat
  `LeadService.Convert`.
- **Migration `000121`**: tabel `crm_lead_attachments` (link `lead_id` → `asset_objects.id`, RLS aktif).
  File-nya disimpan lewat modul `asset` (`AssetService.UploadObject`, class `private`, label
  `crm_lead:<lead_id>`), jadi batas MIME (JPG/PNG/WEBP/PDF), 10MB per file, kuota `storage.max_bytes`, dan
  presigned download mengikuti modul asset. File lampiran juga muncul di `/admin/storage/objects`.
  Hapus lampiran = hapus object storage dulu, baru link-nya (lihat komentar
  `LeadAttachmentService.Delete`).
- **Endpoint baru** (di bawah `/api/v1/app/crm`, permission yang sudah ada — tidak ada seed baru):
  - `GET /leads/:id/attachments` (`lead.read`), `POST /leads/:id/attachments` multipart field `file`
    (`lead.update`), `GET /leads/:id/attachments/:attachmentId/download` (`lead.read`),
    `DELETE /leads/:id/attachments/:attachmentId` (`lead.update`).
  - `GET /members` (`lead.read`) — anggota **aktif** organization (`user_id`, `name`, `email`) untuk
    dropdown owner. Sengaja tidak memakai `/users` karena itu butuh `user.read` global.
- **Perubahan kontrak lead** (aditif, backward compatible): request create/update menerima `job_title`,
  `annual_revenue` (string desimal; `""` = kosongkan), `address`; response menambah `owner_name`,
  `job_title`, `annual_revenue`, `address`.
- **Validasi owner** (perubahan perilaku): `owner_user_id` pada create/update/assign lead sekarang harus
  anggota aktif organization yang sama (`ErrLeadOwnerNotMember`, HTTP 422). Sebelumnya UUID user mana pun
  diterima. `owner_name` di response juga hanya diisi untuk user yang punya membership di organization lead.
- **Belum**: Tickets dan Playbook di panel kanan (belum ada modul/tabelnya — ditunda atas keputusan user).
- **Rencana — chat WhatsApp**: tab `Timeline | WhatsApp` di kolom tengah `LeadDetailPage.vue` bergantung
  pada modul `whatsapp` (lihat `docs/reference-whatsapp.md`). Dampak ke CRM: kolom `phone_normalized` di
  `crm_leads`/`crm_contacts` (matching nomor masuk), tipe activity `whatsapp` di `crm_activities`, dan owner
  lead/contact menjadi assignee default percakapan. `crm_integrations.provider = 'whatsapp'` tetap integrasi
  generik CRM dan tidak dipakai modul `whatsapp`.

## Lead Dashboard & Riwayat Lead

Mendukung tab "Ringkasan" di halaman Leads (grafik pertumbuhan, kartu status + tren, follow-up, aktivitas
terbaru).

- **Migration `000131`**: tabel `crm_lead_events` (RLS aktif). Isinya `event_type` (`created`,
  `status_changed`, `assigned`, `converted`, `deleted`, `restored`), `from_value`/`to_value` (status, atau
  user id owner untuk `assigned`), `actor_user_id`, `created_at`. Migration ini juga menambah index
  `crm_leads(organization_id, created_at)` dan `(organization_id, converted_at)`.
  - Event ditulis `leadRepository` **di transaksi yang sama** dengan perubahan `crm_leads` (row dikunci
    `SELECT ... FOR UPDATE` dulu untuk membaca status/owner lama). Jadi riwayat tidak pernah berbeda dari
    state lead. Update yang tidak mengubah status/owner tidak mencatat event.
  - Backfill: event `created` untuk semua lead lama, dan `converted` untuk lead yang punya `converted_at`.
    Perubahan status sebelum migration ini memang tidak tercatat, jadi tren "masuk ke status X" untuk
    contacted/qualified/unqualified baru terisi setelah deploy.
- **Endpoint** `GET /leads/dashboard` (`lead.read`), query `from`, `to` (`YYYY-MM-DD`, inklusif,
  hari kalender Asia/Jakarta), dan `granularity` (`day`|`month`).
  - Default rentang 30 hari terakhir. Tanpa `granularity`: `day` untuk rentang ≤92 hari, di atas itu `month`.
  - Rentang maksimal 366 hari untuk `day` dan 731 hari untuk `month`. Input tidak valid → 422 `VALIDATION_ERROR`.
  - Periode pembanding = rentang dengan panjang sama tepat sebelum `from`.
  - Response:
    - `status_counts`: status semua lead aktif saat ini.
    - `status_entered`: jumlah lead yang masuk ke tiap status di periode ini vs periode sebelumnya.
    - `created` dan `converted` (`{current, previous}`).
    - `series`: satu entri per bucket, termasuk yang kosong.
    - `by_source`.
    - `follow_up_summary`: pending/overdue/hari ini/7 hari ke depan.
    - `upcoming_follow_ups`: maks 8. Isinya activity + `lead_name`, `company_name`, `assignee_name`.
    - `recent_activity`: maks 10. Gabungan `crm_lead_events` dan `crm_activities` pada lead.
  - Semua hitungan mengabaikan lead yang di-soft-delete. Pengecualiannya event `deleted` yang tetap
    muncul di feed.
  - Nama user (actor/assignee/owner) hanya di-resolve lewat membership organization yang sama, seperti
    aturan `owner_name`.
- **List lead** `GET /leads` (aditif, backward compatible):
  - Query baru `source` (case-insensitive), `created_from`/`created_to` (`YYYY-MM-DD`, inklusif), dan `sort`.
  - Nilai `sort`: `created_at`, `updated_at`, `contact_name`, `score`, `status` (urutan pipeline). Prefix
    `-` untuk descending. Default `-created_at`, sama dengan perilaku lama. Sort tidak dikenal → 422.
  - `per_page` sekarang dibatasi maksimal 100.
- Timestamp `crm_*` bertipe `timestamp without time zone` dan diisi `now()` di timezone DB (Asia/Jakarta).
  Karena itu bucket memakai `date_trunc` langsung tanpa konversi zona.

## Lead Playbook (SOP Penanganan Lead)

Spec: `docs/superpowers/specs/2026-09-30-lead-playbook-design.md`. Setiap lead baru (form maupun WhatsApp auto-create) otomatis mendapat langkah SOP; menyelesaikan langkah selalu mencatat *hasil* yang menentukan status lead dan langkah berikutnya. Definisi playbook di `crm_playbooks/steps/outcomes` (seed migration `000136`), eksekusi di `crm_playbook_runs` + kolom `crm_activities.playbook_*`. Logika transisi murni ada di `internal/modules/crm/playbook`; efek samping dijalankan di transaksi repository (`repository/playbook_tx.go`).

| Endpoint | Perubahan | Permission |
|---|---|---|
| `POST /activities/:id/complete` | Body opsional `{outcome_key, reschedule_at, requirements{summary,budget_estimate,target_date,decision_maker}, disqualify{reason,note}}`. Response: `{activity, lead?, next_activity?, run?}`. | `activity.complete` |
| `POST /activities/:id/cancel` | Langkah playbook ditolak (`PLAYBOOK_STEP_CANCEL_NOT_ALLOWED`). | `activity.cancel` |
| `POST /activities` | Field `status` (`pending` default \| `completed`); `note` selalu `completed`; pending non-note wajib `due_at` (`DUE_AT_REQUIRED`). | `activity.create` |
| `GET /activities`, `GET /activities/:id` | Blok `playbook` (step, attempt, channel_actions, outcomes) bila activity adalah langkah SOP. | `activity.read` |
| `GET /leads/:id/events` | Riwayat event lead, terbaru dulu (`page`, `per_page` maks 100). | `lead.read` |
| `POST /leads/:id/playbook/start` | Mulai SOP manual (`PLAYBOOK_ALREADY_ACTIVE`/`PLAYBOOK_NOT_APPLICABLE` 409, `PLAYBOOK_DISABLED` 422). | `lead.update` |
| `POST /leads/:id/disqualify` | Body `{reason, note}`; status → `unqualified`. | `lead.update` |
| `PATCH /leads/:id` | Terima 4 field kebutuhan; status `unqualified`/`converted` ditolak (`USE_DISQUALIFY_ENDPOINT`). | `lead.update` |
| `GET /leads`, `GET /leads/:id` | Field kebutuhan, `disqualify_*`, `playbook_run`. | `lead.read` |
| `GET /crm/settings`, `PATCH /crm/settings` | Toggle `lead_playbook_enabled`. | `lead.read` / `crm_settings.update` |

Aturan singkat (detail di spec §6): R1 start otomatis di repository Create; R2 satu run aktif per lead; R3 PIC = owner, fallback pembuat; R4 complete wajib hasil dan atomik; R5 langkah tidak bisa di-cancel; R6 run berakhir saat convert/disqualify/delete/status manual tak sesuai; R7 ganti owner memindahkan langkah pending; R8 "Mulai SOP" manual; R9 percobaan ke-3 tidak respon → langkah Tinjau; R10 note selalu completed; R11 jam kerja WIB Senin–Jumat 08–17; R12 toggle organisasi tidak menghentikan run aktif; R13 run menyimpan `playbook_version`.

## Convert Lead (Rilis 2)

Spec: `docs/superpowers/specs/2026-10-01-lead-deal-quotation-design.md`. Convert kini menulis company (opsional), contact, deal (opsional) dan menandai lead `converted` dalam **satu transaksi** (`repository/lead_convert.go`); gagal di langkah mana pun membatalkan semuanya. Baris lead dikunci `FOR UPDATE`, sehingga convert kedua (klik ganda / dua tab) gagal `LEAD_ALREADY_CONVERTED` tanpa membuat record tambahan. Relasi lead↔deal memakai `crm_leads.converted_deal_id` (migration `000137` menambah index-nya dan kolom `crm_deals.description`, `crm_deals.decision_maker`).

| Endpoint | Perubahan | Permission |
|---|---|---|
| `POST /leads/:id/convert` | Body `{create_company?, owner_user_id?, company?{mode: none\|existing\|new, company_id?, name?, industry?, website?, phone?}, deal?{pipeline_id, stage_id, title, value?, expected_close_date?, description?, decision_maker?, owner_user_id?}}`. Tanpa `company`/`deal` = perilaku lama. Response `{lead, contact, company?, deal?}`. | `lead.convert` (+ `deal.create` bila ada `deal`; + `company.create` bila company baru) |
| `POST /leads/:id/deal` | Buat deal untuk lead yang sudah converted tanpa deal. Body = objek `deal` di atas. Response `{lead, deal}`. | `deal.create` |
| `GET /companies/lookup?name=` | Maks 5 company yang namanya mirip (normalisasi: huruf kecil, tanpa PT/CV/Tbk/UD), untuk cegah duplikat. | `company.read` |
| `GET /deals/:id` | Field deal di level atas (kompatibel) + `pipeline` + `source_lead {id, contact_name} \| null`. | `deal.read` |

Validasi deal: pipeline milik organisasi dan tidak diarsipkan, stage milik pipeline itu dan bukan Won/Lost, judul wajib (maks 200), `value` desimal `^\d{1,16}(\.\d{1,2})?$` (kosong = `0`), `expected_close_date` `YYYY-MM-DD`. Owner deal default ke owner convert. Kebutuhan yang diedit saat convert disimpan ke deal saja; field kebutuhan di lead tidak diubah.

Kode error: `422 INVALID_PIPELINE_STAGE`, `422 INVALID_START_STAGE`, `422 VALIDATION_ERROR` (input deal/company), `409 LEAD_ALREADY_CONVERTED`, `409 LEAD_NOT_CONVERTED`, `409 LEAD_DEAL_EXISTS`, `403 FORBIDDEN` (deal tanpa `deal.create`, company baru tanpa `company.create`).

## Non-Goals

- Tidak menggantikan atau berinteraksi langsung dengan `billing_invoices`/`billing_payments` (modul
  `billing` platform) — dua sistem invoice terpisah total.
- Tidak menyediakan Point of Sales (POS) atau Membership/Loyalty — itu modul roadmap terpisah di
  `SKILLS.md`.
- Sinkronisasi lead capture dari `landing` (form builder) ke CRM adalah integrasi lintas modul via
  event/adapter di fase mendatang, bukan bagian dari 5 fase di atas — tidak memindah ownership data lead
  capture dari modul `landing`.
