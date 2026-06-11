# Notification Development Tasks

Dokumen ini adalah breakdown task development untuk core notification.

Sumber requirement:

- `docs/reference-notification.md`
- `README.md`
- `docs/auth-user-development-tasks.md`

## Tujuan

Membangun core Notification & Template Management yang bisa dipakai lintas module seperti auth, user, organization, order, billing, payment, lead, dan permission.

Scope channel:

- `email`
- `whatsapp`
- `in_app`
- `discord` optional

## Development Pattern

`internal/core/notification` memegang business capability:

- Template CRUD.
- Template rendering dan validation.
- Notification rules.
- Notification logs.
- Notification preferences.
- Dispatcher orchestration.
- API management endpoint.

`internal/platform/mail`, `internal/platform/whatsapp`, dan optional `internal/platform/discord` hanya adapter teknis provider.

Tidak boleh:

- Menaruh business template logic di platform adapter.
- Membaca env langsung dari service notification.
- Mengirim provider request tanpa notification log.
- Menyimpan secret provider di template.

## Config Contract

Email memakai env yang sudah ada:

```env
MAIL_HOST=
MAIL_PORT=
MAIL_USER=
MAIL_PASSWORD=
MAIL_FROM=
MAIL_SECURE=
MAIL_TLS=
```

Notification umum:

```env
NOTIFICATION_DEFAULT_LOCALE=id-ID
NOTIFICATION_PROVIDER_MODE=noop
NOTIFICATION_MAX_ATTEMPTS=3
```

WhatsApp:

```env
WHATSAPP_PROVIDER=noop
WHATSAPP_API_URL=
WHATSAPP_API_KEY=
WHATSAPP_API_SESSION=
WHATSAPP_SENDER=
WHATSAPP_API_URL_CALLBACK=
WHATSAPP_TIMEOUT_SECONDS=15
```

Discord optional:

```env
DISCORD_PROVIDER=noop
DISCORD_ENABLED=false
DISCORD_WEBHOOK_URL=
DISCORD_BOT_TOKEN=
DISCORD_DEFAULT_CHANNEL_ID=
DISCORD_USERNAME=
DISCORD_AVATAR_URL=
```

## Phase 0 - Config Foundation

### NT-CONFIG-001: Notification Config

Scope:

- Tambahkan `NotificationConfig`.
- Tambahkan env `NOTIFICATION_DEFAULT_LOCALE`.
- Tambahkan env `NOTIFICATION_PROVIDER_MODE`.
- Tambahkan env `NOTIFICATION_MAX_ATTEMPTS`.

Acceptance criteria:

- App bisa boot dengan provider mode `noop`.
- Missing external provider config tidak mematikan local development.
- Config dibaca dari `internal/config`.

### NT-CONFIG-002: WhatsApp Config

Scope:

- Tambahkan provider mode WhatsApp.
- Tambahkan sender dan timeout.
- Pertahankan env WhatsApp lama yang sudah ada.

Acceptance criteria:

- WhatsApp adapter bisa memakai config tanpa membaca env langsung.
- Default provider adalah `noop`.

### NT-CONFIG-003: Discord Optional Config

Scope:

- Tambahkan config Discord optional.
- Support webhook URL dan bot token.
- Default disabled.

Acceptance criteria:

- App bisa boot tanpa Discord credential.
- Dispatcher Discord hanya aktif jika enabled/provider sesuai.

## Phase 1 - Database Foundation

### NT-DB-001: Create `notification_templates`

Scope:

- Buat migration table `notification_templates`.
- Tambahkan unique `(code, channel, locale, version)`.
- Gunakan JSONB untuk `available_variables` dan `sample_payload`.

Acceptance criteria:

- Up/down migration tersedia.
- Status mendukung `draft`, `active`, `inactive`, `archived`.
- Channel mendukung `email`, `whatsapp`, `in_app`, `discord`.

### NT-DB-002: Create `notification_logs`

Scope:

- Buat migration table `notification_logs`.
- Simpan rendered subject/body.
- Simpan provider response JSONB.
- Tambahkan retry fields.

Acceptance criteria:

- Status mendukung `pending`, `processing`, `sent`, `failed`, `cancelled`, `dead`.
- Recipient snapshot tersedia.
- Query retry bisa memakai index.

