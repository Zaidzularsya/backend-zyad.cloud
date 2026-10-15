CREATE TABLE IF NOT EXISTS wa_conversations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	session_id uuid NOT NULL,
	-- WAHA chat id, e.g. 628123456789@c.us or 1234@lid.
	chat_id varchar(120) NOT NULL,
	phone_normalized varchar(20),
	contact_name varchar(200),
	-- Polymorphic link to a CRM lead/contact; no FK, same as crm_activities.
	related_entity_type varchar(20),
	related_entity_id uuid,
	assignee_user_id uuid,
	last_message_at timestamp without time zone,
	last_message_preview varchar(200),
	unread_count integer NOT NULL DEFAULT 0,
	status varchar(10) NOT NULL DEFAULT 'open',
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT wa_conversations_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT wa_conversations_session_chat_unique UNIQUE (session_id, chat_id),
	CONSTRAINT wa_conversations_related_entity_type_check CHECK (
		related_entity_type IS NULL OR related_entity_type IN ('lead', 'contact')
	),
	CONSTRAINT wa_conversations_related_entity_pair_check CHECK (
		(related_entity_type IS NULL) = (related_entity_id IS NULL)
	),
	CONSTRAINT wa_conversations_unread_count_check CHECK (unread_count >= 0),
	CONSTRAINT wa_conversations_status_check CHECK (status IN ('open', 'closed')),
	CONSTRAINT fk_wa_conversations_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_wa_conversations_session
		FOREIGN KEY (organization_id, session_id) REFERENCES wa_sessions(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_wa_conversations_assignee_user_id
		FOREIGN KEY (assignee_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_wa_conversations_related_entity
	ON wa_conversations(organization_id, related_entity_type, related_entity_id);

CREATE INDEX IF NOT EXISTS idx_wa_conversations_assignee_recent
	ON wa_conversations(organization_id, assignee_user_id, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_wa_conversations_phone
	ON wa_conversations(organization_id, phone_normalized);

SELECT apply_organization_rls('wa_conversations'::regclass);

CREATE TABLE IF NOT EXISTS wa_messages (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	conversation_id uuid NOT NULL,
	waha_message_id varchar(200),
	direction varchar(3) NOT NULL,
	body text,
	status varchar(10) NOT NULL DEFAULT 'pending',
	error text,
	sent_by_user_id uuid,
	sent_at timestamp without time zone NOT NULL DEFAULT now(),
	-- Selected WAHA fields only; the engine's raw _data is not stored (personal data).
	raw jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT wa_messages_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT wa_messages_direction_check CHECK (direction IN ('in', 'out')),
	CONSTRAINT wa_messages_status_check CHECK (
		status IN ('pending', 'sent', 'delivered', 'read', 'failed')
	),
	CONSTRAINT wa_messages_raw_object_check CHECK (jsonb_typeof(raw) = 'object'),
	CONSTRAINT fk_wa_messages_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_wa_messages_conversation
		FOREIGN KEY (organization_id, conversation_id) REFERENCES wa_conversations(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_wa_messages_sent_by_user_id
		FOREIGN KEY (sent_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_wa_messages_conversation_waha_id
	ON wa_messages(conversation_id, waha_message_id)
	WHERE waha_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wa_messages_conversation_sent
	ON wa_messages(conversation_id, sent_at DESC);

SELECT apply_organization_rls('wa_messages'::regclass);
