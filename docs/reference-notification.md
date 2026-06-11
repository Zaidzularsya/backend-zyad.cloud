# Task Development — Notification & Template Management Module

Project: `zyad.cloud`  
Context: CRM / modular backend Go  
Module location: `internal/core/notification`  
Provider adapter location: `internal/platform/mail`, `internal/platform/whatsapp`, optional `internal/platform/broker`  
Migration location: `migrations/notification`

---

## 1. Objective

Membangun module **Notification & Template Management** yang bisa digunakan lintas module seperti auth, user, organization, order, billing, payment, lead, dan permission.

Module ini bertanggung jawab untuk:

- Mengelola template notifikasi.
- Render template berdasarkan variable.
- Validasi variable template.
- Mengirim notifikasi lewat channel email, WhatsApp, in-app, dan optional Discord.
- Menyimpan notification logs.
- Mendukung retry.
- Mendukung preference user.
- Mendukung event-driven notification melalui outbox/worker.
- Menjaga separation of concern antara core notification dan platform provider.

---

## 2. Architecture Decision

### 2.1 Placement

Notification sebagai **core capability**:

```txt
internal/core/notification
```

Provider teknis:

```txt
internal/platform/mail
internal/platform/whatsapp
internal/platform/broker
```

Migration:

```txt
migrations/core/notification
```

Worker:

```txt
cmd/worker
```

### 2.2 Responsibility Boundary

`core/notification`:

- Template CRUD.
- Template rendering.
- Notification rules.
- Notification log.
- Notification preference.
- Channel dispatching.
- Audit notification activity.
- API management endpoint.

`platform/mail`:

- SMTP/provider email client.
- Send email.
- Parse provider response.
- Return provider error.

`platform/whatsapp`:

- WhatsApp provider client.
- Send message/template message.
- Parse provider response.
- Return provider error.

`platform/broker`:

- Publish/consume event.
- RabbitMQ/Redis Streams/NATS implementation.
- No business logic.

---

## 3. Target Folder Structure

```txt
internal/
├── core/
│   └── notification/
│       ├── domain/
│       │   ├── notification.go
│       │   ├── notification_template.go
│       │   ├── notification_log.go
│       │   ├── notification_preference.go
│       │   ├── notification_channel.go
│       │   └── notification_variable.go
│       ├── dto/
│       │   ├── create_template_request.go
│       │   ├── update_template_request.go
│       │   ├── template_response.go
│       │   ├── preview_template_request.go
│       │   ├── preview_template_response.go
│       │   ├── send_notification_request.go
│       │   ├── notification_log_response.go
│       │   └── preference_request.go
│       ├── repository/
│       │   ├── template_repository.go
│       │   ├── notification_log_repository.go
│       │   └── preference_repository.go
│       ├── service/
│       │   ├── notification_service.go
│       │   ├── template_service.go
│       │   ├── template_renderer.go
│       │   ├── template_validator.go
│       │   ├── preference_service.go
│       │   └── notification_rule_service.go
│       ├── dispatcher/
│       │   ├── dispatcher.go
│       │   ├── email_dispatcher.go
│       │   ├── whatsapp_dispatcher.go
│       │   ├── in_app_dispatcher.go
│       │   └── noop_dispatcher.go
│       ├── template/
│       │   ├── engine.go
│       │   ├── variable_registry.go
│       │   └── parser.go
│       ├── handler/
│       │   ├── template_handler.go
│       │   ├── notification_log_handler.go
│       │   └── preference_handler.go
│       ├── consumer/
│       │   └── notification_event_consumer.go
│       ├── seeder/
│       │   └── notification_template_seeder.go
│       └── route.go
├── platform/
│   ├── mail/
│   │   ├── mailer.go
│   │   ├── smtp_mailer.go
│   │   ├── noop_mailer.go
│   │   └── config.go
│   └── whatsapp/
│       ├── client.go
│       ├── noop_client.go
│       └── config.go
```

---

## 4. Database Design

### 4.1 `notification_templates`

