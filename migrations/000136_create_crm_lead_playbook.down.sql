DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'crm_settings.update');
DELETE FROM permissions WHERE slug = 'crm_settings.update';

ALTER TABLE crm_lead_events DROP CONSTRAINT IF EXISTS crm_lead_events_event_type_check;
DELETE FROM crm_lead_events WHERE event_type IN ('playbook_started', 'playbook_ended');
ALTER TABLE crm_lead_events ADD CONSTRAINT crm_lead_events_event_type_check CHECK (
	event_type IN ('created', 'status_changed', 'assigned', 'converted', 'deleted', 'restored')
);

-- crm_leads memakai RLS FORCE: ubah attempting per organisasi.
DO $$
DECLARE org record;
BEGIN
	FOR org IN SELECT id FROM organizations LOOP
		PERFORM set_config('app.organization_id', org.id::text, true);
		UPDATE crm_leads SET status = 'contacted' WHERE organization_id = org.id AND status = 'attempting';
	END LOOP;
	PERFORM set_config('app.organization_id', '', true);
END $$;
ALTER TABLE crm_leads DROP CONSTRAINT IF EXISTS crm_leads_status_check;
ALTER TABLE crm_leads ADD CONSTRAINT crm_leads_status_check CHECK (
	status IN ('new', 'contacted', 'qualified', 'unqualified', 'converted')
);
ALTER TABLE crm_leads
	DROP COLUMN IF EXISTS disqualify_note,
	DROP COLUMN IF EXISTS disqualify_reason,
	DROP COLUMN IF EXISTS decision_maker,
	DROP COLUMN IF EXISTS target_date,
	DROP COLUMN IF EXISTS budget_estimate,
	DROP COLUMN IF EXISTS requirement_summary;

DROP INDEX IF EXISTS idx_crm_activities_playbook_run;
ALTER TABLE crm_activities
	DROP CONSTRAINT IF EXISTS fk_crm_activities_playbook_run,
	DROP COLUMN IF EXISTS final_review,
	DROP COLUMN IF EXISTS attempt_no,
	DROP COLUMN IF EXISTS outcome_key,
	DROP COLUMN IF EXISTS playbook_step_id,
	DROP COLUMN IF EXISTS playbook_run_id;

SELECT remove_organization_rls('crm_settings'::regclass);
DROP TABLE IF EXISTS crm_settings;
SELECT remove_organization_rls('crm_playbook_runs'::regclass);
DROP TABLE IF EXISTS crm_playbook_runs;
DROP TABLE IF EXISTS crm_playbook_outcomes;
DROP TABLE IF EXISTS crm_playbook_steps;
DROP TABLE IF EXISTS crm_playbooks;
