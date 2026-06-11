Berikut breakdown **feature Permission Management** untuk konteks **CRM**, bukan CMS.

Permission Management adalah modul untuk mengatur **siapa boleh melakukan apa**, terhadap **data CRM apa**, dan dalam **cakupan akses apa**.

---

# Permission Management CRM

## 1. Tujuan Modul Permission Management

Modul ini dipakai untuk mengatur akses user terhadap fitur CRM seperti:

```txt
Lead
Customer
Contact
Company
Deal / Opportunity
Pipeline
Activity
Task
Note
Document
Quotation
Invoice
Report
User
Team
Role
Setting
Integration
```

Contoh kebutuhan nyata:

```txt
Sales hanya boleh melihat lead miliknya sendiri.

Sales Manager boleh melihat lead milik semua sales di timnya.

Admin boleh melihat semua data CRM.

Finance hanya boleh melihat quotation, invoice, dan payment.

Marketing boleh membuat lead, tapi tidak boleh menghapus customer.

Customer Support boleh melihat customer dan activity, tapi tidak boleh melihat deal value.
```

---

# 2. Konsep Permission CRM

Permission sebaiknya tidak hanya berupa role seperti `admin`, `sales`, atau `manager`.

Yang lebih kuat adalah:

```txt
User
  -> Role
    -> Permission
      -> Scope
        -> Condition
```

Contoh:

```txt
User: Budi
Role: Sales Executive
Permission: lead.read
Scope: own
Condition: hanya lead yang assigned_to = Budi
```

Contoh lain:

```txt
User: Sari
Role: Sales Manager
Permission: deal.read
Scope: team
Condition: hanya deal milik anggota tim Sari
```

---

# 3. Struktur Permission

Format permission yang disarankan:

```txt
module.action
```

Contoh:

```txt
lead.read
lead.create
lead.update
lead.delete
lead.assign
lead.convert

customer.read
customer.create
customer.update
customer.delete

deal.read
deal.create
deal.update
deal.delete
deal.move_stage
deal.close_won
deal.close_lost

report.sales.read
report.finance.read

user.read
user.create
user.update
user.delete

role.manage
permission.manage
system.setting.update
```

---

# 4. Permission Scope

Scope adalah batas jangkauan data yang boleh diakses.

Ini sangat penting untuk CRM.

| Scope          | Fungsi                                             |
| -------------- | -------------------------------------------------- |
| `none`         | Tidak punya akses                                  |
| `own`          | Hanya data milik sendiri                           |
| `team`         | Data milik anggota tim                             |
| `branch`       | Data dalam cabang tertentu                         |
| `department`   | Data dalam departemen tertentu                     |
| `organization` | Semua data dalam organisasi                        |
| `all`          | Semua data lintas organisasi, biasanya super admin |

Contoh penerapan:

```txt
lead.read:own
lead.update:own

deal.read:team
deal.update:team

customer.read:organization

user.manage:organization
```

---

# 5. Action Permission

Setiap modul CRM biasanya butuh action seperti ini:

```txt
read
create
update
delete
restore
export
import
assign
approve
reject
archive
merge
transfer_owner
```

Namun tidak semua modul perlu semua action.

---

# 6. Permission per Modul CRM

## A. Lead Permission

```txt
lead.read
lead.create
lead.update
lead.delete
lead.restore
lead.assign
lead.convert
lead.import
lead.export
lead.merge
lead.change_status
lead.change_source
lead.transfer_owner
```

Contoh aturan:

```txt
Sales:
- lead.read:own
- lead.create:own
- lead.update:own
- lead.convert:own

Sales Manager:
- lead.read:team
- lead.update:team
- lead.assign:team
- lead.convert:team
- lead.export:team

CRM Admin:
- lead.read:organization
- lead.create:organization
- lead.update:organization
- lead.delete:organization
- lead.import:organization
- lead.export:organization
```

---

## B. Customer Permission

```txt
customer.read
customer.create
customer.update
customer.delete
customer.restore
customer.archive
customer.merge
customer.export
customer.import
customer.transfer_owner
```

Contoh aturan:

```txt
Sales:
- customer.read:own
- customer.update:own

Customer Support:
- customer.read:organization
- customer.update:organization

Finance:
- customer.read:organization

CRM Admin:
- customer.manage:organization
```

---

## C. Contact Permission

```txt
contact.read
contact.create
contact.update
contact.delete
contact.restore
contact.export
contact.import
contact.merge
```

Contoh:

