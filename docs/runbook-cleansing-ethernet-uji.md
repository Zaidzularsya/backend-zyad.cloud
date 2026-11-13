# Runbook: cleansing data uji organisasi `ethernet` (satu kali)

## Tujuan

Uji prod env-dev 11 Nov 2026 meninggalkan data uji di organisasi `ethernet`
(`4c3f4be2-f914-48ef-b8bb-70e9f920031d`): kontak ganda, lead dan deal uji, dua invoice uji webhook DOKU,
router lab beserta 85 secret, dan objek `zyad-managed` di router lab. Runbook ini membersihkannya sebelum
uji ulang Gelombang 2 (spec `docs/superpowers/specs/2026-11-12-mikrotik-gelombang2-onboarding-secret-design.md`
bagian 3.8, data uji di `docs/superpowers/notes/2026-11-11-mikrotik-uji-prod-temuan-bug.md` bagian "Sisa data uji").

Ini **bukan fitur produk**, hanya dijalankan sekali. Organisasi lain tidak boleh tersentuh: setiap kueri
memfilter `organization_id` dan berjalan di bawah RLS organisasi itu.

> **Setiap butir BUTUH PERSETUJUAN EKSPLISIT sebelum dijalankan.** Jalankan per butir, jangan sekaligus.
> Persetujuan untuk satu butir tidak berlaku untuk butir lain.

## Prasyarat

- Backup sebelum mulai (di server prod, kredensial dari `.env`, jangan ditempel ke percakapan):
  `pg_dump -Fc -f ~/backup/pre-cleansing-ethernet-$(date +%Y%m%d%H%M%S).dump <db>`.
- Sesi `psql` ke DB prod dengan role aplikasi (bukan superuser, supaya RLS benar-benar berlaku).
- Variabel sesi:

```sql
\set org '4c3f4be2-f914-48ef-b8bb-70e9f920031d'
\set ON_ERROR_ROLLBACK off
\set ON_ERROR_STOP on
```

## Pola tiap butir

Setiap butir SQL memakai pola yang sama:

1. `BEGIN;` lalu `SET LOCAL TimeZone = 'UTC';` dan `SELECT set_config('app.organization_id', :'org', true);`
   (tanpa `set_config`, tabel ber-RLS mengembalikan 0 baris, lihat B3).
2. `SELECT` pembuktian. Catat jumlah baris dan id, lalu cocokkan dengan "Hasil yang diharapkan".
3. `UPDATE`/`DELETE`. Jumlah baris yang terdampak (`UPDATE n`/`DELETE n`) harus sama dengan hasil `SELECT`.
4. `SELECT` verifikasi.
5. Jalankan dulu dengan `ROLLBACK;` sebagai dry run. Setelah disetujui, ulangi butir yang sama dengan `COMMIT;`.

Bila angka berbeda dari harapan: `ROLLBACK;`, berhenti, dan laporkan.

## Urutan

Butir 1 (router lab) dijalankan **pertama**, karena "Bersihkan" butuh router masih terdaftar dan terhubung.
Butir 5 (hapus router + secret) dijalankan **terakhir** di sisi Mikrotik, karena butir 2 masih membaca
pemasangan secret ke akun.

---

## Butir 1: objek `zyad-managed` di router lab (lewat UI) — butuh persetujuan sebelum dijalankan

Bukan SQL. Objeknya: pool, profile, 4 address-list, 4 filter, plus allowlist `zyad.cloud`.

1. Pastikan tidak ada akun yang masih terisolir dan tidak ada tugas isolir aktif. Akun pilot `10mbps@debug`
   harus sudah pulih (`isolation_state = 'normal'`). Kalau masih ada yang terisolir, pulihkan dulu lewat
   Pusat Kendali ("Pulihkan semua").
2. Keluarkan `10mbps@debug` dari daftar pilot (Pusat Kendali), lalu set mode tenant ke `simulation`
   (mode tenant yang ada hanya `simulation|pilot|active|paused`; tidak ada `off`).
   **Jangan** klik "Aktifkan Isolir Otomatis".
3. Halaman router `lab-gns3` → tombol **"Bersihkan"** → baca pratinjau (objek yang akan dihapus hanya yang
   berlabel `zyad-managed`) → konfirmasi.
4. Verifikasi: pratinjau setup berikutnya menampilkan semua objek sebagai "akan dibuat", dan di router
   `/ip firewall filter print where comment~"zyad-managed"` kosong.

