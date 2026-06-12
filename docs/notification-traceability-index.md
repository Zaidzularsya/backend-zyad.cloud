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
| `NOTIFICATION_WORKER_INTERVAL_SECONDS` | NT-BE-052 | done |
| `NOTIFICATION_WORKER_BATCH_SIZE` | NT-BE-052 | done |
| `TEST_DB_CONNECT_TIMEOUT_SECONDS` | NT-TEST-001 | done |
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
| Preference repository | NT-BE-005 | NT-BE-005 | done |
| Variable registry | NT-BE-010 | NT-BE-010 | done |
| Template parser | NT-BE-011 | NT-BE-011 | done |
| Template validator | NT-BE-012 | NT-BE-012 | done |
| Template renderer | NT-BE-013 | NT-BE-013 | done |
| Template service/API | NT-BE-020, NT-BE-021 | NT-BE-020, NT-BE-021 | done |
| Variable API | NT-BE-022 | NT-BE-022 | done |
| Dispatcher interface | NT-BE-030 | NT-BE-030 | done |
| Email dispatcher | NT-BE-031 | NT-BE-031 | done |
| WhatsApp dispatcher | NT-BE-032 | NT-BE-032 | done |
| Noop dispatcher | NT-BE-033 | NT-BE-033 | done |
| Discord dispatcher optional | Objective | NT-BE-034 | planned |
| Notification service | NT-BE-040 | NT-BE-040 | done |
| Log API | NT-BE-041 | NT-BE-041 | done |
| Preference API | NT-BE-042 | NT-BE-042 | done |
| Event consumer | NT-BE-050 | NT-BE-050 | done |
| Rule mapping | NT-BE-051 | NT-BE-051 | done |
| Outbox/event integration | NT-BE-052 | NT-BE-052 | done |
| Seed templates | NT-SEED-001 | NT-SEED-001 | done |
| Seed permissions | NT-SEED-002 | NT-SEED-002 | done |
| Repository integration test foundation | Testing Strategy | NT-TEST-001 | done |

## API Index