### NT-DB-003: Create `notification_preferences`

Scope:

- Buat migration table `notification_preferences`.
- Preference per user, organization, event, channel.

Acceptance criteria:

- Unique key tersedia.
- Default behavior enabled jika tidak ada preference.

## Phase 2 - Domain, DTO, Repository

### NT-BE-001: Domain Models

Scope:

- Buat domain template, log, preference, channel, variable.
- Tambahkan constants untuk channel/status.

Acceptance criteria:

- Model selaras migration.
- Tidak ada provider-specific logic.

### NT-BE-002: DTOs

Scope:

- Create/update template request.
- Preview request/response.
- Template response.
- Notification log response.
- Preference request.

Acceptance criteria:

- Request/response terpisah.
- Field internal provider tidak diekspos sembarang.

### NT-BE-003: Template Repository

Scope:

- CRUD template.
- Soft delete.
- Activate/deactivate/archive.
- Find active by code/channel/locale.

Acceptance criteria:

- Pagination tersedia.
- Active query ignore deleted template.

### NT-BE-004: Notification Log Repository

Scope:

- Create pending log.
- Mark processing/sent/failed/dead.
- Find retryable.

Acceptance criteria:

- Retry query memakai status dan `next_retry_at`.
- Provider response disimpan.

### NT-BE-005: Preference Repository

Scope:

- Get user preferences.
- Upsert preference.
- Bulk upsert.
- Check enabled.

Acceptance criteria:

- Upsert memakai unique key.
- Default enabled jika tidak ada row.

## Phase 3 - Template Engine

### NT-BE-010: Variable Registry

Scope:

- Registry variable per template code.
- Include auth/user/lead/payment/security/permission events.

Acceptance criteria:

- Variable punya key, description, required flag.

### NT-BE-011: Template Parser

Scope:

- Parse `{{variable}}`.
- Detect unknown/malformed variables.

Acceptance criteria:

- Duplicate variable dikembalikan sekali.

### NT-BE-012: Template Validator

Scope:

- Validate code, channel, locale, subject/body, variables.

Acceptance criteria:

- Email wajib subject.
- Empty body ditolak.

### NT-BE-013: Template Renderer

Scope:

- Render subject/body dari payload.
- Support preview.

Acceptance criteria:

- Missing required variable error.
- Optional variable boleh kosong.

## Phase 4 - Template Management API

### NT-BE-020: Template Service

Scope:

- Create/update/get/list/delete/preview/activate/deactivate/archive/clone.

Acceptance criteria:

- System template delete ditolak.
- Activate hanya untuk template valid.

### NT-BE-021: Template Handler

Endpoints:

```txt
GET    /admin/notification-templates
GET    /admin/notification-templates/:id
POST   /admin/notification-templates
PATCH  /admin/notification-templates/:id
DELETE /admin/notification-templates/:id
POST   /admin/notification-templates/:id/preview
POST   /admin/notification-templates/:id/activate
POST   /admin/notification-templates/:id/deactivate
POST   /admin/notification-templates/:id/archive
POST   /admin/notification-templates/:id/clone
```

Acceptance criteria:

- Response standar.
- Permission guard diterapkan.

### NT-BE-022: Variable Handler

Endpoints:

```txt
GET /admin/notification-template-variables
GET /admin/notification-template-variables/:code
```

Acceptance criteria:

- Protected by `notification_variable.read`.

## Phase 5 - Dispatcher and Provider Adapter

### NT-BE-030: Dispatcher Interface

Scope:

- Buat interface `Dispatcher`.
- Define `Message` dan `ProviderResult`.

Acceptance criteria:

- Dispatcher tidak tahu business module.

### NT-BE-031: Email Dispatcher

Scope:

- Call `internal/platform/mail`.
- Validate destination and subject.

Acceptance criteria:

- Provider error mapped.

### NT-BE-032: WhatsApp Dispatcher

Scope:

- Call `internal/platform/whatsapp`.
- Validate phone destination.

Acceptance criteria:

- Subject ignored.

### NT-BE-033: Noop Dispatcher

Scope:

- Simulate send for local/dev.

Acceptance criteria:

- Tidak call external provider.

### NT-BE-034: Discord Dispatcher Optional

Scope:

- Call optional Discord provider.
- Support webhook message.

Acceptance criteria:

