-- Subdomain platform (type='subdomain') berada di zona DNS platform dan
-- TLS-nya ditanggung wildcard cert di reverse proxy, bukan diprovision
-- per-domain oleh aplikasi. Baris lama yang dibuat sebelum perbaikan ini
-- masih menyimpan ssl_status='pending' bawaan default insert — perbaiki
-- supaya konsisten dengan domain subdomain yang baru dibuat.
UPDATE organization_domains
SET
	ssl_status = 'not_required',
	updated_at = now()
WHERE type = 'subdomain'
	AND ssl_status = 'pending'
	AND deleted_at IS NULL;
