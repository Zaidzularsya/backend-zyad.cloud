# Penanganan Timezone & Timestamp

## Aturan

1. **Semua `timestamp without time zone` menyimpan wall-clock UTC.**
   `database.Connect` mengunci sesi koneksi ke `timezone=UTC`, sehingga
   `now()` / `DEFAULT now()` / `SET x = NOW()` menyimpan UTC, sama seperti nilai
   yang ditulis Go lewat `time.Now().UTC()`. Timezone server DB (`Asia/Jakarta`)
   tidak lagi berpengaruh.
2. **Nilai dari Go harus `.UTC()` sebelum masuk DB.** pgx membuang zona
   `time.Time` non-UTC dan menyimpan wall-clock aslinya (mis. `time.Now()` di
   server WIB tersimpan sebagai jam WIB). Zona IMAP `Date` header juga harus
   di-`UTC()`.
3. **Batas "hari" dihitung di Go, bukan di SQL.** Zona bisnis = `Asia/Jakarta`
   (`internal/core/businesstime`). Jangan pakai `CURRENT_DATE`, `date_trunc`
   pada kolom mentah, atau `::date` yang bergantung pada timezone sesi.
   - `businesstime.DayStartUTC(date)` → instant UTC awal hari WIB (untuk filter `>=`/`<`).
   - `businesstime.DayOf(t)` → tanggal WIB dari sebuah instant (dedup harian,
     mis. `wa_conversations.crm_activity_on`).
   - Bucket harian dashboard: `date_trunc(unit, kolom + make_interval(secs => offset))`
     dengan offset dari `businesstime.Location()`.
4. API mengirim timestamp sebagai RFC3339 `Z` (UTC); frontend yang menampilkan
   di zona pengguna.

## Riwayat insiden (29 Sep 2026)

Sesi koneksi mewarisi `TimeZone` server (`Asia/Jakarta`), jadi kolom berisi
`now()` menyimpan jam WIB yang dibaca sebagai UTC (+7 jam, tampil "di masa
depan"), sementara kolom yang ditulis Go (`wa_messages.sent_at`,
`crm_activities.due_at`) sudah UTC. Bukti: `wa_messages.created_at - sent_at`
persis `07:00:01`.

Efek samping yang ikut benar setelah fix: pengecekan seperti
`sessions.expires_at > now()` (nilai UTC dari Go vs `now()` WIB) sebelumnya
7 jam terlalu longgar.

## Koreksi data lama

`scripts/db/fix_timestamp_utc_shift.sql` — menggeser −7 jam hanya kolom yang
terbukti diisi `now()` DB. **Tidak** di `migrations/` (tidak jalan otomatis).
Dry-run adalah default dan di-rollback. Jalankan sekali, saat `zyad-api` dan
`zyad-worker` berhenti, sebelum start release yang mengunci sesi ke UTC (kalau
dijalankan setelahnya, baris baru yang sudah UTC ikut tergeser salah).
Kolom bercampur (mis. `wa_webhook_events.next_retry_at`, `mail_messages.sent_at`)
sengaja tidak disentuh.

## Tech debt

- Kolom masih `timestamp without time zone` (±294 kolom, 0 `timestamptz`).
  Migrasi ke `timestamptz` **belum dikerjakan**: perlu rencana terpisah
  (tipe kolom + view/index/partial index yang bergantung + konvensi `AT TIME ZONE`).
  Setelah itu, aturan 1–2 di atas tidak lagi diperlukan.
- Layanan non-CRM dengan `now: time.Now` masih bergantung pada disiplin
  `.UTC()` di call site; pertimbangkan codec pgx yang menormalkan ke UTC saat
  encode jika kebocoran muncul lagi.
