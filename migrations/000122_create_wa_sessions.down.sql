DROP TABLE IF EXISTS wa_session_directory;

SELECT remove_organization_rls('wa_sessions'::regclass);

DROP TABLE IF EXISTS wa_sessions;
