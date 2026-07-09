# Permission Context — Auth, RBAC, Multi-Tenant Resolution

> Sumber: `internal/core/auth/`, `internal/core/permission/`, `internal/core/middleware/`,
> `internal/core/tenant/`, `internal/modules/user/`, `internal/modules/organization/`, dan migration
> `000001`–`000004`, `000019`, `000045`–`000048`. Untuk requirement bisnis auth/user secara lengkap, rujuk
> `docs/reference-auth-user.md` dan `docs/auth-user-*.md` — dokumen ini fokus pada **mekanisme teknis**.

## 1. Model Auth

### Login
`POST /api/v1/auth/login` → `AuthService.Login()`:
1. Cari user by `identifier` (email/username).
2. Verifikasi password via bcrypt.
3. Cek status user (`active`, `pending`, atau `invited` boleh login).
4. Buat record di tabel `sessions` (device_name, user_agent, ip_address, expires_at).
5. Generate **access token** (JWT HS256, TTL dari `JWT_EXPIRES_IN`) dan **refresh token** (JWT HS256, TTL
   dari `JWT_REFRESH_EXPIRES_IN` atau `JWT_REMEMBER_ME_REFRESH_EXPIRES_IN` jika `rememberMe=true`).
6. Refresh token disimpan **dalam bentuk hash** (SHA-256) di tabel `sessions`/`refresh_tokens`, bukan
   plaintext.
7. Catat `login_histories` (event=login, success=true/false).

Metode login lain: `POST /auth/google` (Google OAuth, opsional — `AUTH_GOOGLE_ENABLED`).

### Struktur Token Claims
```go
type Claims struct {
    UserID, SessionID, Email, Username string
    Roles, Permissions                 []string
    TokenType                          TokenType // "access" atau "refresh"
    Issuer, Subject                    string
    IssuedAt, ExpiresAt                int64
}
```
Sumber: `internal/core/auth` (token manager terpisah untuk access dan refresh, masing-masing pakai secret
berbeda: `cfg.Auth.Secret` dan `cfg.Auth.RefreshSecret`).

### Validasi Token (Middleware)
`middleware.Authenticate(authenticator)` (`internal/core/middleware/auth.go`):
1. Ekstrak Bearer token dari header `Authorization`.
2. Parse & validasi signature HMAC-SHA256, cek expiry, cek issuer.
3. Simpan `AuthenticatedUser{ID, SessionID, Status, Roles, Permissions}` ke Gin context
   (key: `authenticated_user`).

### Refresh & Revoke
- `POST /auth/refresh-token` — rotasi token: refresh token lama ditandai `replaced_by_token_id`, refresh
  token baru dibuat.
- `POST /auth/logout` — set `sessions.revoked_at = now()`, otomatis invalidasi refresh token terkait.
- `POST /auth/logout-all` — revoke semua session user (opsional exclude session saat ini).

### Password Reset & Email Verification
- `POST /auth/forgot-password` → `password_reset_tokens` (TTL `AUTH_RESET_TOKEN_EXPIRES_IN`).
- `POST /auth/reset-password/validate`, `POST /auth/reset-password`.
- `POST /auth/verify-email`, `POST /auth/resend-verification-email` → `email_verification_tokens` (TTL
  `AUTH_VERIFICATION_TOKEN_EXPIRES_IN`).
- OTP (`otp_codes`) untuk keperluan lain (login/verify_phone/reset_password/change_email), TTL
  `AUTH_OTP_EXPIRES_IN`, max attempt `AUTH_OTP_MAX_ATTEMPTS`.

## 2. Model Role & Permission (RBAC 3 Layer)

**Bukan Casbin** (berbeda dari klaim README.md versi lama) — implementasi RBAC custom di
`internal/core/permission/`.

### Layer 1: Role ↔ Permission (`role_permissions`)
- PK komposit `(role_id, permission_id)`.
- Kolom `scope`: `'none' | 'own' | 'team' | 'branch' | 'department' | 'organization' | 'all'`.
- **Status (dikonfirmasi manual 2026-07-01): "RBAC scope exists in database schema, including organization
  and all, but runtime enforcement needs code verification."** Perlu dibedakan tegas:
  - **Confirmed** — scope `'organization'` dan `'all'` **ada di schema database** (kolom `scope` di
    `role_permissions`) dan dipakai sebagai bagian dari desain layer permission (lihat penamaan middleware
    `RequireOrganization`/`Require` yang selaras dengan kedua scope ini).
  - **Needs code verification** — apakah nilai scope granular lain (`'own'`, `'team'`, `'branch'`,
    `'department'`) benar-benar **dicek di runtime** oleh permission checker
    (`internal/core/permission/policy/checker.go`, fungsi `HasAll`) masih harus dibuktikan dari kode. Jangan
    klaim scope-scope ini "fully enforced" sampai ada bukti implementasi eksplisit di checker/middleware/service.