```sql
CREATE TABLE notification_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(150) NOT NULL,
    name VARCHAR(150) NOT NULL,
    description TEXT NULL,
    channel VARCHAR(50) NOT NULL,
    locale VARCHAR(20) NOT NULL DEFAULT 'id-ID',
    subject_template TEXT NULL,
    body_template TEXT NOT NULL,
    available_variables JSONB NOT NULL DEFAULT '[]'::jsonb,
    sample_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    version INT NOT NULL DEFAULT 1,
    created_by UUID NULL,
    updated_by UUID NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    deleted_at TIMESTAMP NULL,
    UNIQUE(code, channel, locale, version)
);
```

Allowed `channel`:

```txt
email
whatsapp
in_app
discord
```

Allowed `status`:

```txt
draft
active
inactive
archived
```

### 4.2 `notification_logs`

```sql
CREATE TABLE notification_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NULL,
    event_type VARCHAR(150) NULL,
    template_id UUID NULL,
    template_code VARCHAR(150) NULL,
    template_version INT NULL,
    organization_id UUID NULL,
    channel VARCHAR(50) NOT NULL,
    recipient_type VARCHAR(50) NOT NULL DEFAULT 'user',
    recipient_user_id UUID NULL,
    recipient_name_snapshot VARCHAR(150) NULL,
    recipient_email_snapshot VARCHAR(150) NULL,
    recipient_phone_snapshot VARCHAR(50) NULL,
    destination VARCHAR(255) NOT NULL,
    subject TEXT NULL,
    body TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    provider VARCHAR(100) NULL,
    provider_message_id VARCHAR(255) NULL,
    provider_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMP NULL,
    error_message TEXT NULL,
    sent_at TIMESTAMP NULL,
    failed_at TIMESTAMP NULL,
    cancelled_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
```

Allowed `status`:

```txt
pending
processing
sent
failed
cancelled
dead
```

### 4.3 `notification_preferences`

```sql
CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    organization_id UUID NULL,
    event_type VARCHAR(150) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE(user_id, organization_id, event_type, channel)
);
```

### 4.4 `notification_template_versions` Optional Enterprise

Untuk MVP, table ini boleh ditunda.

```sql
CREATE TABLE notification_template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES notification_templates(id) ON DELETE CASCADE,
    version INT NOT NULL,
    subject_template TEXT NULL,
    body_template TEXT NOT NULL,
    available_variables JSONB NOT NULL DEFAULT '[]'::jsonb,
    sample_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    change_note TEXT NULL,
    created_by UUID NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE(template_id, version)
);
```

---

## 5. Index Requirements

```sql
CREATE INDEX idx_notification_templates_code_channel_locale ON notification_templates(code, channel, locale);
CREATE INDEX idx_notification_templates_status ON notification_templates(status);
CREATE INDEX idx_notification_templates_is_active ON notification_templates(is_active);
CREATE INDEX idx_notification_logs_event_type ON notification_logs(event_type);
CREATE INDEX idx_notification_logs_template_code ON notification_logs(template_code);
CREATE INDEX idx_notification_logs_channel_status ON notification_logs(channel, status);
CREATE INDEX idx_notification_logs_recipient_user_id ON notification_logs(recipient_user_id);
CREATE INDEX idx_notification_logs_created_at ON notification_logs(created_at);
CREATE INDEX idx_notification_logs_retry ON notification_logs(status, next_retry_at);
CREATE INDEX idx_notification_preferences_user_event_channel ON notification_preferences(user_id, event_type, channel);
```

---

## 6. API Contract

### 6.1 Template Management API

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

### 6.2 Template Variables API

```txt
GET /admin/notification-template-variables
GET /admin/notification-template-variables/:code
```

### 6.3 Notification Logs API

```txt
GET  /admin/notification-logs
GET  /admin/notification-logs/:id
POST /admin/notification-logs/:id/retry
POST /admin/notification-logs/:id/cancel
```

### 6.4 Notification Preferences API

