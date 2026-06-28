# Google Account Auth Traceability Index

Dokumen ini menghubungkan kebutuhan register dan login memakai account Google ke task development, API contract, config, schema, test, dan status implementasi.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `blocked`: menunggu dependency atau keputusan.
- `deferred`: sengaja ditunda.

## Source Documents

| Source | Purpose |
| --- | --- |
| `README.md` | Visi social login OAuth2 pada platform |
| `docs/reference-auth-user.md` | Feature reference Auth dan User |
| `docs/auth-user-development-tasks.md` | Breakdown Auth/User utama |
| `docs/google-auth-development-tasks.md` | Breakdown detail Google account auth |
| `docs/auth-user-public-api-contract.md` | Kontrak API publik Auth/User |
| `docs/auth-user-traceability-index.md` | Traceability Auth/User utama |
| `docs/auth-user-migration-seed-plan.md` | Schema auth/user dan `auth_identities` |

## Module Mapping

| Module | Path | Responsibility |
| --- | --- | --- |
| Config | `internal/config` | Google auth config, client ID allowlist, policy validation |
| Auth core | `internal/core/auth` | Token/session helper existing; optional Google verifier abstraction jika dipilih |
| User handler | `internal/modules/user/handler` | HTTP binding `POST /auth/google` |
| User service | `internal/modules/user/service` | Google login/register business rule, transaction boundary, session issuance |
| User repository | `internal/modules/user/repository` | Query user, identity, role, session, audit |
| User DTO | `internal/modules/user/dto` | Request/response contract |
| User model | `internal/modules/user/model` | User status and auth domain enums |
| Database | `migrations` | Existing `auth_identities`; optional future columns if needed |
| Notification | `internal/core/notification` | Optional welcome/verification notification |

## Requirement Traceability

| Requirement | Task ID | API Contract | Config/Schema | Status |
| --- | --- | --- | --- | --- |
| Enable/disable Google auth | GAUTH-0001 | Error `AUTH_GOOGLE_DISABLED` | `AUTH_GOOGLE_ENABLED` | done |
| Configure allowed Google client IDs | GAUTH-0001 | Token audience validation | `AUTH_GOOGLE_CLIENT_IDS` | done |
| Verify Google ID token backend-side | GAUTH-0002 | `POST /auth/google` | `google.golang.org/api/idtoken` | done |
| Reject invalid audience/issuer/expired token | GAUTH-0002 | Google auth errors | Client ID allowlist | done |
| Use Google `sub` as provider user ID | GAUTH-0003 | Not public | `auth_identities.provider_user_id` | done |
| Login existing Google identity | GAUTH-0004 | `POST /auth/google` | `auth_identities`, `sessions`, `refresh_tokens` | done |
| Enforce user status during Google login | GAUTH-0004 | Existing account errors | `users.status`, `users.deleted_at` | done |
| Auto-link existing verified email | GAUTH-0005 | Conflict/error contract | `AUTH_GOOGLE_AUTO_LINK_VERIFIED_EMAIL` | done |
| Reject unsafe email conflict | GAUTH-0005 | `AUTH_GOOGLE_EMAIL_CONFLICT` | `users.email`, `auth_identities` | done |
| Auto-register new Google user | GAUTH-0006 | `is_new_user=true` | `AUTH_GOOGLE_AUTO_REGISTER`, `users`, `user_profiles` | done |
| Assign default Google role | GAUTH-0006 | User response roles | `AUTH_GOOGLE_DEFAULT_ROLE`, `user_roles` | done |
| Apply default Google status | GAUTH-0006 | User response status | `AUTH_GOOGLE_DEFAULT_STATUS=active` | done |
| Invitation-aware Google register | GAUTH-0007 | `invite_token` request field | `user_invitations` future table | blocked |
| HTTP endpoint and DTO | GAUTH-0008 | `POST /auth/google` | Route registration | done |
| Rate limit Google auth | GAUTH-0009 | `RATE_LIMITED` | Redis/rate limit store | deferred |
| Login history for Google login | GAUTH-0009 | Admin login histories | `login_histories` | done |
| Audit Google link/register | GAUTH-0009 | Admin audit logs | `audit_logs` | done |
| Public API and OpenAPI docs | GAUTH-0010 | API contract and OpenAPI | None | done |
| Unit, handler, repository tests | GAUTH-0011 | Not public | Test fixtures/mocks | partial |

## API to Task Index

| Endpoint | Task ID | Status | Notes |
| --- | --- | --- | --- |
| `POST /auth/google` | GAUTH-0004, GAUTH-0005, GAUTH-0006, GAUTH-0008 | done | Login, auto-link, or auto-register with Google ID token |

## Config Traceability

| Env | Task ID | Required In Production | Status |
| --- | --- | ---: | --- |
| `AUTH_GOOGLE_ENABLED` | GAUTH-0001 | No | done |
| `AUTH_GOOGLE_CLIENT_IDS` | GAUTH-0001 | Yes, when enabled | done |
| `AUTH_GOOGLE_AUTO_REGISTER` | GAUTH-0001, GAUTH-0006 | No | done |
| `AUTH_GOOGLE_AUTO_LINK_VERIFIED_EMAIL` | GAUTH-0001, GAUTH-0005 | No | done |
| `AUTH_GOOGLE_DEFAULT_ROLE` | GAUTH-0001, GAUTH-0006 | Yes, when auto-register enabled | done |
| `AUTH_GOOGLE_DEFAULT_STATUS` | GAUTH-0001, GAUTH-0006 | Yes, when auto-register enabled | done |