| Endpoint | Task ID | Status |
| --- | --- | --- |
| `GET /admin/notification-templates` | NT-BE-021 | done |
| `GET /admin/notification-templates/:id` | NT-BE-021 | done |
| `POST /admin/notification-templates` | NT-BE-021 | done |
| `PATCH /admin/notification-templates/:id` | NT-BE-021 | done |
| `DELETE /admin/notification-templates/:id` | NT-BE-021 | done |
| `POST /admin/notification-templates/:id/preview` | NT-BE-021 | done |
| `POST /admin/notification-templates/:id/activate` | NT-BE-021 | done |
| `POST /admin/notification-templates/:id/deactivate` | NT-BE-021 | done |
| `POST /admin/notification-templates/:id/archive` | NT-BE-021 | done |
| `POST /admin/notification-templates/:id/clone` | NT-BE-021 | done |
| `GET /admin/notification-template-variables` | NT-BE-022 | done |
| `GET /admin/notification-template-variables/:code` | NT-BE-022 | done |
| `GET /admin/notification-logs` | NT-BE-041 | done |
| `GET /admin/notification-logs/:id` | NT-BE-041 | done |
| `POST /admin/notification-logs/:id/retry` | NT-BE-041 | done |
| `POST /admin/notification-logs/:id/cancel` | NT-BE-041 | done |
| `GET /users/me/notification-preferences` | NT-BE-042 | done |
| `PATCH /users/me/notification-preferences` | NT-BE-042 | done |
| `GET /admin/users/:userId/notification-preferences` | NT-BE-042 | done |
| `PATCH /admin/users/:userId/notification-preferences` | NT-BE-042 | done |
| `POST /internal/notifications/send` | NT-BE-040 | done |

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
| 2026-06-12 | Implemented notification preference repository | `internal/core/notification/repository/preference_repository.go`, `internal/core/notification/repository/preference_repository_integration_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-005 untuk get user preferences, upsert, bulk upsert, dan enabled check dengan fallback default enabled |
| 2026-06-12 | Documented integration test SOP and implemented notification variable registry | `README.md`, `internal/core/notification/template/variable_registry.go`, `internal/core/notification/template/variable_registry_test.go`, `docs/notification-traceability-index.md` | Menambahkan command SOP integration test dan menyelesaikan NT-BE-010 untuk registry variable template MVP |
| 2026-06-12 | Implemented notification template parser | `internal/core/notification/template/parser.go`, `internal/core/notification/template/parser_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-011 untuk ekstraksi variable, dedupe, malformed syntax, dan unknown variable check berbasis registry |
| 2026-06-12 | Implemented notification template validator | `internal/core/notification/template/validator.go`, `internal/core/notification/template/validator_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-012 untuk validasi code, channel, locale, subject/body, registry variable, dan required available variable |
| 2026-06-12 | Implemented notification template renderer | `internal/core/notification/template/renderer.go`, `internal/core/notification/template/renderer_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-013 untuk render subject/body, preview sample payload, missing required variable, dan optional variable kosong |
| 2026-06-12 | Implemented notification template service and handler | `internal/core/notification/service/template_service.go`, `internal/core/notification/handler/template_handler.go`, `internal/app`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-020 dan NT-BE-021 untuk CRUD template, preview, activate/deactivate/archive, clone, response standar, pagination dasar, dan permission guard |
| 2026-06-12 | Implemented notification variable handler | `internal/core/notification/handler/variable_handler.go`, `internal/core/notification/handler/variable_handler_test.go`, `internal/app`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-022 untuk list/get registry variable template dengan permission guard `notification_variable.read` |
| 2026-06-12 | Implemented notification dispatcher interface | `internal/core/notification/dispatcher/dispatcher.go`, `internal/core/notification/dispatcher/dispatcher_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-030 untuk kontrak dispatcher, message, recipient, dan provider result tanpa coupling ke business module |
| 2026-06-12 | Implemented notification email dispatcher | `internal/core/notification/dispatcher/email_dispatcher.go`, `internal/platform/mail/mailer.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-031 untuk email dispatcher, validasi destination/subject, provider error mapping, dan mailer noop interface |
| 2026-06-12 | Implemented notification WhatsApp dispatcher | `internal/core/notification/dispatcher/whatsapp_dispatcher.go`, `internal/platform/whatsapp/client.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-032 untuk WhatsApp dispatcher, validasi destination, subject ignored, provider error mapping, dan WhatsApp noop client interface |
| 2026-06-12 | Implemented notification noop dispatcher | `internal/core/notification/dispatcher/noop_dispatcher.go`, `internal/core/notification/dispatcher/noop_dispatcher_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-033 untuk simulasi send local/dev tanpa memanggil provider eksternal |
| 2026-06-12 | Implemented notification service | `internal/core/notification/service/notification_service.go`, `internal/core/notification/service/notification_service_test.go`, `internal/core/notification/repository/notification_log_repository.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-040 untuk direct send, send by active template, retry, cancel, preference check, log-before-dispatch, dan provider error status update |
| 2026-06-12 | Wired internal notification send API | `internal/core/notification/handler/notification_handler.go`, `internal/core/notification/handler/notification_handler_test.go`, `internal/app`, `docs/notification-traceability-index.md` | Menyelesaikan follow-up NT-BE-040 untuk `POST /internal/notifications/send` memakai `NotificationService` dan dispatcher noop provider |
| 2026-06-12 | Implemented notification log handler | `internal/core/notification/handler/log_handler.go`, `internal/core/notification/handler/log_handler_test.go`, `internal/core/notification/service/notification_service.go`, `internal/core/notification/repository/notification_log_repository.go`, `internal/app`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-041 untuk list/get notification logs, retry failed/dead, cancel pending, dan permission guard |
| 2026-06-12 | Implemented notification preference service and handler | `internal/core/notification/service/preference_service.go`, `internal/core/notification/service/preference_service_test.go`, `internal/core/notification/handler/preference_handler.go`, `internal/core/notification/handler/preference_handler_test.go`, `internal/app`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-042 untuk user self preference, admin user preference, organization-specific query, bulk upsert, dan permission guard admin |
| 2026-06-12 | Seeded notification permissions | `migrations/000008_seed_notification_permissions.up.sql`, `migrations/000008_seed_notification_permissions.down.sql`, `internal/core/notification/repository/permission_seed_integration_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-SEED-002 untuk permission notification idempotent dan assignment ke role `super_admin` |
| 2026-06-12 | Seeded default notification templates | `migrations/000009_seed_notification_templates.up.sql`, `migrations/000009_seed_notification_templates.down.sql`, `internal/core/notification/repository/template_seed_integration_test.go`, `internal/platform/database/testutil/db.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-SEED-001 untuk system template default, variable registry match, dan sample payload preview |
| 2026-06-12 | Implemented notification rule mapping service | `internal/core/notification/service/notification_rule_service.go`, `internal/core/notification/service/notification_rule_service_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-051 untuk hardcoded MVP mapping `auth.password_reset_requested` dan `user.invited`/`user.invitation_created` |
| 2026-06-12 | Implemented notification event consumer | `internal/core/notification/consumer/notification_event_consumer.go`, `internal/core/notification/consumer/notification_event_consumer_test.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-050 untuk consume event, map rule, send notification, ignore unknown event, dan capture error tanpa crash |
| 2026-06-12 | Implemented notification outbox integration | `migrations/000010_create_notification_outbox_events.up.sql`, `migrations/000010_create_notification_outbox_events.down.sql`, `internal/core/notification/domain/outbox_event.go`, `internal/core/notification/repository/outbox_repository.go`, `internal/core/notification/repository/outbox_repository_integration_test.go`, `internal/core/notification/consumer/outbox_worker.go`, `internal/core/notification/consumer/outbox_worker_test.go`, `internal/platform/database/testutil/db.go`, `docs/notification-traceability-index.md` | Menyelesaikan NT-BE-052 untuk outbox event idempotent, claim pending event, worker consume event, dan status succeeded/failed/dead |
| 2026-06-12 | Wired notification worker executable | `cmd/worker/main.go`, `.env.example`, `internal/config`, `README.md`, `docs/notification-development-tasks.md`, `docs/notification-traceability-index.md` | Menyelesaikan follow-up NT-BE-052 untuk menjalankan outbox worker loop dan retry due notification logs dari command line |
| 2026-06-12 | Added notification outbox publisher helper | `internal/core/notification/publisher/outbox_publisher.go`, `internal/core/notification/publisher/outbox_publisher_test.go`, `cmd/worker/main.go`, `.env.example`, `internal/config`, `README.md`, `docs/notification-development-tasks.md`, `docs/notification-traceability-index.md` | Menambahkan helper publish event dari business module ke outbox dan menjadikan worker default mail-first tanpa WhatsApp dispatcher |
| 2026-06-12 | Wired SMTP mail provider for notification send path | `internal/platform/mail/mailer.go`, `internal/app/app.go`, `cmd/worker/main.go`, `README.md`, `docs/notification-development-tasks.md`, `docs/notification-traceability-index.md` | Menambahkan SMTP mailer berbasis `MAIL_*`, fallback noop saat `MAIL_HOST` kosong, dan wiring API/worker agar notifikasi email bisa dites dengan provider mail nyata |
| 2026-06-12 | Completed notification integration test foundation verification | `internal/config`, `internal/platform/database`, `.env.example`, `README.md`, `docs/notification-traceability-index.md` | Menambahkan connect timeout untuk DB/test DB, `Ping` saat connect, dan memverifikasi repository integration test terhadap PostgreSQL `platform_test` di VPS melalui SSH tunnel |
| 2026-06-12 | Added password changed notification support | `internal/core/notification/service/notification_rule_service.go`, `internal/core/notification/template/variable_registry.go`, `migrations/000011_seed_password_changed_notification_template.*.sql`, `docs/notification-traceability-index.md` | Menambahkan mapping event `auth.password_changed`, registry variable, dan system email template untuk penyelesaian `AUTH-0107` |
