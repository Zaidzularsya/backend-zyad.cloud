-- Mailbox email per user (IMAP/SMTP milik user sendiri) untuk kirim & baca
-- email dari CRM. Password disimpan terenkripsi (core/crypto.EncryptSecret),
-- tidak pernah dikembalikan di response. Kolom IMAP dipakai sinkronisasi
-- inbox (fase berikutnya); pengiriman hanya butuh SMTP.
CREATE TABLE IF NOT EXISTS user_mailboxes (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	user_id uuid NOT NULL,
	email_address varchar(255) NOT NULL,
	display_name varchar(150),
	username varchar(255) NOT NULL,
	secret_encrypted text NOT NULL,
	smtp_host varchar(255) NOT NULL,
	smtp_port integer NOT NULL,
	smtp_security varchar(10) NOT NULL,
	imap_host varchar(255),
	imap_port integer,
	imap_security varchar(10),
	status varchar(20) NOT NULL DEFAULT 'active',
	last_error text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT user_mailboxes_org_id_unique UNIQUE (organization_id, id),
	-- Tidak ada opsi plaintext: kredensial selalu lewat TLS.
	CONSTRAINT user_mailboxes_smtp_security_check CHECK (smtp_security IN ('ssl', 'starttls')),
	CONSTRAINT user_mailboxes_imap_security_check CHECK (imap_security IS NULL OR imap_security IN ('ssl', 'starttls')),
	CONSTRAINT user_mailboxes_status_check CHECK (status IN ('active', 'error', 'disabled')),
	CONSTRAINT fk_user_mailboxes_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_user_mailboxes_user_id
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_mailboxes_org_user_email
	ON user_mailboxes(organization_id, user_id, lower(email_address));

SELECT apply_organization_rls('user_mailboxes'::regclass);

-- Email keluar (dan nanti masuk hasil sync) milik satu mailbox.
-- participants = semua alamat from/to/cc/bcc (lowercase) untuk filter
-- "email dengan contact X" lewat index GIN.
CREATE TABLE IF NOT EXISTS mail_messages (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	mailbox_id uuid NOT NULL,
	direction varchar(10) NOT NULL,
	status varchar(10) NOT NULL,
	message_id varchar(998) NOT NULL,
	in_reply_to varchar(998),
	references_header text,
	from_address varchar(255) NOT NULL,
	from_name varchar(255),
	to_addresses text[] NOT NULL DEFAULT '{}',
	cc_addresses text[] NOT NULL DEFAULT '{}',
	bcc_addresses text[] NOT NULL DEFAULT '{}',
	participants text[] NOT NULL DEFAULT '{}',
	subject text NOT NULL DEFAULT '',
	snippet varchar(300) NOT NULL DEFAULT '',
	body_html text NOT NULL DEFAULT '',
	body_text text NOT NULL DEFAULT '',
	error text,
	client_request_id varchar(100),
	related_entity_type varchar(20),
	related_entity_id uuid,
	sent_at timestamp without time zone,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT mail_messages_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT mail_messages_mailbox_message_id_unique UNIQUE (mailbox_id, message_id),
	-- Idempotency kirim: klik Send ganda dengan key sama = satu email.
	CONSTRAINT mail_messages_mailbox_client_request_unique UNIQUE (mailbox_id, client_request_id),
	CONSTRAINT mail_messages_direction_check CHECK (direction IN ('inbound', 'outbound')),
	CONSTRAINT mail_messages_status_check CHECK (status IN ('queued', 'sent', 'failed', 'received')),
	CONSTRAINT mail_messages_related_entity_check CHECK (
		(related_entity_type IS NULL) = (related_entity_id IS NULL)
		AND (related_entity_type IS NULL OR related_entity_type IN ('lead', 'contact'))
	),
	CONSTRAINT fk_mail_messages_mailbox
		FOREIGN KEY (organization_id, mailbox_id)
		REFERENCES user_mailboxes(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_mail_messages_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_mail_messages_org_mailbox_created
	ON mail_messages(organization_id, mailbox_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mail_messages_participants
	ON mail_messages USING GIN (participants);

SELECT apply_organization_rls('mail_messages'::regclass);

-- Lampiran email: file-nya milik modul asset (kuota storage tenant).
CREATE TABLE IF NOT EXISTS mail_message_attachments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	message_id uuid NOT NULL,
	asset_object_id uuid NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT mail_message_attachments_asset_object_unique UNIQUE (asset_object_id),
	CONSTRAINT fk_mail_message_attachments_message
		FOREIGN KEY (organization_id, message_id)
		REFERENCES mail_messages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_mail_message_attachments_asset_object
		FOREIGN KEY (asset_object_id) REFERENCES asset_objects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mail_message_attachments_org_message
	ON mail_message_attachments(organization_id, message_id);

SELECT apply_organization_rls('mail_message_attachments'::regclass);
