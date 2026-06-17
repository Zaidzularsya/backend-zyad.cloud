# Landing Page Schema Audit

Date: 2026-06-17

## Scope

This audit covers `LAND-DB-001` before creating new Landing Page migrations.

Reviewed objects:

- Active migrations in `migrations/`
- Development database `public` schema
- Test database `public` schema
- Archived monolithic baseline at
  `docs/archive/migrations/monolithic_baseline/000001_public_schema_baseline.up.sql`

## Active Schema State

No active migration currently creates `landing_pages` or `brands`.

The development database has migrations `000001` through `000024` applied and
does not contain `public.landing_pages` or `public.brands`.

The test database also does not contain `public.landing_pages` or
`public.brands`.

## Archived Baseline Findings

The archived `brands` table is not compatible with the target Landing branding
model:

- integer primary key instead of UUID;
- no `organization_id`;
- only `name`, `description`, and timestamps;
- no tenant isolation, asset references, color/theme tokens, contact metadata,
  or page override support.

The archived `landing_pages` table is not compatible with the target Landing
model:

- uses `owner_type` and nullable `owner_id` instead of `organization_id`;
- uses `code` instead of canonical `slug`;
- status values are uppercase and include legacy `SUSPENDED`;
- stores broad `config`, `seo`, and `cta` JSONB blobs on the page row;
- has `primary_domain` and `is_custom_domain`, duplicating the new
  `organization_domains` source of truth;
- has no section table, visibility model, soft delete timestamp, RLS policy, or
  page-level tenant unique slug contract.

## Mapping Decision

The active database has no legacy Landing rows to backfill, so no cleanup or
compatibility migration is required before `LAND-DB-002`.

If a future environment contains the archived tables, migrate it through a
separate cleanup/backfill migration before applying production Landing data
migrations. Do not reuse `owner_type`, nullable `owner_id`, or `brands` as the
new tenant ownership model.

## Next Migration Contract

`LAND-DB-002` should create new tenant-owned `landing_pages` and
`landing_page_sections` tables with:

- `organization_id uuid NOT NULL`;
- tenant-scoped unique slug rules;
- explicit status, page type, and visibility checks aligned with domain models;
- JSONB object checks for settings, SEO, section content, and section style;
- soft delete fields;
- `apply_organization_rls(...)` for both tables.