Cek SQL read-only sebelum langkah 3:

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);
SELECT s.name, s.isolation_state, s.account_id
FROM mikrotik_ppp_secrets s
JOIN mikrotik_routers r ON r.organization_id = s.organization_id AND r.id = s.router_id
WHERE s.organization_id = :'org' AND r.name = 'lab-gns3' AND s.isolation_state <> 'normal';
SELECT status, count(*) FROM mikrotik_isolation_tasks
WHERE organization_id = :'org' GROUP BY status;
ROLLBACK;
```

Hasil yang diharapkan: query pertama 0 baris, dan tidak ada tugas berstatus antre atau berjalan.

---

## Butir 2: tiga kontak `customer1@yopmail.com` (satu dipertahankan) — butuh persetujuan sebelum dijalankan

Kontaknya: `10mbps@debug` (terpasang ke secret lewat akun receivable), `Customer Uji Isolir`, dan
`Customer Test 1` (lama). Kontak yang **dipertahankan** (`keep`) adalah yang dirujuk akun receivable
dengan invoice/kontrak terbanyak. Biasanya itu kontak yang akunnya terpasang ke secret `10mbps@debug`.
Pemilik produk memastikan pilihan ini dari hasil 2a.

### 2a. Bukti

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);

-- Kontak dan semua rujukannya.
SELECT c.id, c.first_name, c.last_name, c.email, c.phone, c.created_at,
       a.id   AS account_id,
       (SELECT count(*) FROM receivable_invoices i  WHERE i.organization_id = c.organization_id AND i.account_id = a.id)  AS invoices,
       (SELECT count(*) FROM receivable_contracts k WHERE k.organization_id = c.organization_id AND k.account_id = a.id)  AS contracts,
       (SELECT count(*) FROM mikrotik_ppp_secrets s WHERE s.organization_id = c.organization_id AND s.account_id = a.id)  AS secrets,
       (SELECT count(*) FROM crm_deals d          WHERE d.organization_id = c.organization_id AND d.contact_id = c.id AND d.deleted_at IS NULL) AS deals,
       (SELECT count(*) FROM crm_quotations q     WHERE q.organization_id = c.organization_id AND q.contact_id = c.id)  AS quotations,
       (SELECT count(*) FROM crm_sales_orders so  WHERE so.organization_id = c.organization_id AND so.contact_id = c.id) AS sales_orders
FROM crm_contacts c
LEFT JOIN receivable_accounts a
  ON a.organization_id = c.organization_id AND a.source_type = 'crm_contact'
 AND a.source_id = c.id AND a.deleted_at IS NULL
WHERE c.organization_id = :'org' AND lower(btrim(c.email)) = 'customer1@yopmail.com'
  AND c.deleted_at IS NULL
ORDER BY c.created_at;
ROLLBACK;
```

Hasil yang diharapkan: 3 kontak. Satu punya `account_id` dengan `secrets = 1`. Catat `keep` dan dua id `drop`:

```sql
\set keep '<id kontak yang dipertahankan>'
\set drop1 '<id kontak ganda 1>'
\set drop2 '<id kontak ganda 2>'
```

**Berhenti dan minta keputusan** bila lebih dari satu kontak punya akun receivable yang punya invoice atau
kontrak. Akun seperti itu tidak bisa dipindah ke `keep`, karena satu kontak hanya boleh punya satu akun aktif
(indeks unik `source_type, source_id`), dan menggabungkan akun berarti memindah invoice. Itu di luar runbook ini.

### 2b. Pindahkan rujukan ke `keep`, lalu soft-delete kontak ganda

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);

-- Akun receivable milik kontak ganda dipindah ke keep HANYA bila keep belum punya akun aktif.
UPDATE receivable_accounts a
SET source_id = :'keep', updated_at = now()
WHERE a.organization_id = :'org' AND a.source_type = 'crm_contact' AND a.deleted_at IS NULL
  AND a.source_id IN (:'drop1', :'drop2')
  AND NOT EXISTS (SELECT 1 FROM receivable_accounts k
                  WHERE k.organization_id = a.organization_id AND k.source_type = 'crm_contact'
                    AND k.source_id = :'keep' AND k.deleted_at IS NULL);

-- Rujukan CRM yang dipertahankan.
UPDATE crm_deals        SET contact_id = :'keep', updated_at = now()
WHERE organization_id = :'org' AND contact_id IN (:'drop1', :'drop2') AND deleted_at IS NULL;
UPDATE crm_quotations   SET contact_id = :'keep', updated_at = now()
WHERE organization_id = :'org' AND contact_id IN (:'drop1', :'drop2');
UPDATE crm_sales_orders SET contact_id = :'keep', updated_at = now()
WHERE organization_id = :'org' AND contact_id IN (:'drop1', :'drop2');
UPDATE crm_leads        SET converted_contact_id = :'keep', updated_at = now()
WHERE organization_id = :'org' AND converted_contact_id IN (:'drop1', :'drop2');

