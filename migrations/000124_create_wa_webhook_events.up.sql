-- Raw inbox for WAHA webhooks: gives idempotency (event_id is WAHA's ULID) and
-- lets the worker retry/reprocess. Intentionally NOT under RLS, like
-- notification_outbox_events: rows arrive before the organization is known and
-- are claimed across tenants by the worker, which resolves the organization via
-- wa_session_directory. Never exposed through tenant endpoints.
CREATE TABLE IF NOT EXISTS wa_webhook_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	event_id varchar(64) NOT NULL,
	session_name varchar(64) NOT NULL,
	event_type varchar(60) NOT NULL,
	payload jsonb NOT NULL,
	attempts integer NOT NULL DEFAULT 0,
	error text,
	received_at timestamp without time zone NOT NULL DEFAULT now(),
	next_retry_at timestamp without time zone NOT NULL DEFAULT now(),
	processed_at timestamp without time zone,
	CONSTRAINT wa_webhook_events_event_id_unique UNIQUE (event_id),
	CONSTRAINT wa_webhook_events_attempts_check CHECK (attempts >= 0),
	CONSTRAINT wa_webhook_events_payload_object_check CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_wa_webhook_events_pending
	ON wa_webhook_events(next_retry_at)
	WHERE processed_at IS NULL;
