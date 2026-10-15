-- Menutup note yang tersimpan 'pending' sebelum aturan R10 (lead playbook spec §10).
-- Cara pakai (psql, sebagai role migrasi):
--   1) DRY RUN : \set apply false  lalu \i 2026-09-30-complete-pending-notes.sql
--   2) APPLY   : \set apply true   lalu \i ...  (hanya setelah PO menyetujui hasil dry-run)
-- crm_activities memakai RLS FORCE → loop per organisasi.
\if :apply
\else
\echo 'DRY RUN — tidak ada perubahan'
\endif

CREATE TEMP TABLE IF NOT EXISTS pending_note_report (organization_id uuid, total int, samples text);

DO $$
DECLARE org record;
BEGIN
	FOR org IN SELECT id FROM organizations LOOP
		PERFORM set_config('app.organization_id', org.id::text, true);
		INSERT INTO pending_note_report
		SELECT org.id, COUNT(*), string_agg(subject, ' | ' ORDER BY created_at) FILTER (WHERE rn <= 5)
		FROM (
			SELECT subject, created_at, row_number() OVER (ORDER BY created_at) AS rn
			FROM crm_activities
			WHERE organization_id = org.id AND type = 'note' AND status = 'pending' AND deleted_at IS NULL
		) s
		HAVING COUNT(*) > 0;
	END LOOP;
	PERFORM set_config('app.organization_id', '', true);
END $$;

SELECT * FROM pending_note_report ORDER BY total DESC;

\if :apply
DO $$
DECLARE org record;
BEGIN
	FOR org IN SELECT organization_id AS id FROM pending_note_report LOOP
		PERFORM set_config('app.organization_id', org.id::text, true);
		UPDATE crm_activities SET status = 'completed', completed_at = created_at, updated_at = NOW()
		WHERE organization_id = org.id AND type = 'note' AND status = 'pending' AND deleted_at IS NULL;
	END LOOP;
	PERFORM set_config('app.organization_id', '', true);
END $$;
\echo 'APPLIED'
\endif
