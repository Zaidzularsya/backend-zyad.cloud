# WhatsApp Channel Traceability Index

Dokumen ini menghubungkan requirement WhatsApp Channel ke migration, endpoint, permission, config, test, dan
status.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `deferred`: sengaja ditunda.

Nomor migration di bawah adalah **rencana** (terakhir `000121` per 2026-09-24) — cek ulang saat Fase 3.

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-whatsapp.md` | Source requirement, arsitektur, WAHA Facts (verified) |
| `docs/whatsapp-development-tasks.md` | Breakdown task per fase |
| `docs/reference-crm.md` | Lead Detail (konsumen panel chat) |
| `.env.example` | Config contract `WHATSAPP_*` |

## Module Mapping

| Module | Path | Responsibility |
| --- | --- | --- |
| Platform WhatsApp | `internal/platform/whatsapp` | Adapter WAHA HTTP + HMAC verify + NoopClient |
| WhatsApp | `internal/modules/whatsapp` | Session, conversation, message, webhook inbound |
| Phone helper | `internal/shared/phone` | Normalisasi nomor untuk matching |
| CRM | `internal/modules/crm` | `phone_normalized` lead/contact, activity `whatsapp` |
| Notification | `internal/core/notification/dispatcher/whatsapp_dispatcher.go` | Kirim notifikasi platform via WAHA |
| Worker | `cmd/worker` | Webhook processor + reconcile status |

## Requirement Traceability

| Req | Requirement | Migration | Endpoint | Permission | Test | Status |
| --- | --- | --- | --- | --- | --- | --- |
| WA-R01 | Tenant/platform membuat session (≤ `whatsapp.max_sessions`) | `wa_sessions`, `wa_session_directory`, seed feature | `POST /app/whatsapp/sessions` | `whatsapp.session.manage` | WA-TEST-010 | planned |
| WA-R02 | Lihat daftar & detail session | `wa_sessions` | `GET /app/whatsapp/sessions`, `GET /sessions/:id`, `GET /sessions/:id/status` | `whatsapp.session.read` | WA-TEST-010 | planned |
| WA-R03 | Pairing via QR | – | `GET /app/whatsapp/sessions/:id/qr` | `whatsapp.session.manage` | WA-TEST-010, WA-OPS-002 | planned |
| WA-R04 | Pairing via kode | – | `POST /app/whatsapp/sessions/:id/pairing-code` | `whatsapp.session.manage` | WA-TEST-010, WA-OPS-002 | planned |
| WA-R05 | Start/stop/logout/hapus/ubah session | `wa_sessions` | `POST /sessions/:id/{start,stop,logout}`, `PATCH`/`DELETE /sessions/:id` | `whatsapp.session.manage` | WA-TEST-010 | planned |
| WA-R06 | Status session tersinkron | `wa_sessions` | webhook `session.status` + reconcile worker | – | WA-TEST-020 | planned |
| WA-R07 | Terima webhook aman & idempotent | `wa_webhook_events` | `POST /api/v1/webhooks/waha` | publik + HMAC | WA-TEST-020 | planned |
| WA-R08 | Pesan masuk tersimpan & di-match ke lead/contact | `wa_conversations`, `wa_messages`, `phone_normalized` | (worker) | – | WA-TEST-020 | planned |
| WA-R09 | Auto-create lead untuk nomor tak dikenal (opsional per session) | `wa_sessions.auto_create_lead` | (worker) | – | WA-TEST-020 | planned |
| WA-R10 | Daftar percakapan (own vs all) | `wa_conversations` | `GET /app/whatsapp/conversations` | `whatsapp.conversation.read` / `read_all` | WA-TEST-030 | planned |
| WA-R11 | Riwayat pesan dengan cursor | `wa_messages` | `GET /conversations/:id/messages` | `whatsapp.conversation.read` | WA-TEST-030 | planned |
| WA-R12 | Kirim pesan teks + retry | `wa_messages` | `POST /conversations/:id/messages`, `POST /messages/:id/retry` | `whatsapp.message.send` | WA-TEST-030 | planned |
| WA-R13 | Mulai chat dari lead/contact | `wa_conversations` | `POST /conversations/start` | `whatsapp.message.send` | WA-TEST-030 | planned |
| WA-R14 | Tandai dibaca, assign, tutup percakapan | `wa_conversations` | `POST /conversations/:id/read`, `PATCH /conversations/:id` | `whatsapp.conversation.read` / `assign` | WA-TEST-030 | planned |
| WA-R15 | Status centang pesan keluar | `wa_messages` | webhook `message.ack` | – | WA-TEST-020 | planned |
| WA-R16 | Rate limit kirim per session | – (Redis) | `POST /conversations/:id/messages` → 429 | – | WA-TEST-030 | planned |
| WA-R17 | Jejak chat di timeline CRM | `crm_activities` type `whatsapp` | (service) | – | WA-TEST-030 | planned |
| WA-R18 | Halaman WhatsApp Connections | – | FE | `whatsapp.session.read` | WA-FE-* | planned |
| WA-R19 | Tab WhatsApp di Lead Detail | – | FE | `whatsapp.conversation.read`, `whatsapp.message.send` | WA-FE-014 | planned |
| WA-R20 | Notifikasi platform via WAHA | – | (dispatcher) | – | WA-BE-040 | planned |
| WA-R21 | Kuota pesan bulanan | `organization_usage_counters` (existing) | – | – | – | deferred |

## Migration Traceability (rencana)

| Rencana | Isi | Task |
| --- | --- | --- |
| `000122` | `wa_sessions` + `wa_session_directory` | WA-DB-001, WA-DB-002 |
| `000123` | `wa_conversations` + `wa_messages` | WA-DB-003, WA-DB-004 |
| `000124` | `wa_webhook_events` | WA-DB-005 |
| `000125` | `phone_normalized` lead/contact + backfill + index | WA-DB-006 |
| `000126` | `crm_activities` type `whatsapp` | WA-DB-007 |
| `000127` | Permission seed module `whatsapp` | WA-DB-008 |
| `000128` | Feature seed `whatsapp.max_sessions` | WA-DB-009 |

## Permission Traceability

| Permission | owner | member | super_admin | Dipakai di |
| --- | --- | --- | --- | --- |
| `whatsapp.session.read` | ✓ | ✓ | ✓ | GET sessions, menu FE |
| `whatsapp.session.manage` | ✓ | – | ✓ | create/start/stop/logout/delete/patch session, QR, pairing code |
| `whatsapp.conversation.read` | ✓ | ✓ | ✓ | GET conversations (milik sendiri), messages, read |
| `whatsapp.conversation.read_all` | ✓ | – | ✓ | GET conversations semua assignee |
| `whatsapp.conversation.assign` | ✓ | – | ✓ | PATCH conversation assignee |
| `whatsapp.message.send` | ✓ | ✓ | ✓ | send, retry, start conversation |

## Entitlement Traceability

| Feature key | Tipe | Status | Sumber |
| --- | --- | --- | --- |
| `whatsapp.enabled` | boolean | ada | `000061` (free/starter off, growth+ on) |
| `whatsapp.max_sessions` | integer | planned | seed baru Fase 3 |
| `whatsapp.max_messages_per_month` | integer | ada, penegakan deferred | `000061` |

## Config Traceability

| Config | Task | Status |
| --- | --- | --- |
| `WHATSAPP_PROVIDER` | WA-PLAT-005 | ada (reuse) |
| `WHATSAPP_API_URL` | WA-PLAT-002 | ada (reuse) |
| `WHATSAPP_API_KEY` | WA-PLAT-002 | ada (reuse) |
| `WHATSAPP_API_SESSION` | WA-BE-040 | ada (reuse) |
| `WHATSAPP_API_URL_CALLBACK` | WA-BE-015 | ada (reuse) |
| `WHATSAPP_TIMEOUT_SECONDS` | WA-PLAT-003 | ada (reuse) |
| `WHATSAPP_WEBHOOK_HMAC_KEY` | WA-PLAT-001, WA-BE-020 | done |
| `WHATSAPP_ENGINE` | WA-PLAT-001 | done |
| `WHATSAPP_SEND_RATE_PER_MINUTE` | WA-BE-035 | planned |
| `WHATSAPP_WORKER_INTERVAL_SECONDS` | WA-WRK-010 | planned |
| `WHATSAPP_RECONCILE_INTERVAL_SECONDS` | WA-WRK-001 | planned |
