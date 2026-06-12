CREATE TABLE IF NOT EXISTS notification_outbox_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	event_type varchar(150) NOT NULL,
	organization_id uuid,
	user_id uuid,
	recipient jsonb NOT NULL DEFAULT '{}'::jsonb,
	payload jsonb NOT NULL DEFAULT '{}'::jsonb,
	locale varchar(20),
	status varchar(50) NOT NULL DEFAULT 'pending',
	attempts integer NOT NULL DEFAULT 0,
	max_attempts integer NOT NULL DEFAULT 3,
	next_retry_at timestamp without time zone,
	error_message text,
	processed_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT notification_outbox_events_status_check CHECK (
		status IN ('pending', 'processing', 'succeeded', 'failed', 'dead')
	),
	CONSTRAINT notification_outbox_events_attempts_check CHECK (attempts >= 0),
	CONSTRAINT notification_outbox_events_max_attempts_check CHECK (max_attempts > 0),
	CONSTRAINT notification_outbox_events_attempts_max_check CHECK (attempts <= max_attempts)
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_events_status_retry
	ON notification_outbox_events(status, next_retry_at);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_events_event_type
	ON notification_outbox_events(event_type);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_events_created_at
	ON notification_outbox_events(created_at);