- Disabled by default.
- Missing Discord credential tidak merusak local app.

### NT-PLAT-001: Mail Platform Interface

Scope:

- Buat mailer interface.
- Tambahkan noop mailer.
- SMTP implementation menyusul.

Acceptance criteria:

- Tidak ada business template logic di platform/mail.

### NT-PLAT-002: WhatsApp Platform Interface

Scope:

- Buat WhatsApp client interface.
- Tambahkan noop client.

Acceptance criteria:

- Provider bisa diganti.

### NT-PLAT-003: Discord Platform Interface Optional

Scope:

- Buat Discord client interface.
- Tambahkan noop client.

Acceptance criteria:

- Optional dan tidak memengaruhi channel lain.

## Phase 6 - Notification Service

### NT-BE-040: Notification Service

Scope:

- Send direct notification.
- Send by active template.
- Retry.
- Cancel.

Acceptance criteria:

- Log dibuat sebelum dispatch.
- Preference dicek sebelum send.
- Provider error mark failed.

### NT-BE-041: Notification Log Handler

Endpoints:

```txt
GET  /admin/notification-logs
GET  /admin/notification-logs/:id
POST /admin/notification-logs/:id/retry
POST /admin/notification-logs/:id/cancel
```

Acceptance criteria:

- Retry hanya failed/dead.
- Cancel hanya pending.

### NT-BE-042: Preference Service and Handler

Endpoints:

```txt
GET   /users/me/notification-preferences
PATCH /users/me/notification-preferences
GET   /admin/users/:userId/notification-preferences
PATCH /admin/users/:userId/notification-preferences
```

Acceptance criteria:

- User bisa manage preference sendiri.
- Admin butuh permission.

## Phase 7 - Event Consumer

### NT-BE-050: Notification Event Consumer

Scope:

- Consume event.
- Map event to rule.
- Build payload.
- Send notification.

Acceptance criteria:

- Unknown event ignored/logged.
- Error tidak crash worker.

### NT-BE-051: Rule Mapping Service

Scope:

- Hardcoded MVP mapping.
- Bisa dipindah ke DB nanti.

Acceptance criteria:

- Auth password reset dan invitation mapped.

### NT-BE-052: Outbox/Event Integration

Scope:

- Worker consume outbox/event.
- Business transaction tidak rollback karena notification failure.

Acceptance criteria:

- Idempotent jika event_id ada.

## Phase 8 - Seeder

### NT-SEED-001: Seed Default Templates

Templates:

```txt
auth.password_reset email id-ID
user.invitation email id-ID
lead.created whatsapp id-ID
payment.paid email id-ID
security.new_login email id-ID
permission.updated email id-ID
```

Acceptance criteria:

- Idempotent.
- Variables match registry.

### NT-SEED-002: Seed Notification Permissions

Permissions:

```txt
notification_template.*
notification_variable.read
notification_log.*
notification_preference.*
```

Acceptance criteria:

- Super admin gets permissions.
- Normal role tidak otomatis dapat manage permission.

## Phase 9 - Routing and Wiring

### NT-APP-001: Register Dependencies

Scope:

- Wire repository, service, dispatcher, handler.

Acceptance criteria:

- No cyclic dependency.

### NT-APP-002: Register Routes

Scope:

- Admin template/log routes.
- User preference routes.

Acceptance criteria:

- Permission middleware applied.

## Testing Strategy

### NT-TEST-001: Repository Integration Test Foundation

Scope:

- Tambahkan config `TEST_DB_*`.
- Tambahkan helper integration test database.
- Jalankan migration ke database test.
- Tambahkan integration test minimal untuk `TemplateRepository`.

Acceptance criteria:

- Integration test hanya berjalan dengan build tag `integration`.
- Cleanup hanya berjalan ke database test.
- `go test ./...` biasa tetap tidak membutuhkan database test.
- Template repository lifecycle terverifikasi terhadap PostgreSQL.

Minimal test:

- Template parser.
- Template validator.
- Template renderer.
- Template service.
- Notification service.
- API permission guard.
- Provider noop dispatcher.

## Definition of Done

- Migration added and tested.
- Code compiles.
- API follows contract.
- Permission guard applied.
- Validation works.
- Tests added or updated.
- No provider-specific logic leaks into core domain.
- Notification logs are created for sent/failed notification.