```txt
Sales boleh membuat dan update contact miliknya.

Support boleh membaca contact customer.

Marketing boleh import contact dari campaign.

Admin boleh manage semua contact.
```

---

## D. Company / Account Permission

```txt
company.read
company.create
company.update
company.delete
company.restore
company.archive
company.merge
company.export
company.import
company.transfer_owner
```

Dalam CRM B2B, company sering disebut juga:

```txt
Account
Organization
Client Company
```

Saran: pilih satu istilah agar konsisten. Untuk CRM B2B, nama `company` atau `account` lebih cocok.

---

## E. Deal / Opportunity Permission

```txt
deal.read
deal.create
deal.update
deal.delete
deal.restore
deal.assign
deal.move_stage
deal.close_won
deal.close_lost
deal.reopen
deal.export
deal.transfer_owner
deal.view_value
deal.update_value
deal.view_margin
deal.approve_discount
```

Permission khusus deal yang penting:

```txt
deal.view_value
deal.update_value
deal.view_margin
deal.approve_discount
```

Karena tidak semua user boleh melihat nilai deal, margin, diskon, atau profit.

Contoh:

```txt
Sales:
- deal.read:own
- deal.create:own
- deal.update:own
- deal.move_stage:own
- deal.view_value:own

Sales Manager:
- deal.read:team
- deal.update:team
- deal.move_stage:team
- deal.view_value:team
- deal.approve_discount:team

Finance:
- deal.read:organization
- deal.view_value:organization
- deal.view_margin:organization
```

---

## F. Pipeline Permission

```txt
pipeline.read
pipeline.create
pipeline.update
pipeline.delete
pipeline.configure_stage
pipeline.reorder_stage
pipeline.archive
```

Biasanya hanya admin atau sales ops yang boleh mengatur pipeline.

Contoh:

```txt
Sales:
- pipeline.read:organization

Sales Manager:
- pipeline.read:organization

CRM Admin:
- pipeline.manage:organization
```

---

## G. Activity Permission

Activity bisa berupa call, meeting, email, WhatsApp, follow up, visit, demo, dan sebagainya.

```txt
activity.read
activity.create
activity.update
activity.delete
activity.restore
activity.complete
activity.cancel
activity.assign
```

Contoh:

```txt
Sales:
- activity.read:own
- activity.create:own
- activity.update:own
- activity.complete:own

Manager:
- activity.read:team
- activity.assign:team

Admin:
- activity.manage:organization
```

---

## H. Task Permission

```txt
task.read
task.create
task.update
task.delete
task.assign
task.complete
task.reopen
task.change_due_date
```

Contoh:

```txt
Sales bisa membuat task untuk dirinya sendiri.

Manager bisa assign task ke anggota tim.

Admin bisa melihat semua task organisasi.
```

---

## I. Note Permission

```txt
note.read
note.create
note.update
note.delete
note.private.read
note.private.create
```

Untuk CRM, note bisa sensitif.

Contoh:

```txt
Sales boleh melihat note biasa pada lead miliknya.

Manager boleh melihat note tim.

Private note hanya bisa dibaca pembuat note atau role tertentu.
```

---

## J. Document Permission

```txt
document.read
document.upload
document.update
document.delete
document.download
document.share
document.private.read
```

Contoh:

```txt
Sales boleh upload dokumen untuk customer miliknya.

Finance boleh melihat dokumen invoice/payment.

Admin boleh menghapus dokumen.
```

---

## K. Quotation Permission

```txt
quotation.read
quotation.create
quotation.update
quotation.delete
quotation.send
quotation.approve
quotation.reject
quotation.export_pdf
quotation.view_price
quotation.update_price
quotation.approve_discount
```

Contoh:

```txt
Sales:
- quotation.create:own
- quotation.update:own
- quotation.send:own

Manager:
- quotation.approve:team
- quotation.reject:team
- quotation.approve_discount:team

Finance:
- quotation.read:organization
- quotation.view_price:organization
```

---

## L. Invoice Permission

```txt
invoice.read
invoice.create
invoice.update
invoice.delete
invoice.send
invoice.mark_paid
invoice.cancel
invoice.export_pdf
invoice.view_amount
invoice.update_amount
```

Biasanya invoice lebih cocok diakses oleh finance.

Contoh:

```txt
Sales:
- invoice.read:own

Finance:
- invoice.manage:organization

Admin:
- invoice.manage:organization
```

---

## M. Report Permission

```txt
report.read
report.sales.read
report.marketing.read
report.finance.read
report.activity.read
report.team_performance.read
report.export
report.dashboard.read
```

