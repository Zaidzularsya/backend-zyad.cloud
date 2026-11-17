-- Isian setup isolir terakhir yang berhasil diterapkan per router (pool, profil, rate limit, redirect).
-- Host allowlist sengaja tidak disimpan: diturunkan dari konfigurasi platform setiap kali.
ALTER TABLE mikrotik_routers ADD COLUMN IF NOT EXISTS isolir_setup jsonb NULL;
