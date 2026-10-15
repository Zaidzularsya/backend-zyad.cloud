-- One-off data correction: shift DB-now() timestamps from WIB back to UTC.
--
-- Problem: before the UTC session fix, pool connections inherited the server
-- TimeZone (Asia/Jakarta), so every column filled by now() / DEFAULT now() /
-- "SET x = NOW()" holds WIB wall-clock in a "timestamp without time zone"
-- column, while the API reads it as UTC (+7h in the future).
--
-- This script shifts ONLY columns proven to be filled by the database's
-- now(). Columns written by Go (already UTC) are NOT touched, e.g.
--   crm_activities.due_at            (FE toISOString())
--   wa_messages.sent_at, wa_conversations.last_message_at, wa_sessions.last_status_at
--   wa_webhook_events.next_retry_at  (default now() but rewritten from Go on retry -> mixed)
--   mail_messages.sent_at            (Go; inbound came from the sender zone -> mixed)
--   user_mailboxes.last_synced_at
--
-- Safety:
--   * Run it ONCE, in the window where NO release that still uses a WIB
--     session is writing (stop pm2 zyad-api + zyad-worker, run, then start the
--     release that pins the session to UTC). Rows written after the fix are
--     already UTC and would be wrongly shifted if this ran afterwards.
--   * Idempotent per table/column via timestamp_fix_log.
--   * Runs as a superuser (postgres) because crm_* has FORCE RLS.
--
-- Usage (dry-run is the default and changes nothing):
--   sudo -u postgres psql -d platform -v ON_ERROR_STOP=1 -f scripts/db/fix_timestamp_utc_shift.sql
--   sudo -u postgres psql -d platform -v ON_ERROR_STOP=1 -v apply=yes -v app_stopped=yes \
--        -f scripts/db/fix_timestamp_utc_shift.sql
--
-- Rollback: shift back by +7h for the columns listed in timestamp_fix_log
-- (or restore the pre-run pg_dump; take one first).

\if :{?apply}
\else
\set apply no
\endif
\if :{?app_stopped}
\else
\set app_stopped no
\endif

SELECT set_config('tzfix.apply', :'apply', false);
SELECT set_config('tzfix.app_stopped', :'app_stopped', false);

BEGIN;

CREATE TABLE IF NOT EXISTS timestamp_fix_log (
    table_name   text        NOT NULL,
    column_name  text        NOT NULL,
    rows_shifted bigint      NOT NULL,
    applied_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (table_name, column_name)
);

DO $fix$
DECLARE
    targets constant text[][] := ARRAY[
        ['crm_activities', 'created_at'], ['crm_activities', 'updated_at'], ['crm_activities', 'completed_at'], ['crm_activities', 'deleted_at'],
        ['crm_companies', 'created_at'], ['crm_companies', 'updated_at'], ['crm_companies', 'deleted_at'],
        ['crm_contact_attachments', 'created_at'],
        ['crm_contacts', 'created_at'], ['crm_contacts', 'updated_at'], ['crm_contacts', 'deleted_at'],
        ['crm_deals', 'created_at'], ['crm_deals', 'updated_at'], ['crm_deals', 'deleted_at'], ['crm_deals', 'discount_approved_at'],
        ['crm_document_counters', 'created_at'], ['crm_document_counters', 'updated_at'],
        ['crm_integrations', 'created_at'], ['crm_integrations', 'updated_at'], ['crm_integrations', 'deleted_at'], ['crm_integrations', 'connected_at'],
        ['crm_lead_attachments', 'created_at'],
        ['crm_lead_events', 'created_at'],
        ['crm_leads', 'created_at'], ['crm_leads', 'updated_at'], ['crm_leads', 'deleted_at'], ['crm_leads', 'converted_at'],
        ['crm_pipeline_stages', 'created_at'], ['crm_pipeline_stages', 'updated_at'], ['crm_pipeline_stages', 'deleted_at'],
        ['crm_pipelines', 'created_at'], ['crm_pipelines', 'updated_at'], ['crm_pipelines', 'deleted_at'], ['crm_pipelines', 'archived_at'],
        ['crm_quotation_items', 'created_at'], ['crm_quotation_items', 'updated_at'],
        ['crm_quotations', 'created_at'], ['crm_quotations', 'updated_at'], ['crm_quotations', 'deleted_at'],
        ['crm_quotations', 'sent_at'], ['crm_quotations', 'approved_at'], ['crm_quotations', 'rejected_at'],
        ['mail_message_attachments', 'created_at'],
        ['mail_messages', 'created_at'], ['mail_messages', 'updated_at'],
        ['mailbox_directory', 'created_at'],
        ['user_mailboxes', 'created_at'], ['user_mailboxes', 'updated_at'],
        ['wa_conversations', 'created_at'], ['wa_conversations', 'updated_at'],
        ['wa_messages', 'created_at'], ['wa_messages', 'updated_at'],
        ['wa_session_directory', 'created_at'], ['wa_session_directory', 'deleted_at'],
        ['wa_sessions', 'created_at'], ['wa_sessions', 'updated_at'], ['wa_sessions', 'deleted_at'],
        ['wa_webhook_events', 'received_at'], ['wa_webhook_events', 'processed_at']
    ];
    apply_it boolean := current_setting('tzfix.apply') = 'yes';
    tbl text;
    col text;
    n bigint;
    min_before timestamp;
    min_after timestamp;
    i int;
BEGIN
    IF apply_it AND current_setting('tzfix.app_stopped') <> 'yes' THEN
        RAISE EXCEPTION 'refusing to apply: stop zyad-api/zyad-worker first and pass -v app_stopped=yes';
    END IF;

    FOR i IN 1 .. array_length(targets, 1) LOOP
        tbl := targets[i][1];
        col := targets[i][2];

        IF EXISTS (SELECT 1 FROM timestamp_fix_log WHERE table_name = tbl AND column_name = col) THEN
            RAISE NOTICE 'SKIP  %.% already corrected', tbl, col;
            CONTINUE;
        END IF;

        EXECUTE format('SELECT count(%1$I), min(%1$I) FROM %2$I', col, tbl) INTO n, min_before;
        min_after := (min_before AT TIME ZONE 'Asia/Jakarta') AT TIME ZONE 'UTC';
        RAISE NOTICE '% %.%: % rows, oldest % -> %', CASE WHEN apply_it THEN 'APPLY' ELSE 'DRY  ' END,
            tbl, col, n, min_before, min_after;

        IF apply_it THEN
            EXECUTE format(
                'UPDATE %2$I SET %1$I = (%1$I AT TIME ZONE ''Asia/Jakarta'') AT TIME ZONE ''UTC'' WHERE %1$I IS NOT NULL',
                col, tbl);
            INSERT INTO timestamp_fix_log (table_name, column_name, rows_shifted) VALUES (tbl, col, n);
        END IF;
    END LOOP;
END
$fix$;

-- Dry-run never persists anything (including timestamp_fix_log).
SELECT current_setting('tzfix.apply') = 'yes' AS applied \gset
\if :applied
COMMIT;
\else
ROLLBACK;
\endif
