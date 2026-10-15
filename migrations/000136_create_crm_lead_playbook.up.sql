-- Lead playbook (SOP penanganan lead). Spec:
-- docs/superpowers/specs/2026-09-30-lead-playbook-design.md
-- Definisi (crm_playbooks/steps/outcomes) global tanpa RLS: Rilis 1 hanya
-- berisi seed sistem (organization_id NULL). Eksekusi (runs, settings) RLS.

CREATE TABLE IF NOT EXISTS crm_playbooks (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NULL REFERENCES organizations(id) ON DELETE CASCADE,
	entity_type varchar(20) NOT NULL CHECK (entity_type IN ('lead')),
	trigger varchar(40) NOT NULL CHECK (trigger IN ('lead_created')),
	key varchar(60) NOT NULL,
	name varchar(120) NOT NULL,
	version int NOT NULL DEFAULT 1,
	is_active boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_crm_playbooks_key_version
	ON crm_playbooks (COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid), key, version);

CREATE TABLE IF NOT EXISTS crm_playbook_steps (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	playbook_id uuid NOT NULL REFERENCES crm_playbooks(id) ON DELETE CASCADE,
	key varchar(60) NOT NULL,
	name varchar(120) NOT NULL,
	activity_type varchar(20) NOT NULL CHECK (activity_type IN ('call','email','meeting','task')),
	due_rule jsonb NOT NULL,
	channel_actions text[] NOT NULL DEFAULT '{}',
	entry_statuses text[] NOT NULL DEFAULT '{}',
	max_attempts int NULL CHECK (max_attempts IS NULL OR max_attempts >= 1),
	position int NOT NULL,
	CONSTRAINT uq_crm_playbook_steps_key UNIQUE (playbook_id, key)
);

CREATE TABLE IF NOT EXISTS crm_playbook_outcomes (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	step_id uuid NOT NULL REFERENCES crm_playbook_steps(id) ON DELETE CASCADE,
	key varchar(60) NOT NULL,
	label varchar(120) NOT NULL,
	required_input varchar(30) NOT NULL DEFAULT 'none'
		CHECK (required_input IN ('none','requirements','disqualify','reschedule')),
	set_lead_status varchar(20) NULL,
	disqualify_reason varchar(30) NULL,
	next_step_key varchar(60) NULL,
	next_due_rule jsonb NULL,
	repeat_step boolean NOT NULL DEFAULT false,
	ends_run boolean NOT NULL DEFAULT false,
	run_result varchar(20) NULL CHECK (run_result IS NULL OR run_result IN ('qualified','converted','disqualified','cancelled')),
	visibility varchar(20) NOT NULL DEFAULT 'normal' CHECK (visibility IN ('normal','final_review','both')),
	position int NOT NULL,
	CONSTRAINT uq_crm_playbook_outcomes_key UNIQUE (step_id, key)
);