```txt
GET   /users/me/notification-preferences
PATCH /users/me/notification-preferences
GET   /admin/users/:userId/notification-preferences
PATCH /admin/users/:userId/notification-preferences
```

### 6.5 Internal Send API Optional

```txt
POST /internal/notifications/send
```

---

## 7. Permission Requirements

Tambahkan permission berikut ke Permission Management:

```txt
notification_template.read
notification_template.create
notification_template.update
notification_template.delete
notification_template.preview
notification_template.activate
notification_template.deactivate
notification_template.archive
notification_template.clone
notification_variable.read
notification_log.read
notification_log.retry
notification_log.cancel
notification_preference.read
notification_preference.update
notification_preference.manage
```

Sensitive permission:

```txt
notification_template.update
notification_template.delete
notification_template.activate
notification_template.archive
notification_log.retry
```

Role yang disarankan:

```txt
Super Admin
CRM Admin
System Admin
```

Sales, Support, Finance tidak otomatis boleh edit template.

---

## 8. Template Code Standard

Gunakan format:

```txt
domain.action
```

Contoh standard:

```txt
auth.password_reset
auth.email_verification
auth.otp_login
security.new_login
user.invitation
user.account_activated
user.account_suspended
lead.created
lead.assigned
order.created
order.completed
order.cancelled
invoice.created
invoice.sent
invoice.paid
payment.paid
payment.failed
permission.updated
```

Hindari:

```txt
email1
template_baru
pesan_reset
notif_order
```

---

## 9. Variable Registry

Variable registry wajib dibuat agar template tidak memakai variable liar.

### 9.1 Example Registry

```txt
auth.password_reset:
- app_name required
- user_name required
- reset_url required
- expired_at required

user.invitation:
- app_name required
- inviter_name required
- invitee_email required
- organization_name required
- invitation_url required
- expired_at required

lead.created:
- app_name required
- lead_name required
- lead_source required
- lead_phone optional
- assigned_sales_name optional
- crm_url required

payment.paid:
- app_name required
- customer_name required
- invoice_number required
- amount required
- paid_at required
```

### 9.2 Required Validation

Saat create/update template:

- Variable yang digunakan di subject/body harus ada di registry.
- Required variable harus tersedia dalam `available_variables`.
- `body_template` tidak boleh kosong.
- `subject_template` wajib untuk channel `email`.
- `subject_template` boleh null untuk `whatsapp`, `in_app`, `discord`.
- Channel harus valid.
- Locale harus valid.
- Code harus mengikuti standard `domain.action`.

---

## 10. Template Rendering Rules

Format variable:

```txt
{{user_name}}
{{reset_url}}
{{organization_name}}
```

Rules:

- Unknown variable harus error saat validasi.
- Missing required variable saat render harus error.
- Optional variable yang kosong boleh dirender sebagai string kosong.
- Jangan eksekusi expression atau function dari template untuk menghindari security issue.
- Renderer hanya melakukan replacement variable sederhana.
- Escape/cleaning HTML perlu dipertimbangkan jika channel email mendukung HTML.

---

## 11. Notification Sending Flow

### 11.1 Direct Flow

```txt
Module Service
-> NotificationService.Send()
-> TemplateService.FindActiveTemplate()
-> TemplateRenderer.Render()
-> NotificationLogRepository.CreatePending()
-> Dispatcher.Send()
-> NotificationLogRepository.MarkSent/MarkFailed()
```

### 11.2 Event-Driven Flow

```txt
Business Module
-> insert outbox_events
-> Outbox Worker publish event
-> Notification Event Consumer receive event
-> NotificationService.Send()
-> Dispatcher
-> notification_logs
```

---

## 12. Notification Rule Mapping

Untuk MVP, rule mapping boleh hardcoded dulu di `notification_rule_service.go`.

Example:

```txt
auth.password_reset_requested:
- template_code: auth.password_reset
- channel: email
- recipient: requested user

user.invited:
- template_code: user.invitation
- channel: email
- recipient: invited email

lead.created:
- template_code: lead.created
- channel: whatsapp
- recipient: assigned sales / admin group

payment.paid:
- template_code: payment.paid
- channel: email
- recipient: customer / finance
```

