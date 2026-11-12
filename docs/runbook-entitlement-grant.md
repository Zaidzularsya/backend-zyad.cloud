# Runbook: memberi entitlement `platform_override` (mis. `mikrotik.enabled`)

## Masalah yang dicegah

Kolom `organization_entitlements.effective_from/effective_until` bertipe `timestamp without time zone`, dan
aplikasi membandingkannya dalam UTC. Insert manual dari sesi `psql` ber-zona WIB menulis jam lokal, sehingga
fitur baru aktif 7 jam kemudian dan semua endpoint menjawab 403 `ORGANIZATION_FEATURE_NOT_ENTITLED`
(temuan B2, 11 Nov 2026).

## Cara resmi: CLI

Dari direktori rilis backend (env DB sama dengan API):

```bash
go run ./cmd/entitlementgrant --org <organization-uuid> --feature mikrotik.enabled --days 90 --reason "pilot isolir" --by <user-uuid>
```

Atau binary hasil build (`go build -o entitlementgrant ./cmd/entitlementgrant`).

- Semua waktu dihitung dalam UTC; `effective_from` = sekarang dikurangi 1 menit.
- Wajib: `--org`, `--feature`, `--days` (1–3650), `--reason`, `--by` (UUID user pemberi). Input salah ditolak
  sebelum menyentuh database.
- Idempoten: menjalankan ulang untuk org + fitur yang sama memperbarui override yang ada (tidak menambah baris).
- Output: `Granted <feature> to <org> via platform_override until <UTC> (entitlement_id=...)`.

## Bila terpaksa SQL manual

Constraint `organization_entitlements_override_audit_check` mewajibkan untuk `source = 'platform_override'`:
`effective_until` terisi, `created_by` terisi, dan `reason` tidak kosong. Selalu UTC dan tampilkan dulu:

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT id, feature_key, source, status, effective_from, effective_until, reason
FROM organization_entitlements
WHERE organization_id = '<organization-uuid>' AND feature_key = 'mikrotik.enabled';
-- periksa hasil di atas sebelum INSERT/UPDATE apa pun
COMMIT;
```

`organization_entitlements` tidak ber-RLS (filter `organization_id` wajib manual). Tabel tenant lain yang
ber-RLS butuh `set_config('app.organization_id', '<uuid>', true)` di transaksi yang sama; lihat
`docs/multi-tenant-rls.md`.
