CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notification_logs (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	event_id uuid,
	event_type varchar(150),
	template_id uuid,
	template_code varchar(150),
	template_version integer,
	organization_id uuid,
	channel varchar(50) NOT NULL,
	recipient_type varchar(50) NOT NULL DEFAULT 'user',
	recipient_user_id uuid,
	recipient_name_snapshot varchar(150),
	recipient_email_snapshot varchar(150),
	recipient_phone_snapshot varchar(50),
	destination varchar(255) NOT NULL,
	subject text,
	body text NOT NULL,
	status varchar(50) NOT NULL DEFAULT 'pending',
	provider varchar(100),
	provider_message_id varchar(255),
	provider_response jsonb NOT NULL DEFAULT '{}'::jsonb,
	attempts integer NOT NULL DEFAULT 0,
	max_attempts integer NOT NULL DEFAULT 3,
	next_retry_at timestamp without time zone,
	error_message text,
	sent_at timestamp without time zone,
	failed_at timestamp without time zone,
	cancelled_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT notification_logs_channel_check CHECK (
		channel IN ('email', 'whatsapp', 'in_app', 'discord')
	),
	CONSTRAINT notification_logs_status_check CHECK (
		status IN ('pending', 'processing', 'sent', 'failed', 'cancelled', 'dead')
	),
	CONSTRAINT notification_logs_attempts_check CHECK (attempts >= 0),
	CONSTRAINT notification_logs_max_attempts_check CHECK (max_attempts > 0),
	CONSTRAINT notification_logs_attempts_max_check CHECK (attempts <= max_attempts),
	CONSTRAINT fk_notification_logs_template_id FOREIGN KEY (template_id) REFERENCES notification_templates(id) ON DELETE SET NULL,
	CONSTRAINT fk_notification_logs_recipient_user_id FOREIGN KEY (recipient_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_logs_event_type
	ON notification_logs(event_type);

CREATE INDEX IF NOT EXISTS idx_notification_logs_template_id
	ON notification_logs(template_id);

CREATE INDEX IF NOT EXISTS idx_notification_logs_template_code
	ON notification_logs(template_code);

CREATE INDEX IF NOT EXISTS idx_notification_logs_organization_id
	ON notification_logs(organization_id);

CREATE INDEX IF NOT EXISTS idx_notification_logs_recipient_user_id
	ON notification_logs(recipient_user_id);

CREATE INDEX IF NOT EXISTS idx_notification_logs_channel_status
	ON notification_logs(channel, status);

CREATE INDEX IF NOT EXISTS idx_notification_logs_provider_message_id
	ON notification_logs(provider_message_id)
	WHERE provider_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notification_logs_retryable
	ON notification_logs(status, next_retry_at)
	WHERE status IN ('pending', 'failed') AND next_retry_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notification_logs_created_at
	ON notification_logs(created_at);