Nanti enterprise bisa dipindah ke database:

```txt
notification_rules
notification_rule_channels
notification_rule_recipients
```

---

## 13. Development Phases

## Phase 1 — Database Foundation

### NT-DB-001 Create `notification_templates` migration

Output:

- `migrations/core/notification/001_create_notification_templates.up.sql`
- `migrations/core/notification/001_create_notification_templates.down.sql`

Acceptance criteria:

- Table created successfully.
- Unique constraint `(code, channel, locale, version)` exists.
- `available_variables` and `sample_payload` use JSONB.
- `status`, `is_system`, `is_active`, `version` exist.
- Down migration drops table safely.

### NT-DB-002 Create `notification_logs` migration

Output:

- `002_create_notification_logs.up.sql`
- `002_create_notification_logs.down.sql`

Acceptance criteria:

- Table created successfully.
- Status supports pending, processing, sent, failed, cancelled, dead.
- Retry fields exist: attempts, max_attempts, next_retry_at.
- Provider response stored as JSONB.
- Recipient snapshot fields exist.

### NT-DB-003 Create `notification_preferences` migration

Output:

- `003_create_notification_preferences.up.sql`
- `003_create_notification_preferences.down.sql`

Acceptance criteria:

- Unique constraint `(user_id, organization_id, event_type, channel)` exists.
- User preference can be enabled/disabled per event and channel.

### NT-DB-004 Add indexes

Acceptance criteria:

- Indexes for code/channel/locale.
- Indexes for logs channel/status.
- Index for retry query.
- Index for recipient user.

---

## Phase 2 — Domain, DTO, Repository

### NT-BE-001 Create domain models

Files:

```txt
internal/core/notification/domain/notification_template.go
internal/core/notification/domain/notification_log.go
internal/core/notification/domain/notification_preference.go
internal/core/notification/domain/notification_channel.go
```

Acceptance criteria:

- Domain model matches database schema.
- Constants exist for channel and status.
- No provider-specific logic in domain model.

### NT-BE-002 Create DTOs

Files:

```txt
dto/create_template_request.go
dto/update_template_request.go
dto/template_response.go
dto/preview_template_request.go
dto/preview_template_response.go
dto/notification_log_response.go
dto/preference_request.go
```

Acceptance criteria:

- DTO has validation tags if project uses validator.
- Request and response are separated.
- Internal fields like provider raw error are not exposed unnecessarily.

### NT-BE-003 Create template repository

Methods:

```go
Create(ctx, template)
FindByID(ctx, id)
FindActiveByCodeChannelLocale(ctx, code, channel, locale)
List(ctx, filter)
Update(ctx, template)
SoftDelete(ctx, id)
Activate(ctx, id)
Deactivate(ctx, id)
Archive(ctx, id)
```

Acceptance criteria:

- Repository supports pagination.
- Soft delete is used.
- System template delete is blocked at service layer.
- Active template query ignores deleted template.

### NT-BE-004 Create notification log repository

Methods:

```go
Create(ctx, log)
FindByID(ctx, id)
List(ctx, filter)
MarkProcessing(ctx, id)
MarkSent(ctx, id, providerResponse)
MarkFailed(ctx, id, error)
MarkDead(ctx, id, error)
FindRetryable(ctx, limit)
```

Acceptance criteria:

- Retryable query uses `status` and `next_retry_at`.
- Log stores provider response.
- Log stores rendered subject/body.

### NT-BE-005 Create preference repository

Methods:

```go
GetUserPreferences(ctx, userID, organizationID)
UpsertPreference(ctx, preference)
BulkUpsertPreferences(ctx, preferences)
IsEnabled(ctx, userID, organizationID, eventType, channel)
```

Acceptance criteria:

- Upsert uses unique key.
- Default behavior: notification enabled if no explicit preference exists.

---

## Phase 3 — Template Engine & Validation

### NT-BE-010 Create variable registry

