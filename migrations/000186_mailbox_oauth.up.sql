-- Mailbox Microsoft (Outlook.com / Microsoft 365) tidak menerima basic
-- auth lagi untuk SMTP/IMAP, sehingga dihubungkan lewat OAuth 2.0 (SASL
-- XOAUTH2). Untuk auth_method = 'microsoft_oauth', secret_encrypted berisi
-- refresh token (terenkripsi, dirotasi setiap refresh). Access token di-cache
-- terenkripsi agar tidak memanggil token endpoint di setiap kirim/sync.
ALTER TABLE user_mailboxes
	ADD COLUMN IF NOT EXISTS auth_method varchar(20) NOT NULL DEFAULT 'password',
	ADD COLUMN IF NOT EXISTS access_token_encrypted text,
	ADD COLUMN IF NOT EXISTS access_token_expires_at timestamp without time zone;

ALTER TABLE user_mailboxes
	ADD CONSTRAINT user_mailboxes_auth_method_check
	CHECK (auth_method IN ('password', 'microsoft_oauth'));