-- Soft-delete kontak ganda (pola aplikasi; tidak memutus FK).
UPDATE crm_contacts SET deleted_at = now(), updated_at = now()
WHERE organization_id = :'org' AND id IN (:'drop1', :'drop2') AND deleted_at IS NULL;

-- Verifikasi: tinggal satu kontak customer1@yopmail.com yang aktif, dan tidak ada rujukan aktif ke drop.
SELECT id, first_name FROM crm_contacts
WHERE organization_id = :'org' AND lower(btrim(email)) = 'customer1@yopmail.com' AND deleted_at IS NULL;
SELECT 'account' AS ref, count(*) FROM receivable_accounts
WHERE organization_id = :'org' AND source_type = 'crm_contact' AND source_id IN (:'drop1', :'drop2') AND deleted_at IS NULL
UNION ALL SELECT 'deal', count(*) FROM crm_deals
WHERE organization_id = :'org' AND contact_id IN (:'drop1', :'drop2') AND deleted_at IS NULL;

ROLLBACK;  -- ganti COMMIT setelah disetujui
```

Hasil yang diharapkan: `UPDATE crm_contacts 2`, verifikasi pertama 1 baris (`keep`), dan semua hitungan rujukan 0.
Bila `ref = account` tidak 0, artinya `keep` sudah punya akun dan akun kontak ganda tidak dipindah.
`ROLLBACK`, lalu kembali ke catatan "berhenti" di 2a.

---

## Butir 3: lead `10mbps@debug` dan deal `Uji Isolir - Basic Publik 10mbps` — butuh persetujuan sebelum dijalankan

Soft-delete keduanya, sama seperti tombol hapus di UI. Dengan begitu Leads dashboard dan pipeline deal bersih.

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);

-- Bukti (read-only).
SELECT id, contact_name, email, status, converted_deal_id, created_at FROM crm_leads
WHERE organization_id = :'org' AND contact_name = '10mbps@debug' AND deleted_at IS NULL;
SELECT d.id, d.title, d.status, d.contact_id,
       (SELECT count(*) FROM crm_quotations q   WHERE q.organization_id = d.organization_id AND q.deal_id = d.id)   AS quotations,
       (SELECT count(*) FROM crm_sales_orders s WHERE s.organization_id = d.organization_id AND s.deal_id = d.id)   AS sales_orders
FROM crm_deals d
WHERE d.organization_id = :'org' AND d.title = 'Uji Isolir - Basic Publik 10mbps' AND d.deleted_at IS NULL;

ROLLBACK;
```

Hasil yang diharapkan: tepat 1 lead dan 1 deal. Catat id-nya. Perubahan dikunci ke id ini, bukan ke nama, supaya
data lain bernama sama tidak ikut terhapus:

```sql
\set lead_id '<id lead dari SELECT>'
\set deal_id '<id deal dari SELECT>'
```

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);
UPDATE crm_leads SET deleted_at = now(), updated_at = now()
WHERE organization_id = :'org' AND id = :'lead_id' AND deleted_at IS NULL;
UPDATE crm_deals SET deleted_at = now(), updated_at = now()
WHERE organization_id = :'org' AND id = :'deal_id' AND deleted_at IS NULL;
ROLLBACK;  -- ganti COMMIT setelah disetujui
```

Hasil yang diharapkan: `UPDATE 1` dua kali. Kalau deal punya quotation atau SO,
dokumen itu **tidak** ikut dihapus (FK hanya ke deal, jadi soft-delete aman). Laporkan jumlahnya ke
pemilik produk: kontrak dan invoice yang lahir dari SO itu (mis. `INV-2026-0003`) di luar cakupan
spec 3.8 dan butuh keputusan terpisah.

---

## Butir 4: `INV-2026-0004` dan `INV-2026-0005` (catatan "UJI webhook DOKU") — butuh persetujuan sebelum dijalankan

Invoice tidak punya `deleted_at`. Jalur yang dipakai bergantung status, yang dibuktikan di 4a.

### 4a. Bukti

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);
SELECT i.id, i.invoice_number, i.status, i.grand_total, i.amount_paid, i.notes, i.contract_id,
       (SELECT count(*) FROM receivable_payments p           WHERE p.organization_id = i.organization_id AND p.invoice_id = i.id) AS payments,
       (SELECT count(*) FROM receivable_checkouts c          WHERE c.organization_id = i.organization_id AND c.invoice_id = i.id) AS checkouts,
       (SELECT count(*) FROM landing_page_activations l      WHERE l.organization_id = i.organization_id AND l.invoice_id = i.id) AS activations,
       -- settlement_entries tanpa RLS dan tanpa FK ke invoice: filter organisasi manual.
       (SELECT count(*) FROM settlement_entries se           WHERE se.beneficiary_organization_id = i.organization_id AND se.invoice_id = i.id) AS settlement_entries
FROM receivable_invoices i
WHERE i.organization_id = :'org' AND i.invoice_number IN ('INV-2026-0004', 'INV-2026-0005');

SELECT se.id, se.kind, se.amount, se.provider_reference, se.occurred_at, i.invoice_number
FROM settlement_entries se
JOIN receivable_invoices i ON i.organization_id = se.beneficiary_organization_id AND i.id = se.invoice_id
WHERE se.beneficiary_organization_id = :'org' AND i.invoice_number IN ('INV-2026-0004', 'INV-2026-0005');
ROLLBACK;
```