File:

```txt
internal/core/notification/template/variable_registry.go
```

Acceptance criteria:

- Registry maps `template_code` to allowed variables.
- Each variable has key, description, required flag.
- Registry includes auth.password_reset, user.invitation, lead.created, payment.paid.

### NT-BE-011 Create template parser

Responsibilities:

- Extract variables from subject/body.
- Detect unknown variables.
- Detect duplicate variables.
- Return clean list of variable keys.

Acceptance criteria:

- Parser can detect `{{user_name}}`.
- Parser ignores normal text.
- Parser rejects malformed variable syntax if needed.

### NT-BE-012 Create template validator

Responsibilities:

- Validate code format.
- Validate channel.
- Validate locale.
- Validate subject requirement.
- Validate used variables against registry.
- Validate required variables.

Acceptance criteria:

- Unknown variable returns validation error.
- Email without subject returns validation error.
- Empty body returns validation error.
- Invalid code format returns validation error.

### NT-BE-013 Create template renderer

Responsibilities:

- Render `{{variable}}` using payload.
- Return rendered subject/body.
- Error if required variable missing.
- Optional variable can be empty.

Acceptance criteria:

- Preview works with sample payload.
- Missing required variable returns error.
- Unknown variable cannot be rendered if template validation is enforced.

---

## Phase 4 — Template Management Service & API

### NT-BE-020 Create TemplateService

Methods:

```go
CreateTemplate(ctx, req)
UpdateTemplate(ctx, id, req)
GetTemplate(ctx, id)
ListTemplates(ctx, filter)
DeleteTemplate(ctx, id)
PreviewTemplate(ctx, id, payload)
ActivateTemplate(ctx, id)
DeactivateTemplate(ctx, id)
ArchiveTemplate(ctx, id)
CloneTemplate(ctx, id)
```

Acceptance criteria:

- Create/update validates template.
- Delete blocks system template.
- Activate ensures template is valid.
- Preview uses provided payload or sample_payload.
- Clone creates new draft version or new code variant.

### NT-BE-021 Create TemplateHandler

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

- All endpoints use standardized response.
- All endpoints protected by permission middleware.
- Errors are consistent.
- Pagination supported on list.

### NT-BE-022 Create VariableHandler

Endpoint:

```txt
GET /admin/notification-template-variables
GET /admin/notification-template-variables/:code
```

Acceptance criteria:

- Returns available variables per template code.
- Returns required/optional flag.
- Protected by `notification_variable.read`.

---

## Phase 5 — Dispatcher & Provider Adapter

### NT-BE-030 Create Dispatcher interface

File:

```txt
internal/core/notification/dispatcher/dispatcher.go
```

Interface:

```go
type Dispatcher interface {
    Channel() string
    Send(ctx context.Context, message Message) (ProviderResult, error)
}
```

Acceptance criteria:

- Dispatcher does not know business module.
- Dispatcher returns provider message ID and raw response.

### NT-BE-031 Create EmailDispatcher

Responsibilities:

- Receive rendered email.
- Call `platform/mail`.
- Return provider result.

Acceptance criteria:

- Email destination required.
- Subject required.
- Mail provider errors are mapped.

### NT-BE-032 Create WhatsAppDispatcher

Responsibilities:

- Receive rendered message.
- Call `platform/whatsapp`.
- Return provider result.

Acceptance criteria:

- Phone destination required.
- Subject ignored.
- Provider errors are mapped.

### NT-BE-033 Create NoopDispatcher for local/dev

Responsibilities:

- Simulate send.
- Log message only.
- Return success.

Acceptance criteria:

- Useful in development.
- Does not call external provider.

### NT-PLAT-001 Create platform mail interface

File:

```txt
internal/platform/mail/mailer.go
```

Acceptance criteria:

- Interface can send email with to, subject, body.
- SMTP implementation can be added without changing core notification.
- No business template logic in platform/mail.

### NT-PLAT-002 Create platform WhatsApp interface

File:

```txt
internal/platform/whatsapp/client.go
```

