SELECT remove_organization_rls('mail_message_attachments'::regclass);
DROP TABLE IF EXISTS mail_message_attachments;

SELECT remove_organization_rls('mail_messages'::regclass);
DROP TABLE IF EXISTS mail_messages;

SELECT remove_organization_rls('user_mailboxes'::regclass);
DROP TABLE IF EXISTS user_mailboxes;
