# Notification Traceability Index

Dokumen ini menghubungkan requirement notification ke task, API, migration, config, dan status.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `deferred`: sengaja ditunda.

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-notification.md` | Source requirement Notification & Template Management |
| `docs/notification-development-tasks.md` | Breakdown task development |
| `.env.example` | Config contract provider |

## Module Mapping

| Module | Path | Responsibility |
| --- | --- | --- |
| Notification core | `internal/core/notification` | Template, rendering, log, preference, service, dispatcher orchestration |
| Platform mail | `internal/platform/mail` | Email provider adapter |
| Platform WhatsApp | `internal/platform/whatsapp` | WhatsApp provider adapter |
| Platform Discord optional | `internal/platform/discord` | Discord provider adapter |
| Worker | `cmd/worker` | Event consumer and retry worker |

## Config Traceability

| Config | Task | Status |
| --- | --- | --- |
| `NOTIFICATION_DEFAULT_LOCALE` | NT-CONFIG-001 | done |
| `NOTIFICATION_PROVIDER_MODE` | NT-CONFIG-001 | done |
| `NOTIFICATION_MAX_ATTEMPTS` | NT-CONFIG-001 | done |
| `MAIL_HOST` | NT-PLAT-001 | done |
| `MAIL_PORT` | NT-PLAT-001 | done |
| `MAIL_USER` | NT-PLAT-001 | done |
| `MAIL_PASSWORD` | NT-PLAT-001 | done |
| `MAIL_FROM` | NT-PLAT-001 | done |
| `MAIL_SECURE` | NT-PLAT-001 | done |
| `MAIL_TLS` | NT-PLAT-001 | done |
| `WHATSAPP_PROVIDER` | NT-CONFIG-002 | done |
| `WHATSAPP_API_URL` | NT-CONFIG-002 | done |
| `WHATSAPP_API_KEY` | NT-CONFIG-002 | done |
| `WHATSAPP_API_SESSION` | NT-CONFIG-002 | done |
| `WHATSAPP_SENDER` | NT-CONFIG-002 | done |
| `WHATSAPP_API_URL_CALLBACK` | NT-CONFIG-002 | done |
| `WHATSAPP_TIMEOUT_SECONDS` | NT-CONFIG-002 | done |
| `DISCORD_PROVIDER` | NT-CONFIG-003 | done |
| `DISCORD_ENABLED` | NT-CONFIG-003 | done |
| `DISCORD_WEBHOOK_URL` | NT-CONFIG-003 | done |
| `DISCORD_BOT_TOKEN` | NT-CONFIG-003 | done |
| `DISCORD_DEFAULT_CHANNEL_ID` | NT-CONFIG-003 | done |
| `DISCORD_USERNAME` | NT-CONFIG-003 | done |
| `DISCORD_AVATAR_URL` | NT-CONFIG-003 | done |

## Requirement Traceability

| Requirement | Reference Section | Task ID | Status |
| --- | --- | --- | --- |
| Notification config | 13 Phase 9 | NT-CONFIG-001 | done |
| WhatsApp config | 13 Phase 9 | NT-CONFIG-002 | done |
| Discord optional config | 2 Objective | NT-CONFIG-003 | done |
| Notification templates migration | 4.1, NT-DB-001 | NT-DB-001 | done |
| Notification logs migration | 4.2, NT-DB-002 | NT-DB-002 | done |
| Notification preferences migration | 4.3, NT-DB-003 | NT-DB-003 | done |
| Domain models | NT-BE-001 | NT-BE-001 | done |
| DTOs | NT-BE-002 | NT-BE-002 | done |
| Template repository | NT-BE-003 | NT-BE-003 | done |
| Log repository | NT-BE-004 | NT-BE-004 | done |
| Preference repository | NT-BE-005 | NT-BE-005 | planned |
| Variable registry | NT-BE-010 | NT-BE-010 | planned |
| Template parser | NT-BE-011 | NT-BE-011 | planned |
| Template validator | NT-BE-012 | NT-BE-012 | planned |
| Template renderer | NT-BE-013 | NT-BE-013 | planned |
| Template service/API | NT-BE-020, NT-BE-021 | NT-BE-020, NT-BE-021 | planned |
| Variable API | NT-BE-022 | NT-BE-022 | planned |
| Dispatcher interface | NT-BE-030 | NT-BE-030 | planned |
| Email dispatcher | NT-BE-031 | NT-BE-031 | planned |
| WhatsApp dispatcher | NT-BE-032 | NT-BE-032 | planned |
| Noop dispatcher | NT-BE-033 | NT-BE-033 | planned |
| Discord dispatcher optional | Objective | NT-BE-034 | planned |
| Notification service | NT-BE-040 | NT-BE-040 | planned |
| Log API | NT-BE-041 | NT-BE-041 | planned |
| Preference API | NT-BE-042 | NT-BE-042 | planned |
| Event consumer | NT-BE-050 | NT-BE-050 | planned |
| Rule mapping | NT-BE-051 | NT-BE-051 | planned |
| Outbox/event integration | NT-BE-052 | NT-BE-052 | planned |
| Seed templates | NT-SEED-001 | NT-SEED-001 | planned |
| Seed permissions | NT-SEED-002 | NT-SEED-002 | planned |
| Repository integration test foundation | Testing Strategy | NT-TEST-001 | in_progress |

## API Index

| Endpoint | Task ID | Status |
| --- | --- | --- |
| `GET /admin/notification-templates` | NT-BE-021 | planned |
| `GET /admin/notification-templates/:id` | NT-BE-021 | planned |
| `POST /admin/notification-templates` | NT-BE-021 | planned |
| `PATCH /admin/notification-templates/:id` | NT-BE-021 | planned |
| `DELETE /admin/notification-templates/:id` | NT-BE-021 | planned |
| `POST /admin/notification-templates/:id/preview` | NT-BE-021 | planned |
| `POST /admin/notification-templates/:id/activate` | NT-BE-021 | planned |
| `POST /admin/notification-templates/:id/deactivate` | NT-BE-021 | planned |
| `POST /admin/notification-templates/:id/archive` | NT-BE-021 | planned |
| `POST /admin/notification-templates/:id/clone` | NT-BE-021 | planned |
| `GET /admin/notification-template-variables` | NT-BE-022 | planned |
| `GET /admin/notification-template-variables/:code` | NT-BE-022 | planned |
| `GET /admin/notification-logs` | NT-BE-041 | planned |
| `GET /admin/notification-logs/:id` | NT-BE-041 | planned |
| `POST /admin/notification-logs/:id/retry` | NT-BE-041 | planned |
| `POST /admin/notification-logs/:id/cancel` | NT-BE-041 | planned |
| `GET /users/me/notification-preferences` | NT-BE-042 | planned |
| `PATCH /users/me/notification-preferences` | NT-BE-042 | planned |
| `GET /admin/users/:userId/notification-preferences` | NT-BE-042 | planned |
| `PATCH /admin/users/:userId/notification-preferences` | NT-BE-042 | planned |
| `POST /internal/notifications/send` | NT-BE-040 | planned |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-11 | Email config tetap memakai `MAIL_*` | Repo sudah punya `MailConfig`; menghindari duplikasi `SMTP_*` dan `MAIL_*` |
| 2026-06-11 | WhatsApp dan Discord default ke `noop` | Local development tidak boleh bergantung external provider |
| 2026-06-11 | Discord dibuat optional | Reference menyebut Discord optional, bukan channel wajib MVP |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-11 | Initial notification task documentation and config contract | `docs/notification-development-tasks.md`, `docs/notification-traceability-index.md`, `.env.example`, `internal/config` | Menambahkan task breakdown dan env untuk notification, WhatsApp, Discord optional |
| 2026-06-11 | Implemented notification templates migration | `migrations/000005_create_notification_templates.up.sql`, `migrations/000005_create_notification_templates.down.sql`, `docs/notification-traceability-index.md` | Menyelesaikan NT-DB-001 untuk table template notification |
| 2026-06-11 | Implemented notification logs migration | `migrations/000006_create_notification_logs.up.sql`, `migrations/000006_create_notification_logs.down.sql`, `docs/notification-traceability-index.md` | Menyelesaikan NT-DB-002 untuk log, retry, provider response, dan recipient snapshot |
| 2026-06-11 | Implemented notification preferences migration | `migrations/000007_create_notification_preferences.up.sql`, `migrations/000007_create_notification_preferences.down.sql`, `docs/notification-traceability-index.md` | Menyelesaikan NT-DB-003 untuk preference per user, organization, event, dan channel |
| 2026-06-11 | Implemented notification domain models | `internal/core/notification/domain`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-001 untuk template, log, preference, channel, variable, dan notification request domain |
| 2026-06-11 | Implemented notification DTOs | `internal/core/notification/dto`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-002 untuk template request/response, preview, send request, log response, dan preference DTO |
| 2026-06-11 | Implemented notification template repository | `internal/core/notification/repository/template_repository.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-003 untuk CRUD, soft delete, active lookup, status transition, dan pagination |
| 2026-06-11 | Implemented notification log repository | `internal/core/notification/repository/notification_log_repository.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-004 untuk pending log, status transition, provider response, dan retry query |
| 2026-06-11 | Added repository integration test foundation | `.env.example`, `internal/config`, `internal/platform/database/testutil`, `internal/core/notification/repository/template_repository_integration_test.go`, `docs/notification-development-tasks.md`, `docs/notification-traceability-index.md` | Menambahkan test DB opt-in dengan build tag `integration`; verifikasi integration test menunggu database test aktif |
