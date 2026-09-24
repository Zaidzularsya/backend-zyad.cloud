# WhatsApp Channel (WAHA) Module Reference

Dokumen ini menyimpan source requirement, keputusan arsitektur, batas tanggung jawab, dan fakta WAHA yang
sudah diverifikasi untuk modul `whatsapp` sebelum implementasi dimulai — mengikuti checklist
`docs/module-map.md` bagian "Modul Stub — Checklist Sebelum Mulai Menggarap".

## Source Requirement

- "Prompting Plan — WhatsApp Channel (WAHA) untuk Platform & Tenant" (disusun 2026-09-24, keputusan
  D1–D7) + rencana end-to-end yang disetujui user 2026-09-24 (koreksi K1–K12, lihat bagian
  [Koreksi terhadap Prompting Plan](#koreksi-terhadap-prompting-plan)).
- Keputusan eksplisit user (2026-09-24):
  - Kuota `whatsapp.max_messages_per_month` **ditunda** — MVP cukup rate limit per menit.
  - Chat di Lead Detail = tab `Timeline | WhatsApp` di **kolom tengah** `LeadDetailPage.vue`.
  - `crm_integrations` provider `whatsapp` dibiarkan (lihat [Naming Conflict](#naming-conflict)).
- Entitlement yang sudah ada: `whatsapp.enabled`, `whatsapp.max_messages_per_month`
  (`migrations/000061_seed_product_catalog.up.sql`).

## Objective

Platform dan setiap tenant (organization tipe `customer`, plus organisasi platform) bisa:

- Membuat satu atau lebih session WhatsApp (dibatasi entitlement `whatsapp.max_sessions`).
- Pairing nomor via **QR** atau **kode pairing**.
- Menerima dan mengirim pesan **teks** dari session tersebut.
- Chat follow-up lead langsung dari halaman Lead Detail; nomor masuk dicocokkan ke lead/contact.
- (Fase lanjutan) memakai session platform untuk notifikasi, inbox, media, template, otomatisasi.

## Decision Summary (D1–D7)

| # | Keputusan |
|---|---|
| D1 | Session boleh >1 per organisasi, dibatasi `whatsapp.max_sessions` |
| D2 | Satu nomor dipakai bersama; percakapan di-assign ke **owner lead/contact**, bisa di-reassign |
| D3 | MVP: pairing (QR + kode) → terima & kirim teks → panel chat Lead Detail → pencocokan nomor ke lead |
| D4 | Satu instance WAHA bersama; backend satu-satunya klien; API key WAHA tidak pernah keluar ke FE |
| D5 | Modul baru `internal/modules/whatsapp` (bukan di dalam `crm`) |
| D6 | MVP polling di FE; SSE ditunda |
| D7 | Engine GOWS/NOWEB (bukan WEBJS) + storage session persisten |

## Architecture Decision

```txt
internal/platform/whatsapp/        # adapter teknis: Client (SendText) + WAHA HTTP client + HMAC verify
internal/modules/whatsapp/
├── domain/                        # Session, Conversation, Message, WebhookEvent, status/ack enum, feature key
├── repository/                    # withTx(ctx, scope, fn) — pola crm/repository/lead_repository.go
├── service/                       # SessionService, ConversationService, MessageService, InboundProcessor
├── handler/                       # session_handler, conversation_handler, webhook_handler
├── dto/
└── errors.go
internal/shared/phone/             # NormalizeID — dipakai crm (lead/contact) dan whatsapp
```

- Tidak ada `module.go`/`routes.go`: handler punya `RegisterRoutes(group, permissionChecker)`, wiring di
  `internal/app/app.go` → `dependency.go` → `router.go` (pola CRM).
- **Kenapa modul terpisah, bukan di `crm_integrations`:** WhatsApp channel punya lifecycle sendiri (session,
  pairing, webhook inbound, status pesan) dan konsumennya lintas modul (CRM, notification platform, landing
  CTA, inbox). `crm_integrations` adalah konfigurasi integrasi outbound generik per provider; menaruh session
  dan pesan di sana akan membuat CRM bergantung pada detail WAHA dan menghalangi pemakaian non-CRM.
- `internal/platform/whatsapp` diperluas (bukan paket `waha` baru) karena interface `Client` dan
  `NoopClient` di sana sudah dipakai `WhatsAppDispatcher` notification. Pemilihan provider lewat
  `WHATSAPP_PROVIDER=waha`; default tetap `noop`.
- Background work (webhook processor, reconcile status) berjalan di `cmd/worker` sebagai loop terpisah
  dengan identity `whatsapp-worker` di `WorkerResolver` — pola `notification/consumer/outbox_worker.go`.

## Naming Conflict

- **`crm_integrations.provider = 'whatsapp'`** (`migrations/000087`) adalah integrasi outbound/webhook
  generik milik CRM. **Bukan** WAHA channel. Tidak diubah; di FE halaman Integrations cukup diberi
  catatan/link ke halaman WhatsApp yang baru.
- **Session WAHA** (resource WAHA, nama global di server) ≠ **`wa_sessions`** (record tenant). Relasinya lewat
  `wa_session_directory.session_name`.
- **`whatsapp.max_messages_per_month`** (sudah ada, belum ditegakkan) ≠ rate limit kirim per menit (MVP).

## Tenant Boundary

- Semua tabel tenant `wa_sessions`, `wa_conversations`, `wa_messages` punya `organization_id NOT NULL` dan
  `apply_organization_rls()`; repository memakai `withTx` yang men-set `app.organization_id`.
- Dua tabel **sengaja non-RLS** (analog `notification_outbox_events`):
  - `wa_session_directory` — pemetaan `session_name → organization_id, session_id`. Diperlukan karena
    webhook datang tanpa konteks tenant dan runtime role tidak punya BYPASSRLS.
  - `wa_webhook_events` — inbox mentah untuk idempotensi & reprocess.
  Kedua tabel ini hanya diakses webhook handler dan worker; tidak pernah diekspos lewat API tenant.
- Organisasi **selalu** di-resolve dari `wa_session_directory` (bukan dari `metadata` di payload, bukan dari
  parsing nama session), lalu diverifikasi aktif lewat `WorkerResolver`.
- Grup route `/api/v1/app/whatsapp` memakai `RequireActiveTenant`, `RequireCustomerOrPlatformTenant`,
  `RequireEntitlement(..., "whatsapp.enabled")`. Organisasi platform bypass entitlement/kuota.
- Backend **hanya** mengelola session yang ada di `wa_session_directory`. Session lain di server WAHA
  (lihat WAHA Facts: `session_yulianto`, `session_zyad` milik n8n) tidak boleh disentuh, termasuk oleh job
  reconcile.

## Security Baseline

- **API key WAHA**: hanya di env backend (`WHATSAPP_API_KEY`), dikirim sebagai header `X-Api-Key`. Tidak pernah
  muncul di response, log, atau error message yang diteruskan ke klien.
- **HMAC webhook**: tiap session dibuat dengan `config.webhooks[].hmac.key = WHATSAPP_WEBHOOK_HMAC_KEY`.
  Webhook handler memverifikasi `X-Webhook-Hmac` (HMAC-SHA512 atas raw body, `hmac.Equal`) dan menolak
  `401` bila salah; `503` bila key belum dikonfigurasi (pola `doku_webhook_handler.go`).
- **Nama session** dibangkitkan backend; tidak ada endpoint yang menerima nama session WAHA mentah dari klien.
  Semua endpoint memakai `wa_sessions.id` (UUID) yang dicek RLS.
- **Permission** (module `whatsapp`):

  | Permission | organization_owner | member | super_admin |
  |---|---|---|---|
  | `whatsapp.session.read` | ✓ | ✓ | ✓ |
  | `whatsapp.session.manage` | ✓ | – | ✓ |
  | `whatsapp.conversation.read` (milik sendiri) | ✓ | ✓ | ✓ |
  | `whatsapp.conversation.read_all` | ✓ | – | ✓ |
  | `whatsapp.conversation.assign` | ✓ | – | ✓ |
  | `whatsapp.message.send` | ✓ | ✓ | ✓ |

  Tanpa `read_all`, list/detail conversation difilter `assignee_user_id = user`.
- **Entitlement**: `whatsapp.enabled` (sudah ada; free/starter off, growth+ on), `whatsapp.max_sessions`
  (`000128`: growth=1, business=3, enterprise=10; dicek via `SubscriptionGuardService.RequireQuotaValue`).
  Organisasi yang paketnya di-sync sebelum `000128` belum punya baris runtime `whatsapp.max_sessions` →
  SessionService memakai fallback batas 1 (`domain.DefaultMaxSessionsFallback`) sampai sync paket berikutnya.
- **Rate limit** kirim per session di Redis (`WHATSAPP_SEND_RATE_PER_MINUTE`), `429` bila terlampaui;
  fail-open (log warn) bila Redis tidak tersedia.
- **Data pribadi** (UU PDP): isi chat adalah data pribadi pihak ketiga. `raw jsonb` di `wa_messages` hanya
  menyimpan field yang dibutuhkan (bukan `_data` mentah engine). Kebijakan retensi → Fase lanjutan.

## Session Naming

- Format: `zc_<12 hex pertama organization_id tanpa strip>_<6 char random [a-z0-9]>`, contoh
  `zc_3f2a9c1b7d4e_k8m2qa`. Hanya `[a-z0-9_]` — aman untuk path URL WAHA.
- Prefix `zc_` memisahkan session milik aplikasi dari session manual di server yang sama.
- Keunikan dijamin `UNIQUE` di `wa_session_directory.session_name` dan `wa_sessions.name`; bila bentrok,
  generate ulang.
- `config.metadata` WAHA diisi `{"zyad.organization_id": ..., "zyad.session_id": ...}` **hanya untuk
  debugging**, tidak dipakai untuk otorisasi.

## Data Model (ringkas)

| Tabel | RLS | Isi utama |
|---|---|---|
| `wa_sessions` | ✓ | name (unik global), display_name, phone, push_name, status, engine, waha_server_id, is_default, purpose (`cs`/`sales`/`notification`), auto_create_lead, last_status_at, audit, deleted_at |
| `wa_session_directory` | ✗ | session_name (PK), organization_id, session_id, deleted_at |
| `wa_conversations` | ✓ | session_id, chat_id, phone_normalized, contact_name, related_entity_type/id, assignee_user_id, last_message_at, last_message_preview, unread_count, status (`open`/`closed`); `UNIQUE(session_id, chat_id)` |
| `wa_messages` | ✓ | conversation_id, waha_message_id, direction (`in`/`out`), body, status (`pending`/`sent`/`delivered`/`read`/`failed`), error, sent_by_user_id, sent_at, raw |
| `wa_webhook_events` | ✗ | event_id (unik, ULID dari WAHA), session_name, event_type, payload, attempts, error, processed_at, next_retry_at |
| `crm_leads`/`crm_contacts` | (existing) | + `phone_normalized` (generated dari `phone`) + partial index `(organization_id, phone_normalized)` |
| `crm_activities` | (existing) | type check + `'whatsapp'` |

Pemetaan `ack` WAHA → status pesan: `-1 ERROR → failed`, `0 PENDING → pending`, `1 SERVER → sent`,
`2 DEVICE → delivered`, `3 READ`/`4 PLAYED → read`. Status tidak boleh mundur (mis. `read` → `delivered`
diabaikan).

## Phone Normalization

Satu aturan, dua implementasi yang diuji sama (`internal/shared/phone/phone_integration_test.go`):

- SQL `normalize_phone_id(text)` (migration `000125`, IMMUTABLE) mengisi kolom **generated**
  `crm_leads.phone_normalized` / `crm_contacts.phone_normalized` — otomatis benar untuk semua jalur tulis
  (API, convert lead, import nanti) tanpa perubahan repository CRM.
- Go `internal/shared/phone.NormalizeID(raw) (string, bool)` untuk validasi input & nomor inbound.

Aturan: ambil digit saja; awalan `00` dibuang; `0…` → `62…`; `8…` (tanpa awalan) → `628…`; valid bila 8–15
digit, selain itu NULL / `("", false)`. Nomor luar negeri (`+1…`) tetap digitnya — cukup untuk matching.
- `chatId` WAHA untuk kirim: `<normalized>@c.us`.
- Inbound `from` bisa berupa `@lid` (bukan nomor). Processor memetakan via
  `GET /api/{session}/lids/{lid}` → `pn`; bila `pn` null, conversation tetap dibuat dengan
  `phone_normalized` kosong dan tidak di-match ke lead.

## WAHA Facts (verified)

Diverifikasi 2026-09-24 terhadap instance dev `https://waha.zyad.online` (read-only: `GET /api/version`,
`/api/server/status`, `/api/sessions?all=true`, `/api/server/environment`) dan spesifikasi resmi
`https://waha.devlike.pro/swagger/openapi.json` (versi spec 2026.8.2).

| Fakta | Nilai | Sumber |
|---|---|---|
| Versi | `2026.6.2`, tier `CORE` | instance |
| Engine | `GOWS` (`WHATSAPP_DEFAULT_ENGINE=GOWS`) — sesuai D7 | instance |
| Multi-session di Core | Diizinkan: sejak **2026.6.1** fitur Plus (unlimited sessions, media, storages, security) masuk Core | docs "WAHA Plus" |
| Auth API | Header `X-Api-Key`; tanpa key → `401 {"message":"Unauthorized"}` | instance |
| Storage session | GOWS storage per session `messages/groups/chats/labels = true`; `WAHA_MEDIA_STORAGE=LOCAL`. Persistensi `/app/.sessions` bergantung volume Docker — **belum terverifikasi** (box 10.0.9.2 tidak terjangkau saat verifikasi) | instance + docs |
| Session existing | `session_yulianto`, `session_zyad`, keduanya `FAILED`; webhook ke n8n (`n8n.zyad.cloud`) tanpa HMAC. **Bukan milik aplikasi — jangan disentuh** | instance |
| HMAC global | Tidak ada `WHATSAPP_HOOK_HMAC_KEY` di env → HMAC harus di-set per session | instance |
| Status session | `STOPPED`, `STARTING`, `SCAN_QR_CODE`, `PASSKEY_REQUIRED`, `PASSKEY_CONFIRMATION_REQUIRED`, `WORKING`, `FAILED` | spec |
| Session API | `POST /api/sessions` `{name, start, config{metadata, webhooks[]}}`; `GET/PUT/DELETE /api/sessions/{session}`; `POST /api/sessions/{session}/{start,stop,restart,logout}`; `GET /api/sessions/{session}/me` → `{id, pushName}` | spec + docs |
| Webhook config | `{url, events[], hmac{key}, retries{policy: constant\|linear\|exponential, delaySeconds, attempts}, customHeaders}` | spec |
| QR | `GET /api/{session}/auth/qr?format=image` + `Accept: application/json` → Base64File `{mimetype, data}`; `format=raw` → `{value}` | spec + docs |
| Pairing code | `POST /api/{session}/auth/request-code` `{phoneNumber}` (internasional, tanpa `+`) → `{code: "ABCD-ABCD"}` | spec + docs |
| Kirim teks | `POST /api/sendText` `{session, chatId, text, reply_to?, linkPreview?}` → WAMessage (`id`) | spec |
| chatId | `<nomor>@c.us` (user), `@g.us` (grup), `@newsletter`, `status@broadcast`; `@s.whatsapp.net` harus dikonversi ke `@c.us`; `@lid` = Linked ID | docs |
| LID mapping | `GET /api/{session}/lids/{lid}` → `{lid, pn}` (`pn` bisa null) | docs |
| Envelope webhook | `{id (ULID lowercase), timestamp (ms), event, session, metadata, me{id, pushName}, payload, engine, environment}` | spec |
| Event dipakai | `session.status` (payload `{name, status, statuses}`), `message` (payload WAMessage: `id, timestamp, from, fromMe, source, to, body, hasMedia, ack, ackName`), `message.any` (termasuk pesan `fromMe` dari HP), `message.ack` (payload `{id, from, to, participant, fromMe, ack, ackName}`) | spec + docs |
| Header webhook | `X-Webhook-Request-Id`, `X-Webhook-Timestamp` (ms), `X-Webhook-Hmac`, `X-Webhook-Hmac-Algorithm: sha512` (HMAC atas raw body) | docs |
| Anti-block | Kirim `POST /api/sendSeen` sebelum membalas pesan masuk | docs |
| Aturan karakter nama session | **Tidak didokumentasikan.** Mitigasi: backend hanya memakai `[a-z0-9_]` | docs |

### Temuan keamanan saat verifikasi (tindak lanjut ops, di luar kode)

- `GET /api/server/environment` mengembalikan `WAHA_DASHBOARD_PASSWORD` **plaintext** ke siapa pun yang
  memegang API key. Kredensial dashboard perlu dirotasi dan endpoint ini perlu dibatasi (mis. blokir path di
  nginx).
- API key WAHA dev sempat dibagikan di sesi chat untuk verifikasi → sebaiknya dirotasi setelah backend
  memakai key dari `shared/.env`.
- Webhook `session_yulianto` ke n8n tanpa HMAC (bukan scope modul ini, dicatat saja).

## Konfigurasi (env)

Reuse `WhatsAppConfig` (`internal/config/load.go`):

| Env | Status | Keterangan |
|---|---|---|
| `WHATSAPP_PROVIDER` | ada | `noop` (default) / `waha` |
| `WHATSAPP_API_URL` | ada | base URL WAHA, dev `https://waha.zyad.online` |
| `WHATSAPP_API_KEY` | ada | `X-Api-Key` |
| `WHATSAPP_API_SESSION` | ada | fallback session notifikasi platform |
| `WHATSAPP_API_URL_CALLBACK` | ada | URL webhook publik, dev `https://api.zyad.online/api/v1/webhooks/waha` |
| `WHATSAPP_TIMEOUT_SECONDS` | ada | default 15 |
| `WHATSAPP_WEBHOOK_HMAC_KEY` | **baru** | key HMAC per session |
| `WHATSAPP_ENGINE` | **baru** | informatif, default `GOWS` |
| `WHATSAPP_SEND_RATE_PER_MINUTE` | **baru** | default 20 |
| `WHATSAPP_WORKER_INTERVAL_SECONDS` / `WHATSAPP_RECONCILE_INTERVAL_SECONDS` | **baru** | worker webhook (default 5) / reconcile (default 300) |

## Fase Implementasi

| Fase | Branch | Isi | Plan mode |
|---|---|---|---|
| 1 | `feat/whatsapp-docs` | Dokumen ini + tasks + traceability + module-map | – |
| 2 | `feat/waha-platform-client` | WAHA client + HMAC verify di `internal/platform/whatsapp` | – |
| 3 | `feat/whatsapp-schema` | Migration `wa_*`, phone_normalized, activity type, permission & feature seed, domain + repository | ✓ |
| 4 | `feat/whatsapp-sessions` | Session & pairing API + reconcile worker | – |
| 5 | `feat/whatsapp-webhook` | Webhook receiver + inbound processor | ✓ |
| 6 | `feat/whatsapp-messaging` | Conversation & send API, rate limit, activity CRM | – |
| 7 | FE `feat/whatsapp-connections-ui` | Halaman WhatsApp (session, QR, kode pairing) + ConfirmDialog/Toast | – |
| 8 | FE `feat/lead-whatsapp-chat-panel` | Tab WhatsApp di Lead Detail (`ConversationPanel.vue`) | – |
| 9 | `feat/whatsapp-hardening` | Notification dispatcher via WAHA, audit keamanan, openapi, deploy dev | – |

Detail task: `docs/whatsapp-development-tasks.md`. Traceability: `docs/whatsapp-traceability-index.md`.

## Koreksi terhadap Prompting Plan

| # | Koreksi |
|---|---|
| K1 | `whatsapp.enabled` sudah ada (000061) → seed hanya `whatsapp.max_sessions` |
| K2 | Webhook di `POST /api/v1/webhooks/waha` (pola DOKU), bukan `/public/webhooks/waha` |
| K3 | Resolusi org via tabel non-RLS `wa_session_directory` |
| K4 | Nama session `zc_<12hex>_<rand6>`, bukan `org_<8char>_<seq>` |
| K5 | Reuse env `WHATSAPP_*` yang ada; tambah hanya yang belum ada |
| K6 | Worker: loop terpisah + identity `whatsapp-worker` |
| K7 | Rate limiter: helper kecil (pola `landing/service/visibility_service.go`), bukan library baru |
| K8 | Docs di `backend/docs/`; openapi hanya path whatsapp |
| K9 | FE: buat ConfirmDialog + useToast; QR dirender dari data-URL backend (tanpa dependency) |
| K10 | Chat di tab kolom tengah Lead Detail |
| K11 | RLS di semua tabel tenant `wa_*` walau `.claude/rules/rls.md` masih menyebut hanya `landing_*` |
| K12 | Kuota pesan bulanan ditunda |

## Non-Goals (MVP)

- Broadcast / kirim massal.
- Media (gambar, dokumen, voice) in/out.
- SSE/WebSocket realtime (MVP polling).
- Penegakan `whatsapp.max_messages_per_month`.
- Pesan grup, status, channel/newsletter (diabaikan saat inbound).
- Multi server WAHA (`waha_server_id` disiapkan, selalu `default`).
- Retensi/pembersihan otomatis isi chat.

## Risiko

| Risiko | Dampak | Mitigasi |
|---|---|---|
| WAHA tidak resmi (bukan WhatsApp Business API) | Nomor bisa diblokir WhatsApp, terutama pola kirim massal | Non-goal broadcast, rate limit per session, `sendSeen` sebelum balas, peringatan di UI saat pairing |
| Data pribadi chat (UU PDP) | Kewajiban kerahasiaan & retensi | RLS + permission own/read_all, tidak menyimpan `_data` mentah, kebijakan retensi di fase lanjutan |
| Server WAHA bersama dengan n8n | Operasi salah bisa mengganggu session non-aplikasi | Hanya kelola session di `wa_session_directory` + prefix `zc_` |
| Persistensi session belum terverifikasi | Restart container → semua tenant harus scan ulang | Verifikasi volume `/app/.sessions` di box dev sebelum Fase 9 (checklist restart container) |
| `@lid` tanpa pemetaan `pn` | Pesan masuk tidak bisa di-match ke lead | Conversation tetap dibuat tanpa relasi; bisa di-link manual (fase Inbox) |
| Webhook datang sebelum commit create session | Event ditolak karena session belum ada di directory | Directory ditulis sebelum `POST /api/sessions` ke WAHA; event tak dikenal disimpan dan di-retry |