Contoh aturan:

```txt
Sales:
- report.dashboard.read:own
- report.sales.read:own

Manager:
- report.dashboard.read:team
- report.sales.read:team
- report.team_performance.read:team

Director:
- report.dashboard.read:organization
- report.sales.read:organization
- report.finance.read:organization
```

---

## N. User Management Permission

```txt
user.read
user.create
user.update
user.delete
user.restore
user.activate
user.deactivate
user.suspend
user.invite
user.assign_role
user.assign_team
user.reset_password
```

Contoh:

```txt
Sales Manager boleh melihat user dalam timnya.

Admin boleh create user, assign role, dan assign team.

Super Admin boleh manage semua user.
```

---

## O. Team Permission

```txt
team.read
team.create
team.update
team.delete
team.assign_member
team.remove_member
team.assign_manager
```

Contoh:

```txt
Manager boleh melihat anggota timnya.

Admin boleh membuat team dan memindahkan user antar team.
```

---

## P. Role & Permission Permission

Ini permission paling sensitif.

```txt
role.read
role.create
role.update
role.delete
role.assign_permission

permission.read
permission.manage
permission.matrix.update
```

Saran penting:

```txt
Jangan semua admin boleh mengubah permission.

Permission management sebaiknya hanya untuk:
- Super Admin
- Owner
- System Administrator
```

---

## Q. System Setting Permission

```txt
setting.read
setting.update
setting.security.update
setting.crm.update
setting.notification.update
setting.integration.update
```

---

## R. Integration Permission

```txt
integration.read
integration.create
integration.update
integration.delete
integration.connect
integration.disconnect
integration.sync
integration.view_secret
integration.update_secret
```

Contoh integrasi CRM:

```txt
WhatsApp
Email SMTP
Google Calendar
Discord Lead Notification
Payment Gateway
Webhook
OpenAI Agent
```

Permission sensitif:

```txt
integration.view_secret
integration.update_secret
```

Karena bisa menyangkut API key, token, webhook secret, dan credential.

---

# 7. Permission Matrix

Permission Matrix adalah halaman untuk mengatur role dan permission secara visual.

Contoh matrix:

| Module  |       Permission | Sales | Manager |      Finance |        Admin |
| ------- | ---------------: | ----: | ------: | -----------: | -----------: |
| Lead    |             read |   own |    team |         none | organization |
| Lead    |           create |   own |    team |         none | organization |
| Lead    |           delete |  none |    none |         none | organization |
| Deal    |             read |   own |    team | organization | organization |
| Deal    | approve_discount |  none |    team |         none | organization |
| Invoice |             read |   own |    team | organization | organization |
| Invoice |        mark_paid |  none |    none | organization | organization |
| User    |           create |  none |    none |         none | organization |
| Role    |           manage |  none |    none |         none | organization |

Permission matrix harus bisa:

```txt
Melihat daftar permission per role
Mengubah scope permission
Mengaktifkan atau menonaktifkan permission
Filter berdasarkan module
Search permission
Bulk enable permission
Bulk disable permission
Clone permission dari role lain
Reset ke default role template
```

---

# 8. Role Template CRM

Role template adalah preset permission untuk memudahkan setup awal.

## Super Admin

```txt
Akses penuh semua fitur dan semua data.
```

Permission:

```txt
*:*:all
```

Atau secara konsep:

```txt
all permissions with all scope
```

---

## CRM Admin

```txt
Mengelola konfigurasi CRM, user, team, pipeline, dan data organisasi.
```

Permission:

```txt
lead.manage:organization
customer.manage:organization
contact.manage:organization
company.manage:organization
deal.manage:organization
pipeline.manage:organization
activity.manage:organization
task.manage:organization
user.manage:organization
team.manage:organization
report.read:organization
setting.crm.update:organization
```

---

## Sales Manager

```txt
Mengelola lead, deal, dan aktivitas tim sales.
```

Permission:

```txt
lead.read:team
lead.create:team
lead.update:team
lead.assign:team
lead.convert:team

customer.read:team
customer.update:team

deal.read:team
deal.create:team
deal.update:team
deal.move_stage:team
deal.close_won:team
deal.close_lost:team
deal.approve_discount:team

activity.read:team
activity.create:team
activity.assign:team

task.read:team
task.create:team
task.assign:team

report.sales.read:team
report.team_performance.read:team
```

---

## Sales Executive

