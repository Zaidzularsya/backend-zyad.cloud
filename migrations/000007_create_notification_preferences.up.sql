CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notification_preferences (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	organization_id uuid,
	event_type varchar(150) NOT NULL,
	channel varchar(50) NOT NULL,
	is_enabled boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT notification_preferences_channel_check CHECK (
		channel IN ('email', 'whatsapp', 'in_app', 'discord')
	),
	CONSTRAINT fk_notification_preferences_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_preferences_user_org_event_channel_unique
	ON notification_preferences(user_id, organization_id, event_type, channel)
	WHERE organization_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_preferences_user_global_event_channel_unique
	ON notification_preferences(user_id, event_type, channel)
	WHERE organization_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_id
	ON notification_preferences(user_id);

CREATE INDEX IF NOT EXISTS idx_notification_preferences_organization_id
	ON notification_preferences(organization_id);

CREATE INDEX IF NOT EXISTS idx_notification_preferences_event_channel
	ON notification_preferences(event_type, channel);