### Layer 2: User ↔ Role (`user_roles`)
- Unique **global**: `(user_id, role_id)` where `organization_id IS NULL`.
- Unique **per-organization**: `(user_id, role_id, organization_id)` where `organization_id IS NOT NULL`.
- Artinya user bisa punya role global (mis. `super_admin`) DAN role berbeda di tiap organisasi yang
  diikutinya (mis. `organization_owner` di organisasi A, `member` di organisasi B).

### Layer 3: Direct Override (`user_permissions`)
- Sama pola unique (global vs per-organization) dengan tambahan `effect`: `'allow' | 'deny'`.
- Dipakai untuk override langsung di luar role — mis. mencabut 1 permission spesifik dari user tertentu
  tanpa mengubah role-nya.

### Naming Convention Permission
Format: `module.resource.action` (dot-separated), contoh nyata dari kode:
```
user.read, user.create, user.update, user.delete, user.restore, user.update_status
user.session.read, user.session.revoke
role.read, role.create, role.update, role.delete, role.assign
permission.read, permission.manage
audit.read
organization.user.read, organization.user.manage
organization.billing.read, organization.billing.manage
organization.domain.manage
platform.product.plan.read, platform.product.plan.manage
platform.product.plan_price.read, platform.product.plan_price.manage
platform.product.feature.read, platform.product.feature.manage
platform.product.entitlement.read, platform.product.entitlement.manage
platform.subscription.read, platform.subscription.manage
landing.page.read, landing.page.create, landing.page.update, landing.page.delete, landing.page.publish
landing.submission.read, landing.submission.export
landing.form.manage, landing.section.manage, landing.media.manage
```
Kolom `module`/`action` di tabel `permissions` diturunkan dari `permission_name` (bagian pertama = module).

**Catatan riwayat (refactor domain-split 2026-07-01)**: permission `platform.billing.plan.*`,
`platform.billing.plan_price.*`, `platform.billing.feature.*`, `platform.billing.entitlement.*`, dan
`platform.billing.subscription.*` sudah di-rename (migration 000062, via `UPDATE` bukan delete+insert
sehingga `role_permissions` tidak putus) menjadi `platform.product.*` dan `platform.subscription.*` sesuai
domain baru. `platform.billing.invoice.*`, `platform.billing.payment.manage`, dan
`organization.billing.read/manage` **tidak berubah**. Lihat
[product-subscription-billing-concept.md](product-subscription-billing-concept.md).

### Role Sistem yang Ter-seed
| Role | Sumber | Scope |
|---|---|---|
| `super_admin` | `internal/modules/user/seeder/super_admin.go` | Semua permission, scope `all`, assignment global |
| `member` | `migrations/000045_seed_member_role.up.sql` | Permission terbatas untuk anggota organisasi biasa |
| `organization_owner` | `migrations/000046_seed_organization_owner_role.up.sql` | Permission penuh untuk manajemen organisasi sendiri |

## 3. Middleware Permission Enforcement

Lokasi: `internal/core/permission/middleware/permission.go`.

```go
func Require(checker PermissionChecker, requiredPermissions ...string) gin.HandlerFunc
func RequireOrganization(checker OrganizationPermissionChecker, requiredPermissions ...string) gin.HandlerFunc
func RequireOrganizationOrGlobal(checker CombinedPermissionChecker, requiredPermissions ...string) gin.HandlerFunc
```

- `Require` — cek permission global user (dipakai untuk endpoint platform-level, mis. `platform.product.*`,
  `platform.subscription.*`, `platform.billing.invoice.*`).
- `RequireOrganization` — cek permission user **dalam konteks organisasi aktif** (butuh tenant context sudah
  ter-resolve).
- `RequireOrganizationOrGlobal` — cek org-scoped dulu, fallback ke global (dipakai untuk endpoint yang bisa
  diakses baik operator platform maupun owner organisasi, mis. domain management).
- Response saat gagal: HTTP 403 lewat `response.Error()`.

Interface:
```go
type PermissionChecker interface {
    Can(ctx context.Context, userID string, requiredPermissions []string) error
}
type OrganizationPermissionChecker interface {
    CanOrganization(ctx context.Context, userID, organizationID string, requiredPermissions []string) error
}
```

## 4. Multi-Tenant / Organization Context

### Resolusi Tenant — 3 Jalur

1. **Session-based (authenticated request)** — `AuthenticatedResolver.ResolveAuthenticatedOrganization()`
   (`internal/modules/organization/service/authenticated_resolver.go`): dipanggil setelah `Authenticate()`,
   mengambil `organizationIDSelector` (dari header/param, opsional), validasi membership user, fallback ke
   membership utama (primary) jika selector kosong. `ResolutionSource = "session"`.
