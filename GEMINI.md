# GEMINI.md - Panduan AI Agent Gemini untuk Zyad Cloud Backend Go

Dokumen ini berfungsi sebagai acuan kontekstual dan instruksi kerja bagi AI Coding Agent (seperti Gemini) saat berkontribusi pada repositori `zyad.cloud`. Semua agen yang bekerja di sini wajib mematuhi aturan dan pola yang dijelaskan di bawah ini.

---

## 1. Aturan Kerja Utama (Core Rules)

*   **Bahasa Penjelasan**: Seluruh penjelasan hasil kerja kepada pengguna wajib ditulis dalam **Bahasa Indonesia** yang sopan, terstruktur, dan mudah dipahami.
*   **Bahasa Kode & Komunikasi Teknis**: Gunakan **Bahasa Inggris** untuk penulisan kode program (variabel, fungsi, struct, dll), perintah terminal (commands), nama berkas (filenames), nama branch Git, dan pesan commit (commit messages).
*   **Perubahan Bertahap**: Lakukan modifikasi kode dalam skala kecil dan bertahap (incremental). Hindari membuat perubahan masif dalam satu langkah tanpa melakukan verifikasi terlebih dahulu.
*   **Manajemen Dependensi**: Dilarang menambahkan *production dependency* baru ke dalam proyek tanpa persetujuan atau konfirmasi eksplisit dari pengguna.
*   **Arsitektur Utama**: Dilarang mengubah arsitektur utama aplikasi tanpa konfirmasi dan diskusi mendalam dengan pengguna.

---

## 2. Struktur Proyek & Pola Pengembangan (Development Patterns)

Repositori ini menggunakan pendekatan **Clean/Hexagonal Architecture** untuk memisahkan domain logika bisnis dari adapter eksternal dan framework:

```bash
backend-go/
├── cmd/
│   └── api/
│       └── main.go
│
├── api/
│   └── openapi.yaml
│
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   ├── dependency.go
│   │   ├── router.go
│   │   └── server.go
│   │
│   ├── config/
│   │   ├── config.go
│   │   ├── app.go
│   │   ├── auth.go
│   │   ├── database.go
│   │   ├── redis.go
│   │   ├── mikrotik.go
│   │   ├── xendit.go
│   │   └── whatsapp.go
│   │
│   ├── platform/
│   │   ├── database/
│   │   ├── logger/
│   │   ├── mail/
│   │   ├── mikrotik/
│   │   ├── redis/
│   │   ├── storage/
│   │   ├── whatsapp/
│   │   └── xendit/
│   │
│   ├── core/
│   │   ├── auth/
│   │   ├── crypto/
│   │   ├── errors/
│   │   ├── http/
│   │   ├── idempotency/
│   │   ├── middleware/
│   │   ├── permission/
│   │   └── validation/
│   │
│   ├── modules/
│   │   ├── account/
│   │   ├── asset/
│   │   ├── billing/
│   │   ├── contract/
│   │   ├── dashboard/
│   │   ├── landingpage/
│   │   ├── mikrotik/
│   │   ├── newsaggregator/
│   │   ├── order/
│   │   ├── organization/
│   │   ├── payment/
│   │   ├── product/
│   │   ├── provisioning/
│   │   ├── radius/
│   │   ├── resource/
│   │   └── user/
│   │
│   └── shared/
│       ├── pagination/
│       ├── response/
│       └── utils/
├── docs/
├── scripts/
├── tests/
├── .env.example
├── go.mod
├── go.sum
├── AGENTS.md
└── README.md
```

### Pola Layer Modul (`internal/modules/<module_name>`)
Setiap modul fitur wajib mengikuti struktur layer standar berikut:
1.  **`handler/`**: Menangani HTTP request binding, validasi DTO awal, ekstraksi auth context, dan penulisan HTTP response.
2.  **`service/`**: Menyimpan business logic, transaction boundaries (DB transaction), alur idempotency, dan koordinasi/integrasi dengan modul lain.
3.  **`repository/`**: Khusus berinteraksi dengan basis data (PostgreSQL/Redis), tidak boleh berisi business rules.
4.  **`dto/`**: Struktur data pembungkus request (`create_x_request.go`) dan response (`x_response.go`).
5.  **`model/`**: Definisi skema tabel database dan entitas modul.
6.  **`routes.go` & `errors.go`**: Definisi endpoint HTTP dan error spesifik modul.

---

## 3. Aturan Database & Migrasi

*   **Schema Database**: Skema default yang digunakan adalah `public`.
*   **Perubahan Skema**: Semua modifikasi tabel, kolom, index, atau data seeder awal harus dilakukan melalui berkas migrasi terpisah yang dicatat di folder `migrations/`.
*   **Dilarang Auto-Sync**: Jangan sekali-kali mengaktifkan fitur sinkronisasi skema otomatis (seperti `AutoMigrate` pada ORM atau auto-sync pada TypeORM). Semua perubahan skema wajib dijalankan dan diverifikasi melalui CLI migrasi.
*   **Traceability**: Setiap cleanup atau perubahan skema database harus dicatat agar riwayat perkembangan data dapat ditelusuri.

---

## 4. Panduan Khusus untuk Modul Permission (`internal/core/permission`)

Saat melakukan perubahan di `internal/core/permission`, patuhi hal-macam berikut:
*   **Scope Modul**: Jaga agar modul ini tetap bertindak sebagai *shared core permission* murni. Jangan mencampurnya dengan logika kepemilikan bisnis CRM (seperti kepemilikan data prospek/lead atau struktur hierarki tim penjualan).
*   **Model Hak Akses**: Saat ini menggunakan model **Role-Based Access Control (RBAC)** sederhana (`user -> role -> permission`). Struktur skema database dirancang untuk mendukung **Attribute-Based Access Control (ABAC)** melalui scope dan condition, namun evaluasi logic di service untuk saat ini masih bertumpu pada pencocokan *permission name*.
*   **Aturan Penamaan Permission**: Gunakan format `resource.action` untuk setiap permission baru (contoh: `lead.read`, `customer.create`).
*   **Pengujian (Testing)**:
    *   Setiap kali memodifikasi kode permission, pastikan semua test case lulus dengan menjalankan:
        ```bash
        go test ./...
        ```
    *   Untuk memvalidasi migrasi database terkait permission, gunakan perintah:
        ```bash
        go run ./cmd/migrate -direction up
        ```

---

## 5. Alur Kerja Rekomendasi untuk Gemini

1.  **Analisis Awal**: Selalu baca file `README.md` di root dan dokumentasi di `docs/` sebelum memulai pengerjaan fitur baru.
2.  **Verifikasi Integrasi**: Manfaatkan helper yang ada di `internal/core/` (misalnya `errors.AppError` untuk error formatting, atau `corehttp` untuk respon Gin yang seragam).
3.  **Pertahankan Komentar**: Jangan menghapus komentar kode dan docstring yang sudah ada di codebase kecuali diminta atau jika kode terkait dihapus/diubah secara fungsional.