CREATE TABLE IF NOT EXISTS crm_playbook_runs (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	playbook_id uuid NOT NULL REFERENCES crm_playbooks(id),
	playbook_version int NOT NULL,
	entity_type varchar(20) NOT NULL CHECK (entity_type IN ('lead')),
	entity_id uuid NOT NULL,
	current_step_id uuid NULL REFERENCES crm_playbook_steps(id),
	status varchar(20) NOT NULL CHECK (status IN ('active','completed','cancelled')),
	result varchar(20) NULL CHECK (result IS NULL OR result IN ('qualified','converted','disqualified','cancelled')),
	started_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	started_at timestamp without time zone NOT NULL DEFAULT now(),
	ended_at timestamp without time zone NULL,
	CONSTRAINT crm_playbook_runs_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT fk_crm_playbook_runs_lead FOREIGN KEY (organization_id, entity_id)
		REFERENCES crm_leads(organization_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_crm_playbook_runs_active
	ON crm_playbook_runs (organization_id, entity_type, entity_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_crm_playbook_runs_entity
	ON crm_playbook_runs (organization_id, entity_type, entity_id, started_at DESC);
SELECT apply_organization_rls('crm_playbook_runs'::regclass);

CREATE TABLE IF NOT EXISTS crm_settings (
	organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
	lead_playbook_enabled boolean NOT NULL DEFAULT true,
	updated_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	updated_at timestamp without time zone NOT NULL DEFAULT now()
);
SELECT apply_organization_rls('crm_settings'::regclass);

ALTER TABLE crm_activities
	ADD COLUMN IF NOT EXISTS playbook_run_id uuid NULL,
	ADD COLUMN IF NOT EXISTS playbook_step_id uuid NULL REFERENCES crm_playbook_steps(id),
	ADD COLUMN IF NOT EXISTS outcome_key varchar(60) NULL,
	ADD COLUMN IF NOT EXISTS attempt_no int NULL,
	ADD COLUMN IF NOT EXISTS final_review boolean NOT NULL DEFAULT false,
	ADD CONSTRAINT fk_crm_activities_playbook_run FOREIGN KEY (organization_id, playbook_run_id)
		REFERENCES crm_playbook_runs(organization_id, id) ON DELETE SET NULL (playbook_run_id);
CREATE INDEX IF NOT EXISTS idx_crm_activities_playbook_run
	ON crm_activities (organization_id, playbook_run_id) WHERE playbook_run_id IS NOT NULL;

ALTER TABLE crm_leads
	ADD COLUMN IF NOT EXISTS requirement_summary text NULL,
	ADD COLUMN IF NOT EXISTS budget_estimate numeric(18,2) NULL CHECK (budget_estimate IS NULL OR budget_estimate >= 0),
	ADD COLUMN IF NOT EXISTS target_date date NULL,
	ADD COLUMN IF NOT EXISTS decision_maker varchar(150) NULL,
	ADD COLUMN IF NOT EXISTS disqualify_reason varchar(30) NULL CHECK (disqualify_reason IS NULL OR disqualify_reason IN
		('unresponsive','not_interested','not_fit','budget','competitor','bad_data','duplicate','bad_timing')),
	ADD COLUMN IF NOT EXISTS disqualify_note text NULL;

ALTER TABLE crm_leads DROP CONSTRAINT IF EXISTS crm_leads_status_check;
ALTER TABLE crm_leads ADD CONSTRAINT crm_leads_status_check CHECK (
	status IN ('new', 'attempting', 'contacted', 'qualified', 'unqualified', 'converted')
);

ALTER TABLE crm_lead_events DROP CONSTRAINT IF EXISTS crm_lead_events_event_type_check;
ALTER TABLE crm_lead_events ADD CONSTRAINT crm_lead_events_event_type_check CHECK (
	event_type IN ('created', 'status_changed', 'assigned', 'converted', 'deleted', 'restored',
		'playbook_started', 'playbook_ended')
);

-- Seed playbook default "SOP Lead Masuk".
WITH pb AS (
	INSERT INTO crm_playbooks (organization_id, entity_type, trigger, key, name, version)
	VALUES (NULL, 'lead', 'lead_created', 'default_lead_sop', 'SOP Lead Masuk', 1)
	RETURNING id
),
first_contact AS (
	INSERT INTO crm_playbook_steps (playbook_id, key, name, activity_type, due_rule, channel_actions, entry_statuses, max_attempts, position)
	SELECT id, 'first_contact', 'Kontak pertama', 'call',
		'{"kind":"business_minutes","value":60}'::jsonb,
		ARRAY['whatsapp','email','call'], ARRAY['new','attempting'], 3, 1
	FROM pb RETURNING id
),
discovery AS (
	INSERT INTO crm_playbook_steps (playbook_id, key, name, activity_type, due_rule, channel_actions, entry_statuses, max_attempts, position)
	SELECT id, 'discovery', 'Gali kebutuhan', 'meeting',
		'{"kind":"business_days_at","value":1,"at":"09:00"}'::jsonb,
		ARRAY['whatsapp','email','call','schedule_meeting','requirements_form'], ARRAY['contacted'], NULL, 2
	FROM pb RETURNING id
)
INSERT INTO crm_playbook_outcomes
	(step_id, key, label, required_input, set_lead_status, disqualify_reason, next_step_key, next_due_rule, repeat_step, ends_run, run_result, visibility, position)
SELECT id, 'connected', 'Terhubung', 'none', 'contacted', NULL, 'discovery', NULL, false, false, NULL, 'both', 1 FROM first_contact
UNION ALL
SELECT id, 'no_response', 'Tidak respon', 'none', 'attempting', NULL, NULL,
	'{"kind":"business_days_at","by_attempt":[1,3],"at":"09:00"}'::jsonb, true, false, NULL, 'normal', 2 FROM first_contact
UNION ALL
SELECT id, 'call_back_later', 'Minta dihubungi nanti', 'reschedule', NULL, NULL, NULL, NULL, false, false, NULL, 'normal', 3 FROM first_contact
UNION ALL
SELECT id, 'bad_data', 'Data salah', 'none', 'unqualified', 'bad_data', NULL, NULL, false, true, 'disqualified', 'both', 4 FROM first_contact
UNION ALL
SELECT id, 'give_up', 'Unqualify — tidak responsif', 'none', 'unqualified', 'unresponsive', NULL, NULL, false, true, 'disqualified', 'final_review', 5 FROM first_contact
UNION ALL
SELECT id, 'qualified', 'Qualified', 'requirements', 'qualified', NULL, NULL, NULL, false, true, 'qualified', 'normal', 1 FROM discovery
UNION ALL
SELECT id, 'bad_timing', 'Belum waktunya', 'reschedule', NULL, NULL, NULL, NULL, false, false, NULL, 'normal', 2 FROM discovery
UNION ALL
SELECT id, 'not_fit', 'Tidak cocok / tidak tertarik', 'disqualify', 'unqualified', NULL, NULL, NULL, false, true, 'disqualified', 'normal', 3 FROM discovery;

-- Permission pengaturan CRM (toggle SOP lead otomatis).
WITH upserted AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	VALUES ('crm_settings.update', 'crm_settings', 'update', 'crm_settings.update', 'crm_settings.update',
		'Change CRM organization settings (lead SOP automation)', now(), now())
	ON CONFLICT (slug) DO UPDATE SET description = EXCLUDED.description, updated_at = now()
	RETURNING id
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT r.id, upserted.id, 'organization', now()
FROM roles r CROSS JOIN upserted
WHERE r.role_name IN ('organization_owner', 'super_admin') OR r.slug IN ('organization_owner', 'super_admin')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
