-- Riwayat perubahan lead (dibuat, status berubah, di-assign, convert, hapus,
-- restore). Ditulis di transaksi yang sama dengan UPDATE crm_leads supaya
-- feed "Aktivitas terbaru" dan tren status di dashboard lead tidak pernah
-- berbeda dari state lead yang sebenarnya.
CREATE TABLE IF NOT EXISTS crm_lead_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	lead_id uuid NOT NULL,
	event_type varchar(30) NOT NULL,
	-- status lama/baru untuk status_changed/converted/created, user id owner
	-- lama/baru untuk assigned.
	from_value varchar(64),
	to_value varchar(64),
	actor_user_id uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_lead_events_event_type_check CHECK (
		event_type IN ('created', 'status_changed', 'assigned', 'converted', 'deleted', 'restored')
	),
	CONSTRAINT fk_crm_lead_events_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_lead_events_lead
		FOREIGN KEY (organization_id, lead_id)
		REFERENCES crm_leads(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_crm_lead_events_actor_user_id
		FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_lead_events_org_created
	ON crm_lead_events(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_crm_lead_events_org_lead
	ON crm_lead_events(organization_id, lead_id, created_at DESC);

SELECT apply_organization_rls('crm_lead_events'::regclass);

-- Agregasi dashboard lead per rentang tanggal.
CREATE INDEX IF NOT EXISTS idx_crm_leads_organization_created_at
	ON crm_leads(organization_id, created_at)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_leads_organization_converted_at
	ON crm_leads(organization_id, converted_at)
	WHERE converted_at IS NOT NULL AND deleted_at IS NULL;

-- Backfill event yang bisa diturunkan dari kolom lead yang sudah ada
-- (created_at, converted_at). Riwayat status di antaranya memang tidak
-- tercatat sebelum migration ini. crm_leads memakai RLS FORCE, jadi
-- app.organization_id di-set per organization.
DO $$
DECLARE
	org record;
BEGIN
	FOR org IN SELECT DISTINCT id FROM organizations LOOP
		PERFORM set_config('app.organization_id', org.id::text, true);

		INSERT INTO crm_lead_events (organization_id, lead_id, event_type, to_value, actor_user_id, created_at)
		SELECT l.organization_id, l.id, 'created', 'new', l.created_by, l.created_at
		FROM crm_leads l
		WHERE l.organization_id = org.id
			AND NOT EXISTS (
				SELECT 1 FROM crm_lead_events e
				WHERE e.organization_id = l.organization_id AND e.lead_id = l.id AND e.event_type = 'created'
			);

		INSERT INTO crm_lead_events (organization_id, lead_id, event_type, to_value, actor_user_id, created_at)
		SELECT l.organization_id, l.id, 'converted', 'converted', l.updated_by, l.converted_at
		FROM crm_leads l
		WHERE l.organization_id = org.id
			AND l.converted_at IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM crm_lead_events e
				WHERE e.organization_id = l.organization_id AND e.lead_id = l.id AND e.event_type = 'converted'
			);
	END LOOP;

	PERFORM set_config('app.organization_id', '', true);
END;
$$;