Hasil yang diharapkan: 2 baris, `notes` memuat "UJI webhook DOKU", dan `activations = 0`.
`settlement_entries` menentukan jalur di 4b-B (lihat di bawah).

> Nama kolom dan tabel diambil dari migration (sampai `000183`), bukan dibaca dari DB prod. Dry run (`ROLLBACK`)
> akan langsung gagal bila ada nama yang berbeda.

### 4b-A. Invoice belum dibayar (`payments = 0`): void

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);
UPDATE receivable_invoices
SET status = 'void', voided_at = now(), void_reason = 'Data uji webhook DOKU (cleansing 2026-11)', updated_at = now()
WHERE organization_id = :'org' AND invoice_number IN ('INV-2026-0004', 'INV-2026-0005')
  AND status IN ('draft', 'issued', 'overdue')
  AND NOT EXISTS (SELECT 1 FROM receivable_payments p
                  WHERE p.organization_id = receivable_invoices.organization_id AND p.invoice_id = receivable_invoices.id);
SELECT invoice_number, status FROM receivable_invoices
WHERE organization_id = :'org' AND invoice_number IN ('INV-2026-0004', 'INV-2026-0005');
ROLLBACK;  -- ganti COMMIT setelah disetujui
```

### 4b-B. Invoice sudah dibayar lewat DOKU sandbox (`payments > 0`): hapus permanen

Void tidak cocok untuk invoice lunas, karena pembayarannya tetap tercatat dan laporan jadi tidak konsisten.
Pembayaran sandbox bukan uang nyata, jadi invoice dan pembayarannya dihapus. Item, kiriman, dan checkout
ikut terhapus lewat `ON DELETE CASCADE`.

Lakukan ini **hanya bila `activations = 0` dan `settlement_entries = 0`**.

Bila `settlement_entries > 0`, pembayaran online itu sudah masuk buku settlement. Saldo settlement organisasi
`ethernet` ikut menghitungnya, dan entri itu menunjuk ke invoice lewat `invoice_id` tanpa FK. **Jangan hapus
invoice, pembayaran, maupun `settlement_entries`.** Migration `000165` sengaja membuat catatan uang tidak ikut
terhapus. Berhenti di sini dan minta keputusan pemilik produk. Pilihan yang wajar: biarkan kedua invoice
lunas apa adanya (tidak menghalangi uji ulang), atau koreksi saldo dengan entri `adjustment` negatif lewat UI
settlement super admin, dengan catatan "Pembatalan pembayaran sandbox UJI webhook DOKU".

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);
SELECT p.id, p.method, p.reference, p.amount, p.paid_at FROM receivable_payments p
JOIN receivable_invoices i ON i.organization_id = p.organization_id AND i.id = p.invoice_id
WHERE i.organization_id = :'org' AND i.invoice_number IN ('INV-2026-0004', 'INV-2026-0005');

DELETE FROM receivable_payments p USING receivable_invoices i
WHERE i.organization_id = :'org' AND p.organization_id = i.organization_id AND p.invoice_id = i.id
  AND i.invoice_number IN ('INV-2026-0004', 'INV-2026-0005')
  -- Penjaga: tidak menghapus apa pun bila ada entri settlement (pernyataan berikut juga dijaga).
  AND NOT EXISTS (SELECT 1 FROM settlement_entries se
                  WHERE se.beneficiary_organization_id = i.organization_id AND se.invoice_id = i.id);
DELETE FROM receivable_invoices i
WHERE i.organization_id = :'org' AND i.invoice_number IN ('INV-2026-0004', 'INV-2026-0005')
  AND NOT EXISTS (SELECT 1 FROM settlement_entries se
                  WHERE se.beneficiary_organization_id = i.organization_id AND se.invoice_id = i.id);

SELECT count(*) FROM receivable_invoices
WHERE organization_id = :'org' AND invoice_number IN ('INV-2026-0004', 'INV-2026-0005');
ROLLBACK;  -- ganti COMMIT setelah disetujui
```