Acceptance criteria:

- Interface can send WhatsApp text.
- Provider implementation can be swapped.
- No notification rule in platform/whatsapp.

---

## Phase 6 — Notification Service

### NT-BE-040 Create NotificationService

Methods:

```go
Send(ctx, request)
SendByTemplate(ctx, code, channel, locale, recipient, payload)
Retry(ctx, notificationLogID)
Cancel(ctx, notificationLogID)
```

Acceptance criteria:

- Finds active template.
- Checks user preference before sending.
- Renders template.
- Creates pending log before dispatch.
- Marks log sent/failed.
- Handles provider error.
- Does not panic if provider unavailable.

### NT-BE-041 Create NotificationLogHandler

Endpoints:

```txt
GET  /admin/notification-logs
GET  /admin/notification-logs/:id
POST /admin/notification-logs/:id/retry
POST /admin/notification-logs/:id/cancel
```

Acceptance criteria:

- List supports filter channel/status/event_type/date.
- Retry only allowed for failed/dead logs.
- Cancel only allowed for pending logs.
- Protected by permission middleware.

### NT-BE-042 Create PreferenceService and PreferenceHandler

Endpoints:

```txt
GET   /users/me/notification-preferences
PATCH /users/me/notification-preferences
GET   /admin/users/:userId/notification-preferences
PATCH /admin/users/:userId/notification-preferences
```

Acceptance criteria:

- User can manage own preference.
- Admin can manage user preference with permission.
- Default is enabled when no preference exists.

---

## Phase 7 — Event Consumer Integration

### NT-BE-050 Create notification event consumer

File:

```txt
internal/core/notification/consumer/notification_event_consumer.go
```

Responsibilities:

- Receive event.
- Map event to notification rule.
- Build payload.
- Send notification.

Acceptance criteria:

- Consumer handles known events.
- Unknown event is ignored or logged.
- Consumer is idempotent if event_id is available.
- Errors do not crash worker.

### NT-BE-051 Create rule mapping service

File:

```txt
internal/core/notification/service/notification_rule_service.go
```

Acceptance criteria:

- Maps event type to template code and channel.
- MVP can be hardcoded.
- Can be moved to DB later.

### NT-BE-052 Integrate with outbox/event system

Acceptance criteria:

- Auth/user/lead/order/payment can publish events.
- Notification worker can consume events.
- Notification failure does not rollback original business transaction.

---

## Phase 8 — Seeder

### NT-SEED-001 Seed default templates

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

- Seeder is idempotent.
- Existing system templates are not duplicated.
- Template variables match registry.
- Sample payload can preview successfully.

### NT-SEED-002 Seed default notification permissions

Permissions:

```txt
notification_template.*
notification_variable.read
notification_log.*
notification_preference.*
```

Acceptance criteria:

- Permissions inserted idempotently.
- CRM Admin/Super Admin get proper permissions.
- Normal sales role does not get template update permission.

---

## Phase 9 — Routing & Dependency Wiring

### NT-APP-001 Register dependencies

File:

```txt
internal/app/dependency.go
```

Acceptance criteria:

- Repository, service, dispatcher, handler are wired.
- No cyclic dependency.
- Platform adapter injected through interfaces.

### NT-APP-002 Register routes

File:

```txt
internal/app/module_routes.go
```

Acceptance criteria:

- Admin routes registered.
- User preference routes registered.
- Permission middleware applied.
- Route group naming consistent.

### NT-APP-003 Config updates

Files:

```txt
internal/config/config.go
internal/config/load.go
```

Environment variables:

```env
NOTIFICATION_DEFAULT_LOCALE=id-ID
NOTIFICATION_PROVIDER_MODE=noop
SMTP_HOST=
SMTP_PORT=
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM_EMAIL=
SMTP_FROM_NAME=
WHATSAPP_PROVIDER=noop
WHATSAPP_API_KEY=
WHATSAPP_SENDER=
```

Acceptance criteria:

- App can boot with noop provider.
- Missing external provider config does not break local development when noop mode is used.

