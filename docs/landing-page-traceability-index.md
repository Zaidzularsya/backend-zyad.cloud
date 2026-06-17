# Landing Page Traceability Index

Dokumen ini menghubungkan requirement Landing Page ke task, API, migration, permission, event, dan status implementasi.

Status: `planned`, `in_progress`, `done`, `deferred`, atau `blocked`.

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-landing-page.md` | Requirement, boundary, dan kapasitas |
| `docs/landing-page-development-tasks.md` | Breakdown development |
| `docs/landing-page-public-api-contract.md` | Contract Vue dan public renderer |
| `docs/migration-guide.md` | Aturan migration |
| `README.md` | Konteks platform |
| `docs/reference-multi-tenant.md` | Tenant context, platform organization, dan isolation |
| `docs/multi-tenant-traceability-index.md` | Dependency implementation multi-tenant |

## Module Mapping

| Capability | Path | Responsibility |
| --- | --- | --- |
| Landing module | `internal/modules/landing` | Page, section, form, submission, branding, publish, domain, analytics |
| Storage adapter | `internal/platform/storage` | Binary persistence dan URL |
| Notification core | `internal/core/notification` | Template, outbox, delivery |
| Shared core | `internal/core` | Auth, tenant context, permission, validation |
| App | `internal/app` | Dependency dan route registration |

## Source Requirement Coverage

| Source Section | Requirement Group | Task Coverage | Status |
| --- | --- | --- | --- |
| 1 | Tujuan dan page types | LAND-BE-010 | planned |
| 2.A | Page CRUD, visibility, schedule, homepage, version | LAND-BE-010, LAND-BE-013, LAND-BE-041, LAND-BE-045 | planned |
| 2.B | Section CRUD, reorder, reusable/template | LAND-BE-011, LAND-BE-015 | planned |
| 3.A | Hero component | LAND-BE-011, LAND-SEED-002 | planned |
| 3.B | CTA type, target, tracking | LAND-BE-014, LAND-BE-060 | planned |
| 3.C | Image/video/PDF, optimization, folder, reuse | LAND-BE-023 | planned |
| 4.A | Form builder, file field, anti-spam, submission | LAND-BE-030, LAND-BE-031 | planned |
| 4.B | Notification, CRM/webhook/automation integration | LAND-BE-033 | planned |
| 5 | SEO, sitemap, preview, old slug redirect | LAND-BE-012, LAND-BE-043 | planned |
| 6 | Theme and styling | LAND-BE-020, LAND-BE-022 | planned |
| 7 | Header/footer/nested navigation | LAND-BE-016 | planned |
| 8 | Autosave, revision, compare, restore, audit | LAND-BE-044 | planned |
| 9 | View, visitor, CTA, conversion, source, device, attribution | LAND-BE-060, LAND-BE-061 | planned |
| 10 | Granular permission and role matrix | LAND-0003, LAND-SEED-001, LAND-SEED-003 | planned |
| 11 | Admin workflow end-to-end | LAND-API-001..006, LAND-TEST-003 | planned |
| 12 | Admin/public API | LAND-API-001..006 | planned |
| 13 | Target Go structure | LAND-0001, LAND-0002 | planned |
| 14 | Realistic MVP | MVP tasks in development document | planned |
| 15 | Advanced features | LAND-ADV-001..007 | deferred |
| 16 | Suggested module split | Rejected by explicit user rule; integration boundaries retained | resolved |
| 17 | Development priority | Development phases 0-12 | planned |

## Requirement Traceability

| Requirement | Task ID | API/Data | Status |
| --- | --- | --- | --- |
| Canonical module `landing` | LAND-0001 | Code path | done |
| Module skeleton | LAND-0002 | Code path | done |
| Tenant isolation | LAND-0003, MT-CORE-001..005, MT-DATA-001..004, MT-PLAT-001..003 | Context/resolvers/guards, tenant transaction, repository scope, Landing access policy, RLS foundation, and reusable isolation suite done; concrete Landing repositories pending |
| Existing schema audit | LAND-DB-001 | Legacy `landing_pages`, `brands` | done |
| Landing core tables | LAND-DB-002 | `landing_pages`, `landing_page_sections` | done |
| Page CRUD/duplicate/restore | LAND-BE-010, LAND-API-001 | `/admin/landing-pages` | planned |
| Section builder/reorder | LAND-BE-011, LAND-API-001 | `landing_page_sections` | planned |
| Page visibility/password | LAND-BE-013, LAND-API-004 | Page access endpoints | planned |
| Reusable CTA/tracking | LAND-BE-014, LAND-API-005 | CTA endpoints/events | planned |
| Reusable section template | LAND-BE-015, LAND-API-005 | Template endpoints | planned |
| Navigation/menu | LAND-BE-016, LAND-API-005 | Menu endpoints | planned |
| Form builder | LAND-BE-030, LAND-API-002 | Form/field endpoints | planned |
| Form file upload | LAND-BE-030, LAND-BE-031 | Storage reference | planned |
| Public submission | LAND-BE-031, LAND-API-004 | Public form submit | planned |
| Submission management/export | LAND-BE-032, LAND-API-002 | Submission endpoints | planned |
| Lead notification/integration | LAND-BE-033, LAND-API-006 | Integration/delivery endpoints | planned |
| Branding default/override | LAND-BE-020, LAND-API-003 | Branding endpoints | planned |
| Branding storage asset | LAND-BE-021 | Storage reference | planned |
| Theme/layout/mode | LAND-BE-022, LAND-API-003 | Branding/theme endpoints | planned |
| Landing media library | LAND-BE-023, LAND-API-005 | Media endpoints | planned |
| SEO metadata | LAND-BE-012, LAND-API-001 | SEO endpoint | planned |
| Publish validation | LAND-BE-040 | Validate publish endpoint | planned |
| Immutable publish/version | LAND-BE-041 | Version/publish endpoints | planned |
| Preview | LAND-BE-042 | Preview token endpoint | planned |
| Public resolve/redirect | LAND-BE-043, LAND-API-004 | Public resolve | planned |
| Autosave/revision compare | LAND-BE-044, LAND-API-006 | Revision endpoints | planned |
| Scheduled publish/unpublish | LAND-BE-045, LAND-API-006 | Schedule endpoints/worker | planned |
| Custom domain binding | LAND-BE-050, LAND-API-003, MT-API-003 | Organization domain and page binding | planned |
| SSL/domain status integration | LAND-BE-051, MT-CORE-003 | Organization domain capability | planned |
| Public analytics event | LAND-BE-060 | Public event endpoint | planned |
| Analytics aggregate | LAND-BE-061, LAND-API-003 | Analytics endpoints | planned |
| Visual builder | LAND-ADV-001 | Future editor | deferred |
| Template marketplace | LAND-ADV-002 | Future catalog | deferred |
| A/B testing/personalization | LAND-ADV-003 | Future experiments | deferred |
| Multi-language | LAND-ADV-004 | Future locale variants | deferred |
| AI assistance | LAND-ADV-005 | Future AI integration | deferred |
| Static/edge/heatmap/warehouse | LAND-ADV-006 | Future delivery/analytics | deferred |
| Custom CSS | LAND-ADV-007 | Future restricted editor | deferred |
| Permission seed | LAND-SEED-001 | Permission tables | planned |
| Test coverage | LAND-TEST-001..003 | Unit/integration/handler | planned |
| OpenAPI and Vue contract sync | LAND-DOC-001 | `api/openapi.yaml` | planned |

## Implementation Layer Traceability

| Layer/Concern | Task ID | Status |
| --- | --- | --- |
| Domain models | LAND-BE-001 | done |
| Admin DTO | LAND-BE-002 | done |
| Public DTO | LAND-BE-003 | done |
| Page repository | LAND-REPO-001 | in_progress |
| Section repository | LAND-REPO-002 | in_progress |
| Form repository | LAND-REPO-003 | planned |
| Submission repository | LAND-REPO-004 | planned |
| Version/domain/branding repository | LAND-REPO-005 | planned |
| Analytics repository | LAND-REPO-006 | planned |
| Reusable content/navigation repository | LAND-REPO-007 | planned |
| Media repository | LAND-REPO-008 | planned |
| Revision/schedule repository | LAND-REPO-009 | planned |
| Lead integration repository | LAND-REPO-010 | planned |
| Unit tests | LAND-TEST-001 | planned |
| Repository integration tests | LAND-TEST-002 | planned |
| Handler/public flow tests | LAND-TEST-003 | planned |
| Documentation synchronization | LAND-DOC-001 | planned |

## API Index

| Endpoint | Task | Permission | Status |
| --- | --- | --- | --- |
| `GET /admin/landing-pages` | LAND-API-001 | `landing.page.read` | planned |
| `POST /admin/landing-pages` | LAND-API-001 | `landing.page.create` | planned |
| `GET /admin/landing-pages/:id` | LAND-API-001 | `landing.page.read` | planned |
| `PATCH /admin/landing-pages/:id` | LAND-API-001 | `landing.page.update` | planned |
| `DELETE /admin/landing-pages/:id` | LAND-API-001 | `landing.page.delete` | planned |
| `POST /admin/landing-pages/:id/restore` | LAND-API-001 | `landing.page.restore` | planned |
| `POST /admin/landing-pages/:id/duplicate` | LAND-API-001 | `landing.page.create` | planned |
| `POST /admin/landing-pages/:id/archive` | LAND-API-001 | `landing.page.archive` | planned |
| `PATCH /admin/landing-pages/:id/seo` | LAND-API-001 | `landing.seo.manage` | planned |
| `POST /admin/landing-pages/:id/validate-publish` | LAND-API-001 | `landing.page.publish` | planned |
| `POST /admin/landing-pages/:id/publish` | LAND-API-001 | `landing.page.publish` | planned |
| `POST /admin/landing-pages/:id/unpublish` | LAND-API-001 | `landing.page.publish` | planned |
| `GET /admin/landing-pages/:id/versions` | LAND-API-001 | `landing.page.read` | planned |
| `GET /admin/landing-pages/:id/versions/:version` | LAND-API-001 | `landing.page.read` | planned |
| `POST /admin/landing-pages/:id/versions/:version/restore` | LAND-API-001 | `landing.page.publish` | planned |
| `POST /admin/landing-pages/:id/preview-token` | LAND-API-001 | `landing.preview` | planned |
| `PUT /admin/landing-pages/:id/access` | LAND-API-001 | `landing.page.update` | planned |
| `GET /admin/landing-pages/:id/sections` | LAND-API-001 | `landing.page.read` | planned |
| `POST /admin/landing-pages/:id/sections` | LAND-API-001 | `landing.section.manage` | planned |
| `PATCH /admin/landing-pages/:id/sections/:sectionId` | LAND-API-001 | `landing.section.manage` | planned |
| `POST /admin/landing-pages/:id/sections/:sectionId/duplicate` | LAND-API-001 | `landing.section.manage` | planned |
| `DELETE /admin/landing-pages/:id/sections/:sectionId` | LAND-API-001 | `landing.section.manage` | planned |
| `PUT /admin/landing-pages/:id/sections/reorder` | LAND-API-001 | `landing.section.manage` | planned |
| `GET /admin/landing-pages/:id/forms` | LAND-API-002 | `landing.page.read` | planned |
| `POST /admin/landing-pages/:id/forms` | LAND-API-002 | `landing.form.manage` | planned |
| `PATCH /admin/landing-pages/:id/forms/:formId` | LAND-API-002 | `landing.form.manage` | planned |
| `DELETE /admin/landing-pages/:id/forms/:formId` | LAND-API-002 | `landing.form.manage` | planned |
| `PUT /admin/landing-pages/:id/forms/:formId/fields` | LAND-API-002 | `landing.form.manage` | planned |
| `GET /admin/landing-submissions` | LAND-API-002 | `landing.submission.read` | planned |
| `GET /admin/landing-submissions/:id` | LAND-API-002 | `landing.submission.read` | planned |
| `PATCH /admin/landing-submissions/:id/status` | LAND-API-002 | `landing.submission.update` | planned |
| `POST /admin/landing-submissions/:id/notes` | LAND-API-002 | `landing.submission.update` | planned |
| `DELETE /admin/landing-submissions/:id` | LAND-API-002 | `landing.submission.delete` | planned |
| `GET /admin/landing-submissions/export` | LAND-API-002 | `landing.submission.export` | planned |
| `GET /admin/landing/branding` | LAND-API-003 | `landing.branding.read` | planned |
| `PATCH /admin/landing/branding` | LAND-API-003 | `landing.branding.update` | planned |
| `GET /admin/landing-pages/:id/branding` | LAND-API-003 | `landing.branding.read` | planned |
| `PATCH /admin/landing-pages/:id/branding` | LAND-API-003 | `landing.branding.update` | planned |
| `DELETE /admin/landing-pages/:id/branding` | LAND-API-003 | `landing.branding.update` | planned |
| `GET /admin/landing/theme` | LAND-API-003 | `landing.branding.read` | planned |
| `PATCH /admin/landing/theme` | LAND-API-003 | `landing.theme.manage` | planned |
| `GET /admin/landing/ctas` | LAND-API-005 | `landing.cta.manage` | planned |
| `POST /admin/landing/ctas` | LAND-API-005 | `landing.cta.manage` | planned |
| `PATCH /admin/landing/ctas/:id` | LAND-API-005 | `landing.cta.manage` | planned |
| `DELETE /admin/landing/ctas/:id` | LAND-API-005 | `landing.cta.manage` | planned |
| `GET /admin/landing/section-templates` | LAND-API-005 | `landing.section_template.manage` | planned |
| `POST /admin/landing/section-templates` | LAND-API-005 | `landing.section_template.manage` | planned |
| `PATCH /admin/landing/section-templates/:id` | LAND-API-005 | `landing.section_template.manage` | planned |
| `DELETE /admin/landing/section-templates/:id` | LAND-API-005 | `landing.section_template.manage` | planned |
| `POST /admin/landing-pages/:id/sections/from-template` | LAND-API-005 | `landing.section.manage` | planned |
| `GET /admin/landing/media` | LAND-API-005 | `landing.media.manage` | planned |
| `POST /admin/landing/media` | LAND-API-005 | `landing.media.manage` | planned |
| `PATCH /admin/landing/media/:id` | LAND-API-005 | `landing.media.manage` | planned |
| `DELETE /admin/landing/media/:id` | LAND-API-005 | `landing.media.manage` | planned |
| `GET /admin/landing/menus` | LAND-API-005 | `landing.menu.manage` | planned |
| `POST /admin/landing/menus` | LAND-API-005 | `landing.menu.manage` | planned |
| `PATCH /admin/landing/menus/:id` | LAND-API-005 | `landing.menu.manage` | planned |
| `DELETE /admin/landing/menus/:id` | LAND-API-005 | `landing.menu.manage` | planned |
| `PUT /admin/landing/menus/:id/items` | LAND-API-005 | `landing.menu.manage` | planned |
| `GET /admin/landing/domains/available` | LAND-API-003, MT-API-003 | `landing.domain.read` | planned |
| `GET /admin/landing/domain-bindings` | LAND-API-003 | `landing.domain.read` | planned |
| `POST /admin/landing/domain-bindings` | LAND-API-003 | `landing.domain.manage` | planned |
| `PATCH /admin/landing/domain-bindings/:id` | LAND-API-003 | `landing.domain.manage` | planned |
| `DELETE /admin/landing/domain-bindings/:id` | LAND-API-003 | `landing.domain.manage` | planned |
| `GET /admin/landing-analytics/summary` | LAND-API-003 | `landing.analytics.read` | planned |
| `GET /admin/landing-analytics/timeseries` | LAND-API-003 | `landing.analytics.read` | planned |
| `GET /admin/landing-analytics/sources` | LAND-API-003 | `landing.analytics.read` | planned |
| `GET /admin/landing-analytics/top-pages` | LAND-API-003 | `landing.analytics.read` | planned |
| `GET /admin/landing-analytics/devices` | LAND-API-003 | `landing.analytics.read` | planned |
| `GET /admin/landing-pages/:id/revisions` | LAND-API-006 | `landing.page.read` | planned |
| `GET /admin/landing-pages/:id/revisions/compare` | LAND-API-006 | `landing.page.read` | planned |
| `POST /admin/landing-pages/:id/revisions/:revision/restore` | LAND-API-006 | `landing.page.update` | planned |
| `PUT /admin/landing-pages/:id/schedule` | LAND-API-006 | `landing.page.publish` | planned |
| `DELETE /admin/landing-pages/:id/schedule` | LAND-API-006 | `landing.page.publish` | planned |
| `GET /admin/landing/lead-integrations` | LAND-API-006 | `landing.integration.read` | planned |
| `POST /admin/landing/lead-integrations` | LAND-API-006 | `landing.integration.manage` | planned |
| `PATCH /admin/landing/lead-integrations/:id` | LAND-API-006 | `landing.integration.manage` | planned |
| `DELETE /admin/landing/lead-integrations/:id` | LAND-API-006 | `landing.integration.manage` | planned |
| `POST /admin/landing/lead-integrations/:id/test` | LAND-API-006 | `landing.integration.manage` | planned |
| `GET /admin/landing/lead-deliveries` | LAND-API-006 | `landing.integration.read` | planned |
| `POST /admin/landing/lead-deliveries/:id/retry` | LAND-API-006 | `landing.integration.manage` | planned |
| `GET /public/landing/resolve` | LAND-API-004 | Public | planned |
| `POST /public/landing/access/:publicPageId` | LAND-API-004 | Public/rate limited | planned |
| `POST /public/landing/forms/:publicKey/uploads` | LAND-API-004 | Public/rate limited | planned |
| `GET /public/landing/preview/:token` | LAND-API-004 | Preview token | planned |
| `POST /public/landing/forms/:publicKey/submissions` | LAND-API-004 | Public/rate limited | planned |
| `POST /public/landing/events` | LAND-API-004 | Public/rate limited | planned |

## Migration Index

Nama file final mengikuti nomor migration berikutnya saat implementasi.

| Logical Migration | Task | Status |
| --- | --- | --- |
| Audit/cleanup legacy landing schema | LAND-DB-001 | done |
| `000025_create_landing_page_core_tables` | LAND-DB-002 | done |
| `000026_create_landing_form_tables` | LAND-DB-003 | done |
| `000027_create_landing_version_tables` | LAND-DB-004 | done |
| `000028_create_landing_branding_tables` | LAND-DB-005 | done |
| `000029_create_landing_analytics_tables` | LAND-DB-006 | done |
| `000030_create_landing_reusable_tables` | LAND-DB-007 | done |
| `000031_create_landing_media_tables` | LAND-DB-008 | done |
| `000032_create_landing_revision_tables` | LAND-DB-009 | done |
| `000033_create_landing_integration_tables` | LAND-DB-010 | done |
| Seed permissions | LAND-SEED-001 | planned |
| Seed optional presets | LAND-SEED-002 | planned |

## Permission Traceability

| Permission | Capability | Status |
| --- | --- | --- |
| `landing.page.read` | Page/version/section/form read | planned |
| `landing.page.create` | Create/duplicate | planned |
| `landing.page.update` | Update page/SEO | planned |
| `landing.page.delete` | Soft delete | planned |
| `landing.page.restore` | Restore | planned |
| `landing.page.publish` | Validate/publish/unpublish/version restore | planned |
| `landing.page.archive` | Archive | planned |
| `landing.section.manage` | Section mutation | planned |
| `landing.section_template.manage` | Reusable section template | planned |
| `landing.cta.manage` | Reusable CTA | planned |
| `landing.media.manage` | Landing media | planned |
| `landing.form.manage` | Form/field mutation | planned |
| `landing.submission.read` | Submission read | planned |
| `landing.submission.update` | Status/note | planned |
| `landing.submission.delete` | Submission delete | planned |
| `landing.submission.export` | CSV export | planned |
| `landing.seo.manage` | SEO mutation | planned |
| `landing.branding.read` | Branding read | planned |
| `landing.branding.update` | Branding mutation | planned |
| `landing.theme.manage` | Theme/layout mutation | planned |
| `landing.menu.manage` | Navigation/menu | planned |
| `landing.domain.read` | Domain read | planned |
| `landing.domain.manage` | Domain mutation/verification | planned |
| `landing.analytics.read` | Analytics dashboard | planned |
| `landing.preview` | Draft preview | planned |
| `landing.integration.read` | Lead delivery logs | planned |
| `landing.integration.manage` | Lead integration configuration/retry | planned |

## Event Traceability

| Event | Producer | Consumer | Status |
| --- | --- | --- | --- |
| `landing.submission_created` | LAND-BE-031 | Notification outbox/worker | planned |
| `landing.page_published` | LAND-BE-041 | Notification/audit/cache integration | planned |
| `landing.domain_verification_failed` | LAND-BE-050 | Notification outbox/worker | planned |
| `landing.lead_delivery_failed` | LAND-BE-033 | Notification outbox/worker | planned |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-13 | `internal/modules/landing` adalah target canonical | Requirement user; repo masih punya placeholder `landingpage` |
| 2026-06-13 | Form, submission, lead status, branding, dan analytics tetap di Landing | User menolak pemecahan module |
| 2026-06-13 | Storage dan notification tetap integration boundary | Keduanya adalah platform/core capability yang sudah ada |
| 2026-06-13 | API contract dibuat saat planning dan disinkronkan setelah implementasi | Vue membutuhkan contract; detail final mengikuti OpenAPI dan behavior teruji |
| 2026-06-13 | Legacy `landing_pages` dan `brands` diaudit sebelum migration | Baseline arsip berbeda dari target |
| 2026-06-13 | Attachment percakapan lengkap menjadi source requirement | Menggantikan keterbatasan akses halaman share anonim |
| 2026-06-13 | Requirement advanced dicatat sebagai deferred task | Requirement tetap traceable tanpa memperbesar MVP |
| 2026-06-13 | LAND-0003 bergantung pada capability multi-tenant | Tenant context dan isolation adalah shared platform concern, bukan khusus Landing |
| 2026-06-13 | Organization owns domain verification; Landing owns page binding | Menghindari duplicate domain registry dan inconsistent SSL/host state |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-13 | Initial planning documentation | Reference, tasks, traceability, API contract | Branding masuk MVP dan module tidak dipecah |
| 2026-06-13 | Full source requirement audit | Semua dokumen Landing Page | Menambahkan visibility, schedule, CTA, reusable template, media, menu, revision, lead integration, theme detail, dan advanced roadmap |
| 2026-06-13 | Completed module naming and skeleton | `internal/modules/landing`, `internal/app/module_routes.go`, `README.md` | Menyelesaikan LAND-0001 dan LAND-0002 tanpa endpoint atau dependency baru |
| 2026-06-13 | Started permission and domain foundation | `internal/modules/landing/domain` | Menambahkan permission catalog serta model/enum awal page, section, form, dan submission dengan unit test |
| 2026-06-13 | Multi-tenant domain schema dependency available | `migrations/000015_create_organization_domains.*.sql` | Organization host ownership, verification, SSL state, and reserved labels are ready; Landing binding and resolver remain planned |
| 2026-06-13 | Multi-tenant entitlement schema dependency available | `migrations/000016_create_organization_entitlements.*.sql` | Landing feature and quota records are ready; effective entitlement evaluation and route guards remain planned |
| 2026-06-14 | Multi-tenant domain repository dependency available | `internal/modules/organization/repository` | Active verified host lookup and safe reassignment are ready; Landing page binding and HTTP host resolver remain planned |
| 2026-06-14 | Multi-tenant entitlement repository dependency available | `internal/modules/organization/repository` | Effective source precedence and atomic usage counters are ready; Landing entitlement policy and route guards remain planned |
| 2026-06-16 | LAND-0003 dependency completed | `internal/modules/landing/service` | Landing admin/public scope contract now requires verified context, permission, and `landing.enabled` entitlement before repository access |
| 2026-06-17 | Completed Landing schema audit | `docs/landing-page-schema-audit.md` | Active dev/test schema has no legacy `landing_pages` or `brands`; archived baseline is incompatible with target tenant ownership, so new Landing tables can start at `000025` without cleanup |
| 2026-06-17 | Added Landing page core tables | `migrations/000025_create_landing_page_core_tables.*.sql` | Created tenant-owned pages and sections with organization-scoped uniqueness, soft delete, JSON object constraints, and RLS policies |
| 2026-06-17 | Completed remaining Landing migrations and domain models | `migrations/000026..000033`, `internal/modules/landing/domain/*.go` | Created remaining tables and Go domain models for branding, analytics, reusable content, media, revisions, and integrations |
| 2026-06-17 | Completed Landing Page request and response DTOs | `internal/modules/landing/dto/*.go` | Created admin and public DTO structs matching API contract and verified with unit tests |
| 2026-06-17 | Completed development audit and task tracking setup | `docs/landing-page-development-tasks.md`, `docs/landing-page-traceability-index.md` | Audit task status for Phase 3-12 and added Risk/Follow-up notes for all remaining tasks |

## Update Rules

- Ubah task menjadi `in_progress` saat mulai.
- Update API contract/OpenAPI saat endpoint berubah.
- Isi nama migration aktual setelah dibuat.
- Ubah ke `done` hanya setelah verifikasi.
- Catat keputusan dan milestone baru.
