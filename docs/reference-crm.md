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
- Quotation tenant-ke-customer. (Invoice tenant-ke-customer dulu di sini; sejak Rilis 3 S3 ada di modul `receivable`.)
- Integration: koneksi CRM ke sistem eksternal (webhook, WhatsApp, form capture, dsb).
- Entitlement/quota enforcement per plan tenant.

## Architecture Decision

Mengikuti pola modul `landing` (satu-satunya modul existing dengan RLS penuh dan banyak sub-entity dalam
satu bounded module):

```txt
internal/modules/crm/
├── domain/
│   ├── company.go, contact.go, lead.go, pipeline.go, deal.go
│   ├── activity.go, quotation.go, integration.go
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

### Invoice tenant → customer sudah pindah ke modul `receivable`

`crm_invoices` / `crm_invoice_items` **dihapus** (Rilis 3 S3, migration `000145`); penagihan tenant ke pelanggannya
sekarang ada di modul `receivable` (`receivable_*`, route `/api/v1/app/receivable/*`, lihat
[reference-receivable.md](reference-receivable.md)) dan bisa dipakai tenant tanpa CRM. `billing_invoices` tetap
tagihan **platform Zyad ke tenant** (modul `billing`). Sebelum menjalankan `000145` di produksi:
`SELECT count(*) FROM crm_invoices;` — bila > 0, ekspor dulu. Permission `invoice.*` kini bermodul `receivable`.

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
- **Fase 4** (selesai — backend; FE menyusul): Quotation + Invoice CRM. **[Bagian Invoice dihapus di Rilis 3 S3 — migration `000145`; lihat modul `receivable`. Paragraf ini dipertahankan sebagai riwayat.]** Migration `000084`
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

## Atribut harga quotation (Rilis 3 S1)

Migration `000142`: `crm_quotation_items.charge_type`, `billing_frequency`, `payment_timing` (CHECK sama dengan katalog) dan
`crm_quotations.one_time_total`, `first_invoice_total`, `recurring_totals jsonb`. Backfill: quotation lama `one_time_total = first_invoice_total = grand_total`.

- **Input item** (`POST/PATCH /quotations`): `charge_type?`, `billing_frequency?`, `payment_timing?`. Kosong semuanya → ikut katalog bila ada `product_id`, selain itu `one_time` + `prepaid`. Nilai yang dikirim di baris mengalahkan katalog (sama dengan harga/pajak). Kombinasi tidak valid → `422 VALIDATION_ERROR`, item tidak tersimpan.
- **Revisi** menyalin atribut tiap baris apa adanya (tidak membaca katalog).
- **Rincian total** (dihitung `priceQuotationLines`, total baris = `line_total + tax_amount`, setelah diskon): `one_time_total` = Σ baris `one_time`; `recurring_totals[frekuensi]` = Σ baris `recurring` per frekuensi; `first_invoice_total` = Σ semua baris `prepaid` (sekali bayar maupun periode pertama berulang). `grand_total` tidak berubah maknanya.
- **Respons**: item + `charge_type`, `billing_frequency` (`null` untuk one_time), `payment_timing`; quotation + `one_time_total`, `first_invoice_total`, `recurring_totals` (`{"monthly":"333000.00"}`, objek kosong bila tidak ada baris berulang).
- **PDF / ringkasan WA & email**: harga baris berulang diberi akhiran (`/bulan`, …), baris atribut kecil di bawah deskripsi (kecuali Sekali bayar · Prabayar), dan blok "Sekali bayar / Berulang (frekuensi) / Tagihan pertama" **hanya bila** ada baris berulang — dokumen lama tidak berubah.

## Quotation v2 (Rilis 2 S3)

Migration `000140` menambah `crm_quotations.revision_of_id`, `revision_no`, `pdf_asset_id`, `pdf_generated_at`, status `superseded`, serta snapshot item `crm_quotation_items.product_id` (FK ke `catalog_products`, `ON DELETE SET NULL (product_id)`), `sku`, `unit`, `tax_percent`, `tax_amount`.

**Aturan**

- **Dari deal.** `POST /quotations` menerima `deal_id`; contact, company, dan mata uang diisi dari deal bila kosong (deal tidak ada → `422 QUOTATION_DEAL_NOT_FOUND`).
- **Snapshot katalog (R4).** Item dengan `product_id` diisi nama, SKU, satuan, harga, dan pajak dari katalog sebagai nilai awal; nilai yang dikirim klien (deskripsi/harga/pajak/satuan) mengalahkan katalog. Produk nonaktif/terhapus → `422 PRODUCT_INACTIVE`. Perubahan katalog tidak mengubah item yang sudah tersimpan; revisi menyalin snapshot apa adanya (tidak membaca katalog).
- **Kalkulasi (R5).** `math/big.Rat`, half-up 2 desimal per baris: `gross = qty × harga`, `disc = gross × d%`, `net = gross − disc` (= `line_total`), `tax = net × t%`. Header: `subtotal = Σgross`, `discount_total = Σdisc`, `tax_total = Σtax`, `grand_total = subtotal − discount_total + tax_total`. Total dari klien diabaikan. Kontrak lama (`tax_total` header tanpa pajak per baris) tetap diterima saat create.
- **Penguncian (R6).** Hanya `draft` yang bisa diubah (`PATCH`, termasuk `items` yang mengganti seluruh item) atau dihapus; selain itu `409 QUOTATION_LOCKED`. Baris dikunci `FOR UPDATE`, jadi edit dari tab lama setelah quotation terkirim tetap ditolak.
- **Revisi (R7).** `POST /quotations/:id/revise` hanya dari `sent`/`rejected`/`expired` (`409 QUOTATION_NOT_REVISABLE`). Satu transaksi: versi lama → `superseded`, draft baru `{nomor akar}-R{n}` dengan `revision_no = n` (nomor akar = nomor tanpa akhiran `-R<n>`, jadi R2 dari R1 menjadi `QUO-…-R2`).
- **PDF.** `GET /quotations/:id/pdf`: draft dirender on-demand dengan watermark DRAFT; status lain dilayani dari snapshot final (asset privat kelas `private`, label `quotation`) yang dibuat sekali saat pertama ditandai terkirim, atau saat PDF pertama diminta untuk quotation lama. Tidak ada URL publik; stream lewat backend dengan `Cache-Control: no-store`. Renderer: `internal/modules/crm/quotationpdf` (`codeberg.org/go-pdf/fpdf` + Noto Sans ter-embed, lisensi OFL di `fonts/OFL.txt`). Penerbit = `company_name`/`contact` branding default tenant, fallback nama organisasi. Gagal render/simpan → `502 QUOTATION_PDF_FAILED`.
- **Tandai terkirim.** `POST /quotations/:id/send` tanpa body atau `{"channel":"manual"}`: buat snapshot PDF, status `sent`, dan activity *note completed* "Penawaran … ditandai terkirim (manual)" pada deal. Channel `email`/`whatsapp` disiapkan untuk S4 (`422 CHANNEL_NOT_SUPPORTED` sampai tersedia).
- **Approve/Reject (R4b/R10).** Hanya dari `sent`. Respons menambah `suggest_deal_status` (`won` bila terhubung deal); deal tidak diubah otomatis.
- **Expiry (R11).** Dihitung *lazy* per tenant saat `GET`/`List` quotation: `sent` dengan `valid_until` < hari ini (Asia/Jakarta) → `expired`. Tidak ada job lintas tenant (tabel CRM ber-RLS).

## Kirim Penawaran (Rilis 2 S4)

Migration `000141` membuat `crm_quotation_sends` (log setiap percobaan kirim; RLS; unik `(organization_id, quotation_id, client_request_id)`).

| Endpoint | Perilaku | Permission |
|---|---|---|
| `POST /quotations/:id/send` | Tanpa body / `{"channel":"manual"}` = tandai terkirim manual (S3). `{"channel":"email"\|"whatsapp", "mode":"text"\|"pdf"\|"text_pdf", "client_request_id", "recipient"?, "message"?, "mailbox_id"?, "wa_session_id"?}` = kirim lewat kanal; respons `{quotation, send}`. | `quotation.send` + `email.send` (email) / `whatsapp.message.send` (WhatsApp) |
| `GET /quotations/:id/summary?message=` | Preview ringkasan `{subject, text, html}` persis seperti yang dikirim. | `quotation.read` |
| `GET /quotations/:id/sends` | Riwayat kiriman, terbaru dulu. | `quotation.read` |

Aturan (R8–R9):

- Hanya `draft`/`sent` yang bisa dikirim (`409 QUOTATION_NOT_SENDABLE`); `sent` boleh dikirim ulang atau ke kanal lain.
- Penerima: email = `recipient` atau email kontak quotation (divalidasi); WhatsApp = selalu nomor kontak (percakapan terikat ke kontak).
- Kanal tidak bisa dipakai (kontak tanpa email/nomor valid, tidak ada mailbox aktif, tidak ada session WA tersambung, WhatsApp belum aktif/dikonfigurasi) → `422 CHANNEL_UNAVAILABLE` dengan alasan berbahasa Indonesia; **tidak dicatat** dan status tidak berubah.
- Mode yang butuh PDF memakai snapshot final (`ensureSnapshot`, S3). Email: lampiran PDF, body HTML ringkasan (teks pengguna di-escape), lewat mailbox milik pengirim (`RelatedEntityType=contact`), dianggap sukses saat masuk antrean mailbox. WhatsApp: teks ≤ 1.000 karakter jadi caption dokumen, lebih panjang dikirim terpisah; sukses saat WAHA menerima.
- Setiap percobaan dicatat `sent`/`failed` + alasan. Kiriman sukses pertama mengubah `draft → sent` dan menulis activity completed (`email`/`whatsapp`) "Penawaran … dikirim via …" pada deal. Kiriman gagal tidak mengubah status.
- Idempotensi: `client_request_id` yang sama mengembalikan hasil pertama tanpa mengirim lagi.
- Kode modul ringkasan: `crm/quotationpdf/summary.go` (view-model yang sama dengan PDF). Adapter kanal: `mailbox/service/quotation_mailer.go`, `whatsapp/service/quotation_sender.go` (CRM hanya mendefinisikan interface di `crm/service/quotation_channels.go`).

## Link penawaran & respons customer (Rilis 3 S2)

Migration `000143`: tabel bersama `public_links` (directory token → dokumen, **tanpa RLS**, pola `mailbox_directory`; dipakai juga invoice di S3), `crm_quotation_responses` (RLS), status `revision_requested`, tipe aktivitas `quotation_response` + kolom `crm_activities.metadata jsonb`, dan template notifikasi `crm.quotation_approved` / `crm.quotation_revision_requested`. Paket: `internal/shared/publiclink`.

| Endpoint | Perilaku | Akses |
|---|---|---|
| `GET /public/quotations/:token` | Isi penawaran + `state` (`active`/`decided`/`expired`/`superseded`) + `last_response` | token |
| `GET /public/quotations/:token/pdf` | PDF inline (`no-store`, `noindex`) | token |
| `POST /public/quotations/:token/approve` | `{responder_name, agree:true}` | token |
| `POST /public/quotations/:token/revision` | `{responder_name, categories[], note?}` | token |
| `POST /quotations/:id/link` | URL publik quotation `sent` (idempoten) | `quotation.send` |
| `GET /quotations/:id/responses` | Riwayat respons customer | `quotation.read` |

Aturan (R3–R5):

- **Token.** 32 byte `crypto/rand`, base64url (43 karakter). DB menyimpan `sha256(token)` untuk pencarian dan token terenkripsi (`corecrypto.EncryptSecret`, `APP_SECRET`) agar URL yang sama bisa disisipkan di kiriman berikutnya. Token tidak ditulis ke log (`middleware.AccessLogger` me-mask path `/public/quotations/*`). Satu link aktif per quotation; revisi mencabutnya.
- **Scope tenant.** Dari token → `organization_id` → `WorkerResolver` (identitas `public-document-link`) → query CRM biasa di bawah RLS. Endpoint publik tidak menerima ID selain token.
- **Masa berlaku.** `valid_until` 23:59:59 WIB, atau 30 hari sejak dibuat bila kosong. Lewat masa berlaku ≤ 30 hari → `state: expired` (tombol hilang, POST `409`); lebih lama, token tak dikenal, atau dicabut bukan karena revisi → `404 LINK_INVALID` (pesan sama). Dicabut karena revisi → `state: superseded` (PDF versi lama tetap bisa dilihat).
- **Respons.** Hanya dari `sent`; `UPDATE … WHERE status = 'sent'` + insert respons dalam satu transaksi, jadi klik ganda / dua tab → satu respons, yang kedua `409 QUOTATION_NOT_RESPONDABLE`. Kategori revisi: `price`, `quantity`, `items`, `specification`, `schedule`, `payment_terms`, `validity`, `other` (catatan wajib bila `other`, maks 2.000 karakter). Catatan disimpan apa adanya dan wajib ditampilkan sebagai teks.
- **Efek samping** (kegagalannya dicatat, tidak membatalkan respons): aktivitas `quotation_response` pada deal (metadata `action`, `categories`, `note`, …), email ke pemilik deal (pembuat quotation bila tanpa deal; email notifikasi `text/plain`), dan `QuotationApprovedHook` (sengaja `nil` di S2; S4 mengisinya). Approve internal oleh sales memanggil hook yang sama.
- **Status.** `revision_requested` boleh di-Reject atau di-Revise oleh sales; tidak bisa di-Approve internal.
- **Rate limit.** 30 request/menit per IP (semua endpoint publik) dan 10 aksi POST/jam per token → `429 RATE_LIMITED`. Redis mati → fail open.
- **Catatan operasional.** Token juga muncul di path halaman frontend `/q/<token>`; log akses nginx perlu me-mask atau tidak menyimpan path tersebut.

## Sales Order & aturan Won (Rilis 3 S4)

Quotation yang di-approve (customer lewat link atau sales) memanggil `QuotationApprovedHook` → `SalesOrderService.QuotationApproved`
membuat **SO draft** (`SO-YYYY-NNNN`, idempoten per quotation; aktivitas `order` + notifikasi `crm.sales_order_created` ke PIC).
`suggest_deal_status` pada approve kini selalu kosong.

- **Status:** `draft → confirmed → completed`, `draft → cancelled`. `billing_status`: `none · pending · done · failed`.
- **Syarat Konfirmasi:** `start_date` ≥ hari ini (WIB) − 30 hari; `bill_to_name` 2–200; minimal satu kanal; email butuh
  `bill_to_email` valid; WhatsApp butuh kontak CRM **dan** `bill_to_phone`. Gagal → 422 `SALES_ORDER_INCOMPLETE` `{data.fields}`.
- **Konfirmasi** (`MarkConfirmed` kondisional; tab ganda → 409 `SALES_ORDER_NOT_DRAFT`) lalu `receivable.OrderBilling.BillOrder`:
  invoice awal (baris prabayar, idempotency key `initial`) + Contract (baris berulang). Kegagalan penagihan **bukan error HTTP**:
  SO `confirmed` dengan `billing_status=failed` dan `billing_error` ramah; `POST /:id/retry-billing` melanjutkan tanpa dobel.
- **Konfirmasi diterima** (`POST /:id/deliveries`, `batch_key` wajib): baris `one_time+postpaid` → invoice per batch (idempoten).
- **Aturan Won (R18)** — `WonEvaluator`: per baris SO `confirmed/completed` pada deal `open`: `one_time+prepaid` & `recurring+prepaid` →
  invoice awal `paid`; `recurring+postpaid` → contract ada; `one_time+postpaid` → `delivered`. SO tanpa baris tidak memicu Won; deal
  `won/lost` tidak disentuh. Dipicu: konfirmasi SO, konfirmasi diterima, listener receivable (`InvoicePaid` sumber `sales_order`,
  `ContractCreated`), dan `GET /deals/:id/won-checklist` (pemulihan bila listener gagal; pembayaran tetap tercatat).
- **Endpoint** (`/app/crm`): `sales-orders` (list/get/PATCH draft), `:id/confirm|retry-billing|cancel|deliveries`,
  `deals/:id/sales-orders`, `deals/:id/won-checklist`. Permission: `sales_order.read|manage|confirm` (member hanya `read`).
- Galat: `SALES_ORDER_NOT_DRAFT`, `DELIVERY_NOT_PENDING`, `SALES_ORDER_NOT_CONFIRMED`, `BILLING_NOT_RETRYABLE` 409;
  `SALES_ORDER_INCOMPLETE`, `VALIDATION_ERROR` 422; `BILLING_FAILED` 502; `SALES_ORDER_NOT_FOUND` 404.

## Self-serve checkout (Rilis 4 S2)

Pembeli (tenant customer) memilih produk di pricing page → `POST /api/v1/app/self-serve/checkout {product_id}` →
`{invoice_url, deal_id}`. Handler berjalan di konteks tenant pembeli (`RequireActiveTenant` + `RequireCustomerTenant` +
`organization.billing.manage`); `SelfServeService` bekerja di scope **org platform**.

- **Rantai** (cari-dulu-baru-buat, konsistensi lewat indeks unik + advisory lock `self_serve:<workspace_id>`, tanpa tabel status):
  company (tertaut workspace, `crm_companies.tenant_organization_id`, dibuat atomik dengan convert) → contact → lead `landing_page`
  tanpa playbook → deal di pipeline **Self-Serve** (`crm_pipelines.system_key='self_serve'`) → quote `channel='self_serve'`
  (diterima online: `AcceptOnline`) → SO draft→confirmed (`UpdateDraft` + `Confirm`, PIC bot) → contract + invoice awal →
  `InvoiceService.Link`. Stage deal diambil lewat **posisi** (0 "Checkout dimulai", 1 "Menunggu pembayaran"), bukan nama.
  Deal pindah ke Won otomatis lewat evaluator R3 saat invoice lunas.
- **Idempoten & resume:** klik ulang untuk produk sama mengembalikan link yang sama; proses yang terhenti di tengah dilanjutkan
  (kegagalan langkah → 502 `SELF_SERVE_STEP_FAILED`, `data.step` ∈ company|lead|deal|quotation|sales_order|invoice).
  Checkout paralel untuk workspace sama → 409 `SELF_SERVE_IN_PROGRESS`.
- **Ganti pilihan (K14):** produk lain sebelum bayar → validasi produk baru dulu, lalu `SalesOrderService.CancelUnpaid`
  (void invoice → akhiri contract → SO cancelled) dan deal lama Lost "Ganti pilihan ke <SKU>". Sudah ada pembayaran →
  409 `SELF_SERVE_ALREADY_SUBSCRIBED`, tidak ada yang diubah.
- **Produk layak (R1):** publik, aktif, langganan (`recurring`) prabayar, harga > 0, ≥ 1 fitur; selain itu 422
  `SELF_SERVE_PRODUCT_UNAVAILABLE`. Workspace yang sudah punya SO berkontrak aktif memuat produk berfitur → 409
  `SELF_SERVE_ALREADY_SUBSCRIBED` (K13).
- **Tidak ada penggabungan otomatis:** company tak tertaut dengan nama sama dan contact dengan email sama milik company lain
  tidak diambil alih; company+contact baru dibuat. Company tertaut dipakai ulang (+ contact berdasarkan email di company itu).
- **Aktor bot:** user `SELF_SERVE_BOT_EMAIL` (default `self-serve-bot@zyad.cloud`, seed `cmd/seed -name self-serve`, anggota
  aktif org platform) menjadi `created_by`, PIC SO, dan owner deal (kecuali `SELF_SERVE_DEAL_OWNER_EMAIL` diisi; owner itu
  wajib anggota aktif org platform). Konfigurasi di-resolve lazily; belum di-seed → 503 `SELF_SERVE_NOT_CONFIGURED`.
- **Endpoint lama** `POST /app/billing/upgrade`, `/app/billing/invoices/:id/checkout`, `.../checkout/sync` → 410 `ENDPOINT_RETIRED`.
- **Checklist deploy:** (1) migration `000151`; (2) `go run ./cmd/seed -name self-serve`; (3) env `SELF_SERVE_*`;
  (4) `receivable_settings.default_sender` org platform terisi (PIC bot tidak punya mailbox, pengiriman invoice butuh pengirim
  default); (5) **produksi: S2 dan S3 harus naik bersamaan** — tanpa S3, pembeli membayar tetapi belum mendapat fitur.

## Tautan workspace & akses dari contract (Rilis 4 S3)

`crm_companies.tenant_organization_id` menautkan company ke workspace pelanggan. Dari tautan inilah fitur produk yang dibeli
menjadi entitlement workspace.

- **API:** `PATCH /app/crm/companies/:id` menerima `tenant_organization_id` (UUID = tautkan/ganti, `null` = lepas, tidak dikirim =
  tidak diubah). Syarat: org pemanggil = **platform** (selain itu 422 `WORKSPACE_LINK_PLATFORM_ONLY`), permission
  `company.link_workspace` (403; `organization_owner` & `super_admin`), workspace `type=customer` (422 `WORKSPACE_NOT_CUSTOMER` /
  `WORKSPACE_NOT_FOUND`), belum tertaut ke company lain (409 `WORKSPACE_ALREADY_LINKED`). Respons company membawa
  `tenant_organization {id,name,slug,status} | null` (hanya untuk org platform).
- **Sinkronisasi:** setelah tautan berubah, `TenantAccess.OnWorkspaceLinkChanged` mencabut entitlement contract company itu dari
  workspace lama (yang kembali ke paket gratis bila tak punya contract lain) lalu memberikannya ke workspace baru sesuai status bayar.
- **Rantai fitur:** baris quote (snapshot S1) → item SO (`crm_sales_order_items.features`) → `OrderLine.Features` → item contract
  (`receivable_contract_items.features`). Migration `000152`.
- **Kapan diberikan (`TenantAccess.SyncContract`, dipicu `InvoicePaid` ber-`ContractID`, `ContractCreated`, `ContractEnded`,
  perubahan tautan):** item **prabayar** setelah ada invoice contract yang lunas; item **pascabayar** sejak contract terbentuk.
  Fitur seluruh item digabung per contract (angka/desimal → terbesar, boolean → `true` menang, string → item `position` terkecil).
  Contract `ended`/`cancelled` → dicabut. Company tanpa tautan workspace tidak mengubah apa pun.
- **Penyimpanan:** `organization_entitlements` `source='contract'`, `source_reference=<contract_id>`; dicabut dengan
  `status='expired'` + `effective_until` (riwayat tetap ada). Prioritas `FindEffective`/`ListEffective`:
  `platform_override`(5) > `addon`(4) > `contract`(3) > `trial`(2) > `plan`(1) > `default`(0). Baris `plan` lama dari
  `customer_subscriptions` dibiarkan sampai S5.
- **Paket gratis:** `source='default'` dari fitur produk platform ber-SKU `SELF_SERVE_FREE_PRODUCT_SKU` (default `FREE`), diberikan
  saat onboarding workspace dan saat contract terakhir berakhir; dicabut begitu ada contract aktif. Produk belum ada →
  `DEFAULT_PRODUCT_NOT_FOUND` dicatat di log (onboarding tetap sukses); membuat produknya lalu memanggil ulang provisioner memperbaiki.
- **Guard:** `RequireFeature`/`RequireQuota` tidak lagi membaca `customer_subscriptions`; hanya entitlement (dan bypass org platform).

## Non-Goals

- Tidak menggantikan atau berinteraksi langsung dengan `billing_invoices`/`billing_payments` (modul
  `billing` platform) — dua sistem invoice terpisah total.
- Tidak menyediakan Point of Sales (POS) atau Membership/Loyalty — itu modul roadmap terpisah di
  `SKILLS.md`.
- Sinkronisasi lead capture dari `landing` (form builder) ke CRM adalah integrasi lintas modul via
  event/adapter di fase mendatang, bukan bagian dari 5 fase di atas — tidak memindah ownership data lead
  capture dari modul `landing`.