---

## Phase 10 — Testing

### NT-TEST-001 Template parser tests

Cases:

- Extract single variable.
- Extract multiple variables.
- Extract duplicate variable once.
- Ignore normal text.
- Detect malformed variable if supported.

### NT-TEST-002 Template validator tests

Cases:

- Reject unknown variable.
- Reject email template without subject.
- Reject empty body.
- Reject invalid channel.
- Reject invalid code format.
- Accept valid template.

### NT-TEST-003 Template renderer tests

Cases:

- Render subject and body.
- Error when required variable missing.
- Optional variable empty is allowed.
- Sample payload preview works.

### NT-TEST-004 Template service tests

Cases:

- Create template success.
- Update template success.
- Delete system template denied.
- Activate invalid template denied.
- Preview template with provided payload.
- Preview template with sample payload.

### NT-TEST-005 Notification service tests

Cases:

- Send email success.
- Send WhatsApp success.
- Provider error marks log failed.
- Missing active template returns error.
- Preference disabled skips send or marks cancelled.
- Retry failed notification.

### NT-TEST-006 API tests

Cases:

- List templates with pagination.
- Create template requires permission.
- Preview template requires permission.
- Notification log retry requires permission.
- User can update own preference.
- User cannot update others preference without admin permission.

---

## Phase 11 — Frontend Task Breakdown

If frontend is included, pages:

```txt
/admin/notification-templates
/admin/notification-templates/create
/admin/notification-templates/:id/edit
/admin/notification-templates/:id/preview
/admin/notification-logs
/profile/notification-preferences
```

### NT-FE-001 Template list page

Features:

- Search by code/name.
- Filter by channel.
- Filter by status.
- Filter by locale.
- Create button.
- Action: edit, preview, activate, deactivate, clone, archive.

Acceptance criteria:

- Table uses API pagination.
- Actions respect permissions.
- System template shows protected badge.

### NT-FE-002 Template editor page

Features:

- Code input.
- Name input.
- Channel dropdown.
- Locale dropdown.
- Subject editor.
- Body editor.
- Available variable helper.
- Sample payload JSON editor.
- Save draft.
- Activate.

Acceptance criteria:

- Email requires subject.
- Body required.
- Variable helper displays allowed variables.
- Validation errors displayed clearly.

### NT-FE-003 Template preview panel

Features:

- Render subject/body.
- Use sample payload.
- Allow custom payload.
- Show validation/render error.

Acceptance criteria:

- Preview does not save template.
- Preview result matches backend renderer.

### NT-FE-004 Notification log page

Features:

- Filter by status/channel/event.
- Detail log.
- Retry failed log.
- Cancel pending log.
- View provider response.

Acceptance criteria:

- Retry action only available for failed/dead.
- Cancel action only available for pending.
- Sensitive provider response hidden unless permitted.

### NT-FE-005 Notification preference page

Features:

- User can enable/disable event per channel.
- Admin can manage user preference.

Acceptance criteria:

- Save preference via API.
- Default enabled state displayed properly.

---

## 14. Security Requirements

### NT-SEC-001 Permission Guard

Every admin endpoint must be protected:

```txt
notification_template.read
notification_template.create
notification_template.update
notification_template.delete
notification_template.preview
notification_template.activate
notification_template.deactivate
notification_template.archive
notification_template.clone
notification_log.read
notification_log.retry
notification_log.cancel
notification_preference.manage
```

### NT-SEC-002 System Template Protection

Rules:

- System template cannot be deleted.
- System template can only be updated by privileged role.
- Archive system template should require high permission.

### NT-SEC-003 No Secret in Template

Templates must not store:

```txt
password
raw token
API key
secret key
provider credential
```

Reset URL may contain token, but token is runtime payload, not stored permanently in template.

### NT-SEC-004 Audit Log

Every sensitive action should create audit log:

```txt
notification_template.created
notification_template.updated
notification_template.deleted
notification_template.activated
notification_template.deactivated
notification_template.archived
notification_template.cloned
notification_log.retry_requested
notification_log.cancelled
notification_preference.updated
```