```txt
Mengelola lead, customer, deal, activity, dan task miliknya sendiri.
```

Permission:

```txt
lead.read:own
lead.create:own
lead.update:own
lead.convert:own

customer.read:own
customer.update:own

contact.read:own
contact.create:own
contact.update:own

company.read:own
company.create:own
company.update:own

deal.read:own
deal.create:own
deal.update:own
deal.move_stage:own
deal.close_won:own
deal.close_lost:own
deal.view_value:own

activity.read:own
activity.create:own
activity.update:own
activity.complete:own

task.read:own
task.create:own
task.update:own
task.complete:own

note.read:own
note.create:own
```

---

## Marketing

```txt
Mengelola campaign lead dan import lead.
```

Permission:

```txt
lead.read:organization
lead.create:organization
lead.import:organization
lead.update:own
lead.export:organization

campaign.read:organization
campaign.create:organization
campaign.update:organization

report.marketing.read:organization
```

---

## Customer Support

```txt
Melihat dan memperbarui data customer untuk kebutuhan support.
```

Permission:

```txt
customer.read:organization
customer.update:organization

contact.read:organization
contact.update:organization

activity.read:organization
activity.create:organization

task.read:own
task.create:own
task.update:own

note.read:organization
note.create:organization
```

Biasanya tidak diberi akses:

```txt
deal.view_margin
deal.update_value
invoice.mark_paid
role.manage
permission.manage
```

---

## Finance

```txt
Mengelola quotation, invoice, payment, dan laporan keuangan.
```

Permission:

```txt
customer.read:organization
company.read:organization
deal.read:organization
deal.view_value:organization

quotation.read:organization
quotation.approve:organization
quotation.export_pdf:organization
quotation.view_price:organization

invoice.read:organization
invoice.create:organization
invoice.update:organization
invoice.send:organization
invoice.mark_paid:organization
invoice.cancel:organization
invoice.export_pdf:organization
invoice.view_amount:organization

payment.read:organization
payment.create:organization
payment.update:organization

report.finance.read:organization
```

---

# 9. Direct Permission Override

Selain role, kadang user butuh permission khusus.

Contoh:

```txt
User Rina role-nya Sales Executive.
Tapi sementara dia ditugaskan membantu Manager.
Maka dia diberi direct permission:
deal.read:team
lead.read:team
```

Direct permission bisa berupa:

```txt
allow
deny
```

Contoh:

```txt
allow: report.sales.read:team
deny: deal.view_margin
```

Saran desain:

```txt
Default akses dari role.
Direct allow bisa menambah akses.
Direct deny harus mengalahkan role permission.
```

Prioritas evaluasi:

```txt
1. Super Admin bypass
2. Direct deny
3. Direct allow
4. Role permission
5. Default deny
```

---

# 10. Permission Group

Permission perlu dikelompokkan agar UI mudah digunakan.

Contoh group:

```txt
Sales Management
- lead.*
- deal.*
- pipeline.*

Customer Management
- customer.*
- contact.*
- company.*

Activity Management
- activity.*
- task.*
- note.*

Finance
- quotation.*
- invoice.*
- payment.*

Reporting
- report.*

Administration
- user.*
- team.*
- role.*
- permission.*
- setting.*

Integration
- integration.*
- webhook.*
```

---

# 11. Database Design

## permissions

