# WhatsApp Channel Development Tasks

Dokumen ini adalah breakdown task development modul WhatsApp Channel (WAHA).

Sumber requirement:

- `docs/reference-whatsapp.md`
- `docs/reference-crm.md` (Lead Detail)
- `docs/whatsapp-traceability-index.md`

Status: `planned` · `in_progress` · `done` · `deferred`.

Setiap fase dikerjakan di branch `feat/…` sendiri → verifikasi lokal → merge ke `staging-dev` → deploy dev
(`zyad.online`). Fase bertanda **Plan mode** menyentuh migration/RLS/webhook publik (aturan `CLAUDE.md`).

## Fase 1 — Dokumen & verifikasi WAHA (`feat/whatsapp-docs`)

| ID | Task | Status |
|---|---|---|
| WA-DOC-001 | `docs/reference-whatsapp.md` (requirement, arsitektur, tenant boundary, security, data model, non-goals, risiko) | done |
| WA-DOC-002 | `docs/whatsapp-development-tasks.md` | done |
| WA-DOC-003 | `docs/whatsapp-traceability-index.md` | done |
| WA-DOC-004 | Baris `whatsapp` di `docs/module-map.md` + catatan dependency di `docs/reference-crm.md` | done |
| WA-DOC-005 | Verifikasi WAHA dev (versi, engine, auth, event, HMAC, QR, pairing) → "WAHA Facts (verified)" | done |
| WA-DOC-006 | Verifikasi persistensi volume `/app/.sessions` di box dev | planned (butuh akses 10.0.9.2) |

## Fase 2 — Platform client (`feat/waha-platform-client`)

| ID | Task | Status |
|---|---|---|
| WA-PLAT-001 | `WhatsAppConfig` + `LoadWhatsApp`: `WebhookHMACKey`, `Engine`, `SendRatePerMinute`, interval worker; `.env.example` | done |
| WA-PLAT-002 | `WAHAClient`: CreateSession, Start/Stop/Logout/DeleteSession, GetSession, GetQR (Base64File), RequestPairingCode, GetMe, SendText, SendSeen, ResolveLID | done |
| WA-PLAT-003 | Error sentinel `ErrSessionNotFound`, `ErrUnauthorized`, `ErrUpstream`; timeout; `io.LimitReader` | done |
| WA-PLAT-004 | `VerifyWebhookSignature(body, headers, key)` HMAC-SHA512 + `hmac.Equal` | done |
| WA-PLAT-005 | `NewClientFromConfig`: `waha` → WAHAClient, lainnya → NoopClient | done |
| WA-PLAT-006 | Unit test `httptest.Server` untuk semua method + HMAC | done |

## Fase 3 — Schema, domain, permission (`feat/whatsapp-schema`, Plan mode)

| ID | Task | Status |
|---|---|---|
| WA-DB-001 | Migration `wa_sessions` (RLS) | planned |
| WA-DB-002 | Migration `wa_session_directory` (non-RLS) | planned |
| WA-DB-003 | Migration `wa_conversations` (RLS, `UNIQUE(session_id, chat_id)`) | planned |
| WA-DB-004 | Migration `wa_messages` (RLS, index `(conversation_id, sent_at desc)`) | planned |
| WA-DB-005 | Migration `wa_webhook_events` (non-RLS, `event_id` unik) | planned |
| WA-DB-006 | `phone_normalized` di `crm_leads`/`crm_contacts` + backfill + index | planned |
| WA-DB-007 | `crm_activities_type_check` + `'whatsapp'`; const Go di `crm/domain/activity.go` | planned |
| WA-DB-008 | Permission seed module `whatsapp` (owner/member/super_admin) | planned |
| WA-DB-009 | Feature seed `whatsapp.max_sessions` + nilai per plan | planned |
| WA-BE-001 | `internal/shared/phone.NormalizeID` + unit test; set `phone_normalized` di repo lead & contact | planned |
| WA-BE-002 | `internal/modules/whatsapp/{domain,repository,dto}` + repository contract | planned |
| WA-TEST-001 | Migrate up → down → up bersih; integration test RLS menolak baris org lain | planned |

## Fase 4 — Session & pairing API (`feat/whatsapp-sessions`)

| ID | Task | Status |
|---|---|---|
| WA-BE-010 | Grup `/api/v1/app/whatsapp` + middleware tenant/entitlement; wiring `app.go`/`dependency.go`/`router.go` | planned |
| WA-BE-011 | `GET/POST /sessions`, `GET/PATCH/DELETE /sessions/:id` (quota `whatsapp.max_sessions`) | planned |
| WA-BE-012 | `POST /sessions/:id/{start,stop,logout}` | planned |
| WA-BE-013 | `GET /sessions/:id/qr` (data-URL, hanya saat `SCAN_QR_CODE`), `POST /sessions/:id/pairing-code` | planned |
| WA-BE-014 | `GET /sessions/:id/status` (sinkron ke WAHA bila basi) | planned |
| WA-BE-015 | Pembangkitan nama session `zc_<12hex>_<rand6>` + directory ditulis sebelum create di WAHA | planned |
| WA-WRK-001 | Loop reconcile di `cmd/worker` (identity `whatsapp-worker`) | planned |
| WA-TEST-010 | Unit test SessionService dengan fake client (quota, status, tenant) | planned |

