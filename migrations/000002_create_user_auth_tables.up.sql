CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name varchar(150) NOT NULL,
	email varchar(255) NOT NULL,
	username varchar(100),
	password_hash text,
	phone varchar(50),
	status varchar(30) NOT NULL DEFAULT 'pending',
	email_verified_at timestamp without time zone,
	phone_verified_at timestamp without time zone,
	last_login_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT users_status_check CHECK (
		status IN ('active', 'inactive', 'pending', 'suspended', 'banned', 'deleted', 'invited')
	)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active_unique
	ON users (lower(email))
	WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_active_unique
	ON users (lower(username))
	WHERE username IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

CREATE TABLE IF NOT EXISTS user_profiles (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL UNIQUE,
	avatar_url text,
	bio text,
	job_title varchar(150),
	department varchar(150),
	company varchar(150),
	address text,
	timezone varchar(100),
	language varchar(20),
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_user_profiles_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS auth_identities (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	provider varchar(50) NOT NULL,
	provider_user_id varchar(255) NOT NULL,
	provider_email varchar(255),
	access_token_hash text,
	refresh_token_hash text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT auth_identities_provider_check CHECK (
		provider IN ('local', 'google', 'github', 'gitlab', 'facebook', 'whatsapp')
	),
	CONSTRAINT auth_identities_provider_user_unique UNIQUE (provider, provider_user_id),
	CONSTRAINT fk_auth_identities_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_identities_user_id ON auth_identities(user_id);

CREATE TABLE IF NOT EXISTS sessions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	refresh_token_hash text NOT NULL,
	device_name varchar(150),
	user_agent text,
	ip_address inet,
	last_used_at timestamp without time zone,
	expires_at timestamp without time zone NOT NULL,
	revoked_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_sessions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_refresh_token_hash_unique ON sessions(refresh_token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_revoked_at ON sessions(revoked_at);

CREATE TABLE IF NOT EXISTS refresh_tokens (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	session_id uuid NOT NULL,
	user_id uuid NOT NULL,
	token_hash text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	revoked_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	replaced_by_token_id uuid,
	CONSTRAINT fk_refresh_tokens_session_id FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	CONSTRAINT fk_refresh_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	CONSTRAINT fk_refresh_tokens_replaced_by_token_id FOREIGN KEY (replaced_by_token_id) REFERENCES refresh_tokens(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash_unique ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_session_id ON refresh_tokens(session_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	token_hash text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	used_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_password_reset_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_password_reset_tokens_token_hash_unique ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	email varchar(255) NOT NULL,
	token_hash text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	used_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_email_verification_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_verification_tokens_token_hash_unique ON email_verification_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id ON email_verification_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires_at ON email_verification_tokens(expires_at);

CREATE TABLE IF NOT EXISTS otp_codes (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid,
	purpose varchar(50) NOT NULL,
	destination varchar(255) NOT NULL,
	code_hash text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	used_at timestamp without time zone,
	attempts integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT otp_codes_purpose_check CHECK (
		purpose IN ('login', 'verify_phone', 'reset_password', 'change_email')
	),
	CONSTRAINT otp_codes_attempts_check CHECK (attempts >= 0),
	CONSTRAINT fk_otp_codes_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_otp_codes_user_id ON otp_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_otp_codes_destination_purpose ON otp_codes(destination, purpose);
CREATE INDEX IF NOT EXISTS idx_otp_codes_expires_at ON otp_codes(expires_at);

CREATE TABLE IF NOT EXISTS login_histories (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid,
	identifier varchar(255),
	event varchar(50) NOT NULL,
	success boolean NOT NULL,
	ip_address inet,
	user_agent text,
	device_name varchar(150),
	reason text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT login_histories_event_check CHECK (
		event IN ('login', 'logout', 'failed_login', 'token_refresh', 'token_revoked')
	),
	CONSTRAINT fk_login_histories_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_login_histories_user_id ON login_histories(user_id);
CREATE INDEX IF NOT EXISTS idx_login_histories_identifier ON login_histories(identifier);
CREATE INDEX IF NOT EXISTS idx_login_histories_created_at ON login_histories(created_at);

CREATE TABLE IF NOT EXISTS audit_logs (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	module varchar(100) NOT NULL,
	event varchar(100) NOT NULL,
	actor_user_id uuid,
	target_user_id uuid,
	target_type varchar(100),
	target_id uuid,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	ip_address inet,
	user_agent text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_audit_logs_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_audit_logs_target_user_id FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_module_event ON audit_logs(module, event);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_user_id ON audit_logs(target_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