Hasil yang diharapkan: jumlah `DELETE` pembayaran sama dengan hasil SELECT, `DELETE 2` untuk invoice,
dan verifikasi 0. Nomor `INV-2026-0004/0005` **tidak** dipakai ulang, karena counter dokumen tidak mundur.
Ini disengaja.

---

## Butir 5: router `lab-gns3` beserta 85 secret — butuh persetujuan sebelum dijalankan

Jalankan setelah butir 1–4.

1. **UI dulu:** hapus router `lab-gns3` dari halaman router. Ini soft-delete, dan ditolak bila masih ada
   secret terisolir atau tugas isolir aktif (penjaga `SoftDelete`). Nama router jadi bebas dipakai lagi
   untuk uji ulang.
2. **SQL:** soft-delete menyisakan 85 baris secret beserta pemasangannya ke akun. Hapus permanen router
   yang sudah soft-deleted. Secret, saran pemasangan, tugas isolir, dan catatan onboarding secret ikut
   terhapus lewat `ON DELETE CASCADE`.

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);

SELECT r.id, r.name, r.deleted_at,
       (SELECT count(*) FROM mikrotik_ppp_secrets s       WHERE s.organization_id = r.organization_id AND s.router_id = r.id)                         AS secrets,
       (SELECT count(*) FROM mikrotik_ppp_secrets s       WHERE s.organization_id = r.organization_id AND s.router_id = r.id AND s.account_id IS NOT NULL) AS paired,
       (SELECT count(*) FROM mikrotik_ppp_secrets s       WHERE s.organization_id = r.organization_id AND s.router_id = r.id AND s.isolation_state <> 'normal') AS isolated
FROM mikrotik_routers r
WHERE r.organization_id = :'org' AND r.name = 'lab-gns3'
ORDER BY r.deleted_at NULLS FIRST;
ROLLBACK;
```

Catat id router yang baru di-soft-delete lewat UI (baris dengan `deleted_at` hari ini). Bila ada router
`lab-gns3` lain yang sudah lama di-soft-delete dari uji sebelumnya, **jangan** ikut dihapus tanpa persetujuan
tersendiri.

```sql
\set router_id '<id router lab-gns3 dari SELECT>'
```

```sql
BEGIN;
SET LOCAL TimeZone = 'UTC';
SELECT set_config('app.organization_id', :'org', true);

DELETE FROM mikrotik_routers
WHERE organization_id = :'org' AND id = :'router_id' AND deleted_at IS NOT NULL;

SELECT count(*) AS sisa_secret FROM mikrotik_ppp_secrets s
WHERE s.organization_id = :'org'
  AND NOT EXISTS (SELECT 1 FROM mikrotik_routers r WHERE r.organization_id = s.organization_id AND r.id = s.router_id);

ROLLBACK;  -- ganti COMMIT setelah disetujui
```

Hasil yang diharapkan: SELECT menampilkan router dengan `deleted_at` terisi, `secrets = 85`, `paired = 1`,
`isolated = 0`. Lalu `DELETE 1` dan `sisa_secret = 0`. Bila `deleted_at` masih kosong, langkah UI belum dijalankan dan
`DELETE 0`. Jangan menghapus router yang belum di-soft-delete.

Akun receivable milik kontak `keep` tetap ada. Isinya bisa dipakai lagi saat uji ulang onboarding,
atau di-onboard ulang dari secret hasil impor baru.

---

## Setelah selesai

- Catat waktu dan hasil tiap butir (jumlah baris, COMMIT/ROLLBACK) di
  `docs/superpowers/notes/2026-11-11-mikrotik-uji-prod-temuan-bug.md`, bagian "Sisa data uji".
- Uji ulang mengikuti kriteria selesai spec Gelombang 2 bagian 6: tambah router lab lagi, impor secret,
  onboarding satu secret dari halaman detail.
- Rollback: hanya lewat restore dump prasyarat (`pg_restore` ke DB terpisah lalu salin baris yang perlu).
  Soft-delete di butir 2 dan 3 bisa dibatalkan dengan `UPDATE … SET deleted_at = NULL` untuk id yang tercatat.