## Schema Traceability

| Schema Object | Related Task | Status | Notes |
| --- | --- | --- | --- |
| `auth_identities.provider='google'` | GAUTH-0003 | available | Existing migration allows Google provider |
| `auth_identities.provider_user_id` | GAUTH-0003 | available | Store Google `sub` |
| `auth_identities.provider_email` | GAUTH-0003 | available | Store current verified Google email |
| `roles.slug='member'` | GAUTH-0006 | done | Seeded by `000045_seed_member_role` for Google auto-register default role |
| `users.email` | GAUTH-0005, GAUTH-0006 | available | Used for safe linking and new user registration |
| `users.email_verified_at` | GAUTH-0005, GAUTH-0006 | available | Can be set from Google verified email based on policy |
| `user_profiles.avatar_url` | GAUTH-0006 | available | Can store Google picture URL if accepted |
| `sessions` | GAUTH-0004, GAUTH-0006 | available | Existing session issuance |
| `refresh_tokens` | GAUTH-0004, GAUTH-0006 | available | Existing refresh rotation |
| `login_histories` | GAUTH-0009 | available | Record Google login success/failure |
| `audit_logs` | GAUTH-0009 | available | Record identity link and register |
| `user_invitations` | GAUTH-0007 | planned | Dependency from `AUTH-0402` |

No new migration is required for MVP unless implementation needs extra metadata beyond current `auth_identities`.

## Test Traceability

| Test Area | Task ID | Minimum Coverage | Status |
| --- | --- | --- | --- |
| Config validation | GAUTH-0001 | Disabled, enabled without client ID, invalid default status | done |
| Token verifier | GAUTH-0002 | Invalid token, invalid audience, email unverified, success mapping | partial |
| Existing identity login | GAUTH-0004 | Active success, inactive/suspended/banned/deleted rejected | done |
| Auto-link | GAUTH-0005 | Verified email link success, unverified/conflict rejected | partial |
| Auto-register | GAUTH-0006 | New user, username collision, default role/status | done |
| Handler | GAUTH-0008 | Validation error, success response envelope | partial |
| Rate limit/history/audit | GAUTH-0009 | Failed auth history, no token in metadata | partial |
| Repository integration | GAUTH-0003 | Identity lookup, create identity, unique conflict | deferred |

## Dependency Traceability

| Dependency | Related Task | Status | Notes |
| --- | --- | --- | --- |
| Google token verification implementation | GAUTH-0002 | done | Uses `google.golang.org/api/idtoken` |
| Rate limit infrastructure | GAUTH-0009 | deferred | Existing `AUTH-0307` is still planned |
| Invitation flow | GAUTH-0007 | blocked | Depends on `AUTH-0402` |
| Notification welcome/verification | GAUTH-0006 | optional | Existing notification outbox can be reused |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-28 | Google auth MVP uses frontend-provided Google ID token verified by backend | Keeps backend as session authority and avoids storing Google access tokens |
| 2026-06-28 | Google `sub` is the canonical provider user ID | Email can change; Google subject is stable per client/project |
| 2026-06-28 | Auto-link requires Google verified email and explicit config policy | Prevents unsafe account takeover via email conflict |
| 2026-06-28 | No new migration required for MVP | Existing `auth_identities` already supports provider `google` |
| 2026-06-28 | Google verifier uses `google.golang.org/api/idtoken` package | User approved Google verifier package and official verifier avoids custom JWK validation |
| 2026-06-28 | New Google users default to `active` | User locked business policy that new Google accounts are immediately active |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-28 | Added Google auth planning documentation | `docs/google-auth-development-tasks.md`, `docs/google-auth-traceability-index.md` | Detailed task and traceability for Google account register/login |
| 2026-06-28 | Implemented Google account auth MVP | `internal/config`, `internal/core/auth`, `internal/modules/user`, `.env.example`, `api/openapi.yaml`, `docs/*google-auth*`, `docs/auth-user-*` | Added config, Google ID token verifier, Google login/register/link service, repository transaction, route `POST /api/v1/auth/google`, tests, and OpenAPI contract |
| 2026-06-28 | Seeded default Google member role | `migrations/000045_seed_member_role.*.sql` | Added default `member` role required by `AUTH_GOOGLE_DEFAULT_ROLE=member` and applied migration |

## Update Rules

- Saat task mulai dikerjakan, ubah status menjadi `in_progress`.
- Saat endpoint selesai, update `docs/auth-user-public-api-contract.md` dan `api/openapi.yaml`.
- Saat config ditambahkan, update `.env.example` dan config traceability.
- Saat migration baru dibuat, update schema traceability.
- Saat ada keputusan teknis baru, tambahkan entry ke `Decision Log`.
- Saat task selesai, ubah status menjadi `done` dan catat command/test pada development history.