2. **Public host-based** — `PublicHostResolver.ResolvePublicHost(Detail)()`
   (`internal/modules/organization/service/public_host_resolver.go`): resolve dari `Host` header —
   normalisasi host, cek apakah platform primary domain, custom domain, atau subdomain.
   `ResolutionSource ∈ {"platform_host", "subdomain", "custom_domain"}`.
3. **Worker/internal** — untuk job background/service-to-service, `ResolutionSource ∈ {"worker", "internal"}`.

### Middleware Chain (`internal/app/router.go`)
```go
protected := api.Group("")
protected.Use(
    middleware.Authenticate(deps.Authenticator),
    middleware.ResolveAuthenticatedOrganization(deps.OrganizationResolver),
)

publicModules := api.Group("")
publicModules.Use(
    middleware.ResolvePublicOrganization(deps.PublicHostResolver, opts),
    middleware.RequireActiveTenant(),
)
```

### Context yang Tersimpan (`internal/core/tenant`)
`coretenant.Context` berisi: `OrganizationID`, `OrganizationSlug`, `OrganizationType` (platform/customer),
`OrganizationStatus`, `MembershipID`, `MembershipStatus`, `MembershipVersion`, `ResolutionSource`,
`DataPlacement`, `IsPlatformOperator`, `ImpersonationSessionID` (jika sedang impersonasi).

Akses di handler: `middleware.TenantContext(c)` atau `middleware.RequireTenantContext(c)`.

### Apakah Platform Admin Bisa Lihat Semua Tenant?
Ya — lewat flag `IsPlatformOperator` di context dan endpoint `/api/v1/platform/organizations*` yang
memakai `Require`/`RequireOrganizationOrGlobal` dengan permission global (bukan org-scoped). Detail endpoint
platform-level ada di `internal/modules/organization/handler/platform_handler.go`.

### Risiko Data Leakage Antar Tenant
- Tabel `landing_*` **terlindungi RLS** — bahkan jika query lupa filter `organization_id`, database akan
  menolak baris dari organisasi lain (asalkan session variable `app.organization_id` di-set benar).
- Tabel `organization_*` dan `billing_*` **tidak terlindungi RLS** — sepenuhnya bergantung pada disiplin
  filter manual di repository. Ini risiko nyata: bug di satu query repository (lupa `WHERE organization_id
  = $1`) bisa membocorkan data lintas tenant tanpa ada lapisan pertahanan kedua dari database. Lihat
  [database-context.md](database-context.md) bagian 3 dan [next-development-tasks.md](next-development-tasks.md).

## 5. Gap yang Ditemukan (Ringkasan)

| Gap | Detail | Dampak |
|---|---|---|
| README.md lama klaim Casbin ABAC | Tidak ada di kode — RBAC custom tanpa attribute-based rule (jam kerja, IP, dll) | Sudah dikoreksi di README.md sebagai bagian paket dokumentasi ini |
| Scope granular (`own`/`team`/`branch`/`department`) belum jelas dipakai | Kolom `role_permissions.scope` sudah ada di schema (Confirmed) termasuk `organization`/`all`; enforcement runtime scope granular lain belum terkonfirmasi dicek di checker | **Needs code verification** sebelum mengandalkan scope granular untuk fitur baru — lihat tabel Ringkasan Verifikasi Manual di [repository-context.md](repository-context.md) |
| RLS tidak mencakup `organization_*`/`billing_*` | Lihat bagian 4 | Risiko data leakage jika repository lupa filter manual |
| Tidak ada dokumentasi permission slug per endpoint di OpenAPI | Lihat [api-contract-review.md](api-contract-review.md) | Sulit audit kepatuhan akses dari luar kode |
| Redis untuk refresh token — klaim README lama | Refresh token disimpan **hash di PostgreSQL** (`sessions`/`refresh_tokens`), bukan Redis | Sudah dikoreksi |

## 6. Rekomendasi Permission Berikutnya

1. Verifikasi pemakaian nyata scope granular (`own`/`team`/`branch`/`department`) di
   `internal/core/permission/policy/checker.go` — status saat ini **Needs code verification** (scope
   `organization`/`all` sudah Confirmed ada di schema, tapi enforcement scope granular lain belum
   dibuktikan dari kode). Jika hasil audit menunjukkan belum dipakai, dokumentasikan sebagai
   "reserved for future use" secara eksplisit di skema/komentar migration.
2. Tambahkan anotasi permission per endpoint di OpenAPI (lihat api-contract-review.md P2).
3. Evaluasi apakah tabel `organization_*`/`billing_*` perlu RLS tambahan atau audit query manual yang
   ketat (code review checklist) sebagai mitigasi — jadikan keputusan eksplisit, bukan dibiarkan ambigu.
