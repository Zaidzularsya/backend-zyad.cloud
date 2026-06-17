CREATE TABLE IF NOT EXISTS landing_analytics_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	version integer NOT NULL,
	event_name varchar(50) NOT NULL,
	section_key varchar(120),
	target_key varchar(120),
	session_id varchar(120),
	utm_source varchar(100),
	utm_medium varchar(100),
	utm_campaign varchar(100),
	utm_term varchar(100),
	utm_content varchar(100),
	referrer text,
	browser varchar(50),
	device varchar(50),
	os varchar(50),
	ip_address_hash varchar(64),
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_analytics_events_event_name_check CHECK (
		event_name IN (
			'page_view',
			'cta_click',
			'form_view',
			'form_start',
			'submission'
		)
	),
	CONSTRAINT landing_analytics_events_version_check CHECK (version > 0),
	CONSTRAINT fk_landing_analytics_events_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_analytics_events_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_landing_analytics_events_org_page_date
	ON landing_analytics_events(organization_id, landing_page_id, created_at);


CREATE TABLE IF NOT EXISTS landing_analytics_daily (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	date date NOT NULL,
	page_views integer NOT NULL DEFAULT 0,
	unique_visitors integer NOT NULL DEFAULT 0,
	cta_clicks integer NOT NULL DEFAULT 0,
	form_starts integer NOT NULL DEFAULT 0,
	submissions integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_analytics_daily_page_views_check CHECK (page_views >= 0),
	CONSTRAINT landing_analytics_daily_unique_visitors_check CHECK (unique_visitors >= 0),
	CONSTRAINT landing_analytics_daily_cta_clicks_check CHECK (cta_clicks >= 0),
	CONSTRAINT landing_analytics_daily_form_starts_check CHECK (form_starts >= 0),
	CONSTRAINT landing_analytics_daily_submissions_check CHECK (submissions >= 0),
	CONSTRAINT fk_landing_analytics_daily_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_analytics_daily_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_analytics_daily_org_page_date_unique
	ON landing_analytics_daily(organization_id, landing_page_id, date);

SELECT apply_organization_rls('landing_analytics_events'::regclass);
SELECT apply_organization_rls('landing_analytics_daily'::regclass);