### NT-SEC-005 Rate Limiting

Recommended:

- Retry endpoint should be rate limited.
- Send endpoint if exposed should be internal-only or protected.
- Password reset notification should be rate limited at auth layer.

---

## 15. Definition of Done

A task is done only when:

- Migration added and tested.
- Code compiles.
- API follows contract.
- Permission guard applied.
- Validation works.
- Tests added or updated.
- No provider-specific logic leaks into core domain.
- No business logic exists in platform adapter.
- Notification logs are created for sent/failed notification.
- Documentation updated if behavior changes.

---

## 16. Suggested Implementation Order

Follow this exact order for stable vibe coding:

```txt
1. Database migrations
2. Domain models
3. DTOs
4. Repositories
5. Variable registry
6. Template parser
7. Template validator
8. Template renderer
9. Template service
10. Template API
11. Platform mail noop adapter
12. Dispatcher interface
13. Email dispatcher
14. Notification log repository
15. Notification service
16. Notification log API
17. Preference repository/service/API
18. Seed default templates
19. Seed permissions
20. Route and dependency wiring
21. Unit tests
22. API tests
23. Worker/event consumer integration
24. Optional WhatsApp dispatcher
25. Optional frontend
```

---

## 17. Prompt for Coding Agent

Use this prompt when starting development:

```txt
Kamu adalah backend engineer untuk project zyad.cloud.

Saya ingin membangun module Notification & Template Management.

Sebelum coding, baca dokumen task development ini dan ikuti urutan phase.

Architecture decision:
- Core logic di internal/core/notification.
- Provider teknis di internal/platform/mail dan internal/platform/whatsapp.
- Migration di migrations/core/notification.
- Jangan taruh business logic di platform adapter.
- Jangan membuat modules/notification kecuali nanti fitur broadcast/campaign menjadi business module tersendiri.

Mulai dari Phase 1:
1. Buat migration notification_templates.
2. Buat migration notification_logs.
3. Buat migration notification_preferences.
4. Tambahkan index sesuai dokumen.
5. Jangan implementasi API dulu sebelum migration selesai.

Aturan:
- Template variable harus divalidasi.
- Email template wajib subject.
- Template body wajib ada.
- System template tidak boleh dihapus.
- Admin endpoint wajib permission guard.
- Notification log harus tersimpan untuk setiap send.
- Gunakan noop provider agar local development tidak bergantung SMTP/WA.
- Jangan refactor module lain di luar kebutuhan wiring.
```

---

## 18. MVP Scope

MVP includes:

```txt
notification_templates
notification_logs
notification_preferences
template CRUD
template preview
template variable registry
template validation
template rendering
noop email dispatcher
notification service
notification log
seed auth.password_reset
seed user.invitation
seed lead.created
permission guard
basic tests
```

MVP excludes:

```txt
template approval workflow
full versioning table
A/B testing
scheduled broadcast
campaign management
WhatsApp inbox
drag-and-drop editor
complex notification rules stored in DB
```

---

## 19. Future Enhancement

After MVP stable:

```txt
notification_template_versions
template approval workflow
notification_rules table
notification_rule_recipients table
broadcast campaign module
scheduled notification
multi-language template management
HTML email editor
provider failover
webhook delivery logs
in-app notification center
user notification inbox
```

---

## 20. Final Notes

This module should be treated as a core capability.

Correct dependency direction:

```txt
modules/* -> core/event
modules/* -> core/notification interface optional
core/notification -> platform/mail
core/notification -> platform/whatsapp
core/notification -> platform/broker
```

Avoid:

```txt
platform/mail -> modules/*
platform/whatsapp -> modules/*
core/notification -> modules/order concrete package
core/notification -> modules/payment concrete package
```

If notification becomes a product feature such as broadcast campaign, WhatsApp inbox, marketing automation, or notification center, create a separate business module later:

```txt
internal/modules/notification_campaign
```

Do not mix campaign product logic with core notification delivery.