```sql
CREATE TABLE permissions (
    id UUID PRIMARY KEY,
    module VARCHAR(100) NOT NULL,
    action VARCHAR(100) NOT NULL,
    slug VARCHAR(150) NOT NULL UNIQUE,
    name VARCHAR(150) NOT NULL,
    description TEXT,
    group_name VARCHAR(100),
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

Contoh data:

```txt
module: lead
action: read
slug: lead.read
name: Read Lead
group_name: Sales Management
```

---

## roles

```sql
CREATE TABLE roles (
    id UUID PRIMARY KEY,
    name VARCHAR(150) NOT NULL,
    slug VARCHAR(150) NOT NULL UNIQUE,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

---

## role_permissions

```sql
CREATE TABLE role_permissions (
    id UUID PRIMARY KEY,
    role_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    scope VARCHAR(50) NOT NULL DEFAULT 'none',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    UNIQUE(role_id, permission_id)
);
```

Scope:

```txt
none
own
team
branch
department
organization
all
```

---

## user_permissions

```sql
CREATE TABLE user_permissions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    effect VARCHAR(20) NOT NULL,
    scope VARCHAR(50) NOT NULL DEFAULT 'none',
    reason TEXT,
    expires_at TIMESTAMP NULL,
    created_by UUID NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    UNIQUE(user_id, permission_id, effect)
);
```

Effect:

```txt
allow
deny
```

---

## user_roles

```sql
CREATE TABLE user_roles (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    role_id UUID NOT NULL,
    organization_id UUID NULL,
    team_id UUID NULL,
    assigned_by UUID NULL,
    assigned_at TIMESTAMP NOT NULL,

    UNIQUE(user_id, role_id, organization_id, team_id)
);
```

---

# 12. API Permission Management

## Permission API

```txt
GET    /admin/permissions
GET    /admin/permissions/grouped
GET    /admin/permissions/:id
POST   /admin/permissions
PATCH  /admin/permissions/:id
DELETE /admin/permissions/:id
```

Catatan:

```txt
Untuk permission system bawaan, delete sebaiknya tidak diizinkan.
Gunakan is_system = true.
```

---

## Role Permission API

```txt
GET    /admin/roles/:roleId/permissions
PUT    /admin/roles/:roleId/permissions
PATCH  /admin/roles/:roleId/permissions/:permissionId
DELETE /admin/roles/:roleId/permissions/:permissionId
```

Contoh request update role permission:

```json
{
  "permissions": [
    {
      "permission": "lead.read",
      "scope": "team"
    },
    {
      "permission": "lead.create",
      "scope": "team"
    },
    {
      "permission": "lead.delete",
      "scope": "none"
    }
  ]
}
```

---

## User Permission Override API

```txt
GET    /admin/users/:userId/permissions
GET    /admin/users/:userId/effective-permissions
POST   /admin/users/:userId/permissions/allow
POST   /admin/users/:userId/permissions/deny
DELETE /admin/users/:userId/permissions/:permissionId
```

Contoh allow:

```json
{
  "permission": "deal.read",
  "scope": "team",
  "reason": "Temporary access for sales backup",
  "expires_at": "2026-07-01T00:00:00Z"
}
```

Contoh deny:

```json
{
  "permission": "deal.view_margin",
  "scope": "organization",
  "reason": "Margin is restricted for this user"
}
```

---

## Permission Matrix API

```txt
GET /admin/permission-matrix
PUT /admin/permission-matrix
GET /admin/permission-matrix/roles/:roleId
POST /admin/permission-matrix/clone
POST /admin/permission-matrix/reset-role-template
```

Contoh clone permission:

```json
{
  "source_role_id": "sales-manager-role-id",
  "target_role_id": "regional-manager-role-id"
}
```

---

# 13. Backend Service

Service yang disarankan:

```txt
PermissionService
RoleService
RolePermissionService
UserPermissionService
EffectivePermissionService
PermissionMatrixService
AccessControlService
PermissionSeederService
AuditLogService
```

---

# 14. Permission Check Backend

Contoh konsep pengecekan permission:

```txt
can(user, "lead.read", lead)
```

Flow:

```txt
1. Ambil effective permission user.
2. Cek apakah ada direct deny.
3. Cek apakah ada direct allow.
4. Cek role permission.
5. Cek scope.
6. Cek ownership data.
7. Allow atau deny request.
```

Contoh logic scope:

```txt
own:
data.owner_id == user.id

team:
data.owner_id berada dalam team user

organization:
data.organization_id == user.active_organization_id

all:
boleh lintas organisasi
```

---

# 15. Frontend Pages

Halaman Permission Management:

```txt
/admin/permissions
/admin/permissions/create
/admin/permissions/:id/edit

/admin/roles
/admin/roles/create
/admin/roles/:id/edit
/admin/roles/:id/permissions

/admin/permission-matrix

/admin/users/:id/roles
/admin/users/:id/permissions
/admin/users/:id/effective-permissions
```

---

# 16. UI Permission Matrix

Kolom:

```txt
Module
Permission
Description
Sales Executive
Sales Manager
Marketing
Support
Finance
CRM Admin
Super Admin
```

Isi cell bisa berupa dropdown:

```txt
No Access
Own
Team
Branch
Department
Organization
All
```

Contoh:

| Module  | Permission  | Sales     | Manager   | Finance      | Admin        |
| ------- | ----------- | --------- | --------- | ------------ | ------------ |
| Lead    | Read        | Own       | Team      | No Access    | Organization |
| Lead    | Delete      | No Access | No Access | No Access    | Organization |
| Deal    | View Value  | Own       | Team      | Organization | Organization |
| Deal    | View Margin | No Access | No Access | Organization | Organization |
| Invoice | Mark Paid   | No Access | No Access | Organization | Organization |

---

# 17. Audit Log Permission

Setiap perubahan permission wajib masuk audit log.

Event yang perlu dicatat:

```txt
permission.created
permission.updated
permission.deleted

role.created
role.updated
role.deleted

role_permission.assigned
role_permission.updated
role_permission.removed

user_permission.allow_added
user_permission.deny_added
user_permission.removed

permission_matrix.updated
role_permission.cloned
role_template.reset
```

Data audit:

```txt
actor_user_id
target_user_id
target_role_id
permission_id
old_value
new_value
reason
ip_address
user_agent
created_at
```

---

# 18. Security Rule Penting

Aturan wajib:

```txt
Default deny semua akses.
Permission sensitif hanya untuk role tertentu.
Direct deny harus mengalahkan allow.
Super Admin tidak boleh mudah dihapus.
Role system tidak boleh dihapus.
Permission system tidak boleh dihapus.
Perubahan permission wajib masuk audit log.
Role dan permission tidak boleh bisa diedit oleh user tanpa permission.manage.
User tidak boleh menaikkan permission dirinya sendiri.
User tidak boleh menghapus role terakhir miliknya sendiri jika itu membuat sistem tanpa admin.
```

---

# 19. Permission Sensitif

Permission ini harus sangat dibatasi:

```txt
permission.manage
role.assign_permission
role.delete
user.assign_role
user.delete
user.suspend
setting.security.update
integration.view_secret
integration.update_secret
deal.view_margin
deal.approve_discount
invoice.mark_paid
payment.update
report.finance.read
audit_log.read
```

---

# 20. MVP Permission Management

Untuk tahap awal, jangan buat terlalu kompleks.

## Phase 1 — RBAC Basic

```txt
permissions
roles
role_permissions
user_roles
check permission by slug
middleware permission
seed default permissions
seed default roles
```

## Phase 2 — Scope Access

```txt
own
team
organization
all
```

## Phase 3 — Permission Matrix

```txt
UI matrix role vs permission
update scope permission
clone role permission
reset role template
```

## Phase 4 — User Override

```txt
direct allow
direct deny
expires_at
effective permission viewer
```

## Phase 5 — Advanced Security

```txt
audit log detail
permission change approval
sensitive permission protection
temporary permission
```

---

# 21. Minimum Permission untuk CRM MVP

Untuk MVP CRM, mulai dari permission ini dulu:

```txt
lead.read
lead.create
lead.update
lead.delete
lead.assign
lead.convert

customer.read
customer.create
customer.update
customer.delete

contact.read
contact.create
contact.update
contact.delete

company.read
company.create
company.update
company.delete

deal.read
deal.create
deal.update
deal.delete
deal.move_stage
deal.close_won
deal.close_lost
deal.view_value

activity.read
activity.create
activity.update
activity.delete
activity.complete

task.read
task.create
task.update
task.delete
task.assign
task.complete

note.read
note.create
note.update
note.delete

quotation.read
quotation.create
quotation.update
quotation.send
quotation.approve

invoice.read
invoice.create
invoice.update
invoice.send
invoice.mark_paid

report.sales.read
report.finance.read
report.export

user.read
user.create
user.update
user.delete
user.assign_role

team.read
team.create
team.update
team.assign_member

role.read
role.create
role.update
role.delete
role.assign_permission

permission.read
permission.manage

setting.read
setting.update
```

---

# 22. Kesimpulan Desain

Desain Permission Management CRM yang kuat sebaiknya memakai:

```txt
RBAC + Scope + Direct Override + Audit Log
```

Struktur aksesnya:

```txt
User
  -> User Roles
    -> Role
      -> Role Permissions
        -> Permission + Scope

User
  -> Direct Permission Override
    -> Allow / Deny + Scope
```

Untuk CRM, bagian terpenting bukan hanya “boleh akses fitur atau tidak”, tapi juga:

```txt
boleh akses data siapa?
data sendiri?
data tim?
data seluruh organisasi?
atau semua tenant?
```

Jadi pondasi terbaiknya:

```txt
permission slug + scope + ownership checking
```

Contoh final:

```txt
lead.read:own
deal.read:team
customer.read:organization
invoice.mark_paid:organization
permission.manage:organization
```

Dengan desain ini, CRM kamu bisa berkembang dari MVP sederhana sampai enterprise CRM yang punya multi-team, multi-branch, multi-organization, dan kontrol akses yang rapi.