## Fase 5 — Webhook & inbound (`feat/whatsapp-webhook`, Plan mode)

| ID | Task | Status |
|---|---|---|
| WA-BE-020 | `POST /api/v1/webhooks/waha`: HMAC → 401, simpan `wa_webhook_events` idempotent, 200 | planned |
| WA-WRK-010 | Processor: claim `FOR UPDATE SKIP LOCKED`, resolve org via directory, tenant context, retry/attempts | planned |
| WA-BE-021 | `session.status` → update `wa_sessions` (+ phone/push_name saat WORKING); notifikasi FAILED/logout | planned |
| WA-BE-022 | `message` inbound (skip grup/status/newsletter), LID → pn, upsert conversation, insert message | planned |
| WA-BE-023 | Matching `phone_normalized` lead → contact; assignee = owner; auto-create lead bila `auto_create_lead` | planned |
| WA-BE-024 | `message.ack` → status pesan outbound (tidak mundur) | planned |
| WA-TEST-020 | Integration test payload contoh, duplikat, signature salah | planned |

## Fase 6 — Conversation & send API (`feat/whatsapp-messaging`)

| ID | Task | Status |
|---|---|---|
| WA-BE-030 | `GET /conversations` (filter + scope own/read_all) | planned |
| WA-BE-031 | `GET /conversations/:id/messages?before=&limit=` (cursor) | planned |
| WA-BE-032 | `POST /conversations/:id/messages`, `POST /messages/:id/retry` | planned |
| WA-BE-033 | `POST /conversations/start` (session default, validasi nomor & status WORKING) | planned |
| WA-BE-034 | `POST /conversations/:id/read`, `PATCH /conversations/:id` (assignee, status) | planned |
| WA-BE-035 | Rate limit Redis per session → 429 | planned |
| WA-BE-036 | Activity CRM tipe `whatsapp` (satu per conversation per hari) | planned |
| WA-TEST-030 | Unit test scope own vs read_all, rate limit, gagal kirim | planned |

## Fase 7 — FE WhatsApp Connections (`feat/whatsapp-connections-ui`, repo frontend)

| ID | Task | Status |
|---|---|---|
| WA-FE-001 | `ConfirmDialog.vue` + `useToast` + host di layout | planned |
| WA-FE-002 | `src/features/whatsapp` api/queries/types; permission di union `src/types/auth.ts` | planned |
| WA-FE-003 | Route tenant & platform + menu grup Integrations | planned |
| WA-FE-004 | Daftar session + aksi (hubungkan ulang, logout, hapus, jadikan default) | planned |
| WA-FE-005 | Modal hubungkan: tab QR (polling 3 dtk) + tab kode pairing | planned |
| WA-FE-006 | State kosong/loading/error, pesan quota, peringatan risiko | planned |
| WA-FE-007 | Catatan/link di IntegrationsPage untuk provider `whatsapp` | planned |

## Fase 8 — FE chat di Lead Detail (`feat/lead-whatsapp-chat-panel`, repo frontend)

| ID | Task | Status |
|---|---|---|
| WA-FE-010 | `ConversationPanel.vue` reusable | planned |
| WA-FE-011 | Tab `Timeline \| WhatsApp` di kolom tengah `LeadDetailPage.vue` + badge unread | planned |
| WA-FE-012 | Polling 5 dtk saat tab aktif & `visibilityState === 'visible'`; mark read | planned |
| WA-FE-013 | Composer (Enter/Shift+Enter, optimistic, retry), read-only tanpa `whatsapp.message.send` | planned |
| WA-FE-014 | Vitest: empty state, bubble, kirim | planned |

## Fase 9 — Hardening & deploy dev (`feat/whatsapp-hardening`)

| ID | Task | Status |
|---|---|---|
| WA-BE-040 | `WhatsAppDispatcher` pakai `NewClientFromConfig` di `app.go` dan `cmd/worker` | planned |
| WA-SEC-001 | Audit: API key tidak bocor, HMAC wajib, RLS semua tabel tenant, tidak ada nama session mentah dari klien | planned |
| WA-DOC-010 | Path `/app/whatsapp/*` + `/webhooks/waha` di `api/openapi.yaml` | planned |
| WA-DOC-011 | Update status traceability & tasks | planned |
| WA-OPS-001 | Env WAHA di `shared/.env` dev, deploy BE (migrate+api+worker) & FE | planned |
| WA-OPS-002 | Checklist uji manual dev (pairing QR/kode, kirim/terima, ack, auto-lead, restart container, scope member, quota) | planned |

## Deferred (fase lanjutan)

| ID | Task |
|---|---|
| WA-NEXT-010 | Halaman Inbox (semua percakapan, filter, assign) |
| WA-NEXT-011 | Media in/out via modul asset |
| WA-NEXT-012 | Template / quick reply + variabel |
| WA-NEXT-013 | Otomatisasi (sambutan lead landing, pengingat activity) |
| WA-NEXT-014 | Kirim quotation/invoice CRM (PDF) |
| WA-NEXT-015 | SSE realtime |
| WA-NEXT-016 | Multi server WAHA + dashboard kesehatan session |
| WA-NEXT-017 | Penegakan `whatsapp.max_messages_per_month` via `organization_usage_counters` |
| WA-NEXT-018 | Kebijakan retensi isi chat (UU PDP) |
