# Runbook: notifikasi pembayaran DOKU dan konfirmasi pembayaran pelanggan terisolir

Tujuan: setiap pembayaran DOKU untuk invoice receivable (`RCV-<invoice_id>`) tercatat sebagai `paid`, dan
pelanggan Mikrotik yang terisolir dipulihkan otomatis tanpa harus membuka halaman apa pun.

## 1. Tiga jalur konfirmasi

| Jalur | Pemicu | Kode |
|---|---|---|
| Webhook DOKU | DOKU memanggil `POST /api/v1/webhooks/doku` | `OnlinePayment.HandleNotification` |
| Pelanggan kembali ke halaman bayar | `GET /api/v1/public/invoices/<token>/status` (dipicu `?paid=1`) | `OnlinePayment.SyncStatus` |
| Polling worker untuk pelanggan terisolir | runner isolir tiap 2 menit | `OnlinePayment.SyncInvoice` via `MikrotikIsolationRunner` |

Ketiganya idempoten: pembayaran dicatat lewat `HandleNotification` yang sama, checkout hanya diproses selagi
berstatus `pending`, dan invoice yang sudah `paid` tidak menerima pembayaran kedua.

- `?paid=1` **hanya** memicu polling halaman; tidak pernah dianggap bukti lunas. Untuk VA, pelanggan membayar
  di aplikasi bank dan tidak kembali ke halaman, jadi `?paid=1` tidak berlaku.
- Polling worker hanya menyentuh invoice pemicu isolir milik akun yang sedang terisolir, melewati invoice tanpa
  checkout `pending` atau yang checkout-nya kedaluwarsa, dan dibatasi minimal 10 detik per invoice.
- Polling worker **butuh kredensial DOKU** (`DOKU_CLIENT_ID`, `DOKU_SECRET_KEY`/`DOKU_ACTIVE_SECRET`) di env
  proses `zyad-worker`. Tanpa kredensial, polling dimatikan (log: `DOKU credentials missing, isolated customers
  rely on the webhook only`) karena klien Noop selalu menjawab SUCCESS.

## 2. Konfigurasi DOKU Back Office (wajib per lingkungan)

1. Masuk DOKU Back Office (sandbox atau produksi).
2. *Settings > Payment Settings*.
3. Untuk **setiap channel/VA yang dipakai**, isi **Payment Notification URL**:
   `https://api.zyad.cloud/api/v1/webhooks/doku` (produksi; ganti host untuk lingkungan lain).
4. Bila memakai URL override, path-nya harus sama persis dengan path terkonfigurasi. Aplikasi mengirim
   `override_notification_url = APP_URL + /api/v1/webhooks/doku`.

Penyebab webhook tidak pernah datang pada uji 11 Nov 2026: Payment Notification URL di Back Office kosong.
Setelah diisi, webhook terbukti masuk (19:21:57, signature lolos, status `paid`).

## 3. Fakta operasional

- Retry DOKU bila webhook gagal: 30 menit, 6 jam, 12 jam. Karena jedanya panjang, polling worker tetap perlu.
- IP pengirim webhook yang teramati: `147.139.130.145` (sandbox). Jangan dijadikan satu-satunya filter;
  verifikasi tetap lewat signature.
- Cek webhook masuk: cari `POST /api/v1/webhooks/doku` di log `zyad-api`.

## 4. Uji di sandbox

- Simulator: `https://sandbox.doku.com/gtw-config-v2/simulator` (tanpa login). VA "DOKU VA (Other Banks)" =
  "ALTO, ATM Bersama, Prima (DOKU VA)" di bagian Non SNAP. URL lama `.../integration/simulator/` kosong.
- Kartu uji (Visa 4811 1111 1111 1114, Mastercard 5211 1111 1111 1117) ditolak merchant sandbox ("Your card is
  not allowed"). Aktifkan channel Cards di Back Office, atau uji lewat VA.
- Uji polling tanpa webhook: kosongkan sementara Payment Notification URL channel yang dipakai, bayar invoice
  uji lewat simulator, lalu pastikan dalam beberapa menit invoice `paid` dan pelanggan pulih tanpa membuka
  halaman. **Kembalikan URL notifikasi setelah uji.**

## 5. Troubleshooting

| Gejala | Periksa |
|---|---|
| Invoice tetap `issued` setelah bayar | log webhook; Payment Notification URL per channel; kredensial DOKU di worker; checkout masih `pending`? |
| Log `pembayaran DOKU untuk invoice non-payable, perlu refund manual` | pembayaran ganda/terlambat untuk invoice yang sudah lunas; tangani refund manual |
| Log `payment status sync failed` berulang | koneksi worker ke `api(-sandbox).doku.com`, kredensial, status DOKU |
