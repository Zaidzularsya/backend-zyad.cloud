ALTER TABLE user_mailboxes DROP CONSTRAINT IF EXISTS user_mailboxes_auth_method_check;
ALTER TABLE user_mailboxes
	DROP COLUMN IF EXISTS access_token_expires_at,
	DROP COLUMN IF EXISTS access_token_encrypted,
	DROP COLUMN IF EXISTS auth_method;
