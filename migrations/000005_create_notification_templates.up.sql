CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notification_templates (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	code varchar(150) NOT NULL,
	name varchar(150) NOT NULL,
	description text,
	channel varchar(50) NOT NULL,
	locale varchar(20) NOT NULL DEFAULT 'id-ID',
	subject_template text,
	body_template text NOT NULL,
	available_variables jsonb NOT NULL DEFAULT '[]'::jsonb,
	sample_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
	status varchar(50) NOT NULL DEFAULT 'draft',
	is_system boolean NOT NULL DEFAULT false,
	is_active boolean NOT NULL DEFAULT true,
	version integer NOT NULL DEFAULT 1,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT notification_templates_channel_check CHECK (
		channel IN ('email', 'whatsapp', 'in_app', 'discord')
	),
	CONSTRAINT notification_templates_status_check CHECK (
		status IN ('draft', 'active', 'inactive', 'archived')
	),
	CONSTRAINT notification_templates_version_check CHECK (version > 0),
	CONSTRAINT fk_notification_templates_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_notification_templates_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT notification_templates_code_channel_locale_version_unique UNIQUE (code, channel, locale, version)
);

CREATE INDEX IF NOT EXISTS idx_notification_templates_code_channel_locale
	ON notification_templates(code, channel, locale);

CREATE INDEX IF NOT EXISTS idx_notification_templates_status
	ON notification_templates(status);

CREATE INDEX IF NOT EXISTS idx_notification_templates_is_active
	ON notification_templates(is_active);

CREATE INDEX IF NOT EXISTS idx_notification_templates_deleted_at
	ON notification_templates(deleted_at);
