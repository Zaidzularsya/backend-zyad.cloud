SELECT remove_organization_rls('wa_messages'::regclass);

DROP TABLE IF EXISTS wa_messages;

SELECT remove_organization_rls('wa_conversations'::regclass);

DROP TABLE IF EXISTS wa_conversations;
