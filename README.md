# Multi-Tenant IT Solution Platform (Golang-Based)

Platform multi-tenant berbasis **Golang** yang dirancang khusus untuk menyediakan solusi IT terintegrasi (*SaaS & Custom IT Solutions*) bagi berbagai tenant. Platform ini memungkinkan penyediaan produk instan seperti Landing Page, Company Profile, Membership, hingga Point of Sales (POS) dalam satu ekosistem yang terisolasi dan aman.

---

## 🚀 Fitur Utama & Modul Inti

### 1. Multi-Tenancy Architecture
Mendukung isolasi data tingkat tinggi untuk setiap tenant dengan pendekatan fleksibel:
*   **Strategi Isolasi Data:**
    *   *Shared Database, Shared Schema (Row-level security via Tenant ID)*: Cocok untuk tenant skala kecil-menengah untuk menghemat resource.
    *   *Database-per-Tenant (Isolated)*: Mendukung koneksi database terpisah untuk tenant skala Enterprise yang membutuhkan kepatuhan keamanan data ketat.
*   **Tenant Resolution Middleware:**
    *   Mengidentifikasi tenant berdasarkan **Subdomain** (`tenant1.platform.com`), **Custom Domain** (`www.tenantcustom.com`), atau **HTTP Header** (`X-Tenant-ID`).
    *   Menginjeksikan koneksi database tenant ke dalam context request Golang (`context.Context`).

### 2. Flexible Auth (Social Media Integration)
Sistem autentikasi yang fleksibel dan aman yang mendukung autentikasi tradisional serta OAuth2:
*   **Metode Auth:**
    *   Username & Password (terenkripsi menggunakan `bcrypt`).
    *   Social Login OAuth2 (Google, Facebook, GitHub) menggunakan pustaka Golang seperti **Goth** atau custom handler.
*   **JWT & Session Management:**
    *   Menggunakan JWT (JSON Web Tokens) stateless dengan rotasi Refresh Token yang disimpan di Redis untuk logout instan dan keamanan maksimal.
    *   Mendukung Single Sign-On (SSO) lintas domain/subdomain tenant jika dikonfigurasi.

### 3. CRM (Customer Relationship Management) Module
Modul terpusat untuk membantu tenant mengelola interaksi pelanggan mereka:
*   **Lead & Contact Management:** Melacak data prospek dan pelanggan.
*   **Sales Pipeline / Kanban Board:** Visualisasi proses penjualan dari prospek hingga deal.
*   **Interaction Logs:** Pencatatan otomatis riwayat komunikasi (email, chat, POS transaksi).
*   **Customer Segmentation & Analytics:** Pengelompokan pelanggan berdasarkan aktivitas pembelian dan loyalitas.

### 4. Permission Management (RBAC/ABAC)
Kontrol akses granular untuk memastikan keamanan data internal tenant:
*   **Role-Based Access Control (RBAC):** Definisi role default (Super Admin, Tenant Admin, Manager, Cashier, Member).
*   **Attribute-Based Access Control (ABAC):** Pembatasan akses berdasarkan kondisi tertentu (misal: jam kerja, IP address, atau kepemilikan data).
*   **Enforcer Engine:** Integrasi dengan **Casbin** (go-casbin) untuk manajemen policy permission yang dinamis tanpa perlu deploy ulang kode.

---

## 📦 Custom Product IT Solution (Tenant Modules)

Setiap tenant dapat mengaktifkan atau menonaktifkan modul produk berikut secara modular:

1.  **Landing Page Generator**
    *   CRM dinamis untuk membuat landing page promo dengan template editor.
    *   Optimasi SEO bawaan (sitemap generator, meta tag editor, schema markup).
    *   Integrasi lead capture form langsung ke CRM.
2.  **Company Profile Page**
    *   Manajemen portofolio, layanan, blog/artikel, dan tim.
    *   Halaman statis & dinamis dengan performa tinggi (mendukung static site generation/caching).
3.  **Module Membership (Loyalty Program)**
    *   Pendaftaran member/pelanggan tenant dengan tiering system (Silver, Gold, Platinum).
    *   Sistem poin reward berdasarkan transaksi POS atau pembelian online.
    *   Kupon promo dan manajemen voucher digital.
4.  **Module POS (Point of Sales)**
    *   Antarmuka kasir cepat (optimized for tablet/desktop web).
    *   Manajemen Inventori (stok real-time, alert stok menipis, mutasi stok).
    *   Dukungan multi-outlet / multi-cabang per tenant.
    *   Integrasi dengan Membership untuk klaim poin/diskon saat checkout.

---

## 🏗️ Rancangan Arsitektur Folder (Clean/Hexagonal Architecture)

Kami merekomendasikan struktur folder standar industri Golang (`golang-standards/project-layout`) dengan pendekatan **Clean Architecture** untuk memisahkan logika bisnis dari library eksternal/framework:


```text
backend-go/
├── api/                         # Spesifikasi API seperti OpenAPI/Swagger
│   └── openapi.yaml
│
├── cmd/                         # Entry point aplikasi
│   └── api/
│       └── main.go              # File utama untuk menjalankan HTTP API server
│
├── internal/                    # Kode privat aplikasi yang tidak bisa di-import project lain
│   ├── app/                     # Bootstrap aplikasi, dependency injection, router, dan server
│   │   ├── app.go               # Struktur utama aplikasi
│   │   ├── dependency.go        # Wiring dependency: config, database, service, handler
│   │   ├── router.go            # Registrasi route dan middleware global
│   │   └── server.go            # Konfigurasi dan lifecycle HTTP server
│   │
│   ├── config/                  # Load dan mapping konfigurasi aplikasi
│   │   ├── config.go            # Root config yang menggabungkan semua konfigurasi
│   │   ├── app.go               # Konfigurasi aplikasi: env, port, debug, app URL
│   │   ├── auth.go              # Konfigurasi auth: JWT, token TTL, password hash
│   │   ├── database.go          # Konfigurasi koneksi database
│   │   ├── redis.go             # Konfigurasi Redis
│   │   ├── mikrotik.go          # Konfigurasi integrasi MikroTik
│   │   ├── xendit.go            # Konfigurasi payment gateway Xendit
│   │   └── whatsapp.go          # Konfigurasi WhatsApp gateway
│   │
│   ├── platform/                # Adapter/integrasi teknis ke infrastruktur eksternal
│   │   ├── database/            # Koneksi database, pool, transaction helper
│   │   ├── logger/              # Logger aplikasi
│   │   ├── mail/                # Email sender dan template email
│   │   ├── mikrotik/            # Client API MikroTik
│   │   ├── redis/               # Redis client/helper
│   │   ├── storage/             # File storage/local/S3-compatible storage
│   │   ├── whatsapp/            # Client WhatsApp gateway
│   │   └── xendit/              # Client Xendit/payment gateway
│   │
│   ├── core/                    # Fondasi teknis internal yang dipakai lintas module
│   │   ├── auth/                # JWT, password hashing, current user, auth helper
│   │   ├── crypto/              # Helper kriptografi/token/random string
│   │   ├── errors/              # Error standar aplikasi dan error mapping
│   │   ├── http/                # Helper HTTP request/response/context
│   │   ├── idempotency/         # Pencegah duplicate request untuk order/payment/webhook
│   │   ├── middleware/          # Middleware global: auth, logging, recovery, CORS, rate limit
│   │   ├── permission/          # RBAC/permission checker, policy, middleware permission
│   │   └── validation/          # Helper validasi request dan format error validasi
│   │
│   ├── modules/                 # Module fitur/domain bisnis aplikasi
│   │   ├── account/             # Akun, profil, credential, preferensi akun
│   │   ├── asset/               # Aset fisik/digital seperti router, ODP, perangkat
│   │   ├── billing/             # Tagihan, invoice, siklus billing, status pembayaran
│   │   ├── contract/            # Kontrak pelanggan dan dokumen kontrak
│   │   ├── dashboard/           # Statistik, grafik, dan ringkasan dashboard
│   │   ├── landingpage/         # Konten landing page, form lead, banner, halaman publik
│   │   ├── mikrotik/            # Data router MikroTik dari sisi aplikasi
│   │   ├── newsaggregator/      # Agregasi berita/konten eksternal
│   │   ├── order/               # Order lifecycle, order item, approval, cancellation
│   │   ├── organization/        # Tenant, perusahaan, cabang, struktur organisasi
│   │   ├── payment/             # Payment, webhook, reconciliation, payment gateway
│   │   ├── product/             # Produk, paket internet, harga, benefit
│   │   ├── provisioning/        # Aktivasi/deaktivasi layanan ke sistem teknis
│   │   ├── radius/              # RADIUS user, profile, session, accounting, AAA
│   │   ├── resource/            # Resource jaringan seperti IP pool, VLAN, bandwidth profile
│   │   └── user/                # User aplikasi, role assignment, status user, akses admin
│   │
│   └── shared/                  # Helper umum yang aman dipakai lintas module
│       ├── pagination/          # Helper pagination, limit, offset, metadata page
│       ├── response/            # Format response sukses/error yang konsisten
│       └── utils/               # Utility umum kecil seperti string, time, slug, phone helper
│
├── docs/                        # Dokumentasi requirement, PRD, SRS, SDD, TBD, dan development note
├── scripts/                     # Script pendukung development, deployment, migration, seed
├── tests/                       # Test tambahan/integration/e2e jika diperlukan
├── .env.example                 # Contoh environment variable
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksum
├── AGENTS.md                    # Instruksi kerja untuk AI coding agent seperti Codex
└── README.md                    # Dokumentasi utama project untuk developer
```

---

## 🧪 Development Commands

### Unit Test

Jalankan test biasa tanpa kebutuhan database:

```bash
go test ./...
```

### Integration Test Database

Integration test database bersifat opt-in dan hanya berjalan dengan build tag `integration`. Pastikan `.env` berisi config test database terpisah, bukan database development/production:

```env
TEST_DB_TYPE=postgres
TEST_DB_HOST=localhost
TEST_DB_PORT=5432
TEST_DB_USERNAME=postgres
TEST_DB_PASSWORD=postgres
TEST_DB_NAME=zyad_cloud_test
TEST_DB_SCHEMA=public
TEST_DB_SSL=disable
TEST_DB_CONNECT_TIMEOUT_SECONDS=5
```

Database test wajib mengandung kata `test` pada `TEST_DB_NAME`. Helper test akan menjalankan migration dan melakukan cleanup table notification dengan `TRUNCATE ... CASCADE`.

Jalankan semua integration test notification repository:

```bash
go test -tags=integration ./internal/core/notification/repository
```

Untuk compile-check integration test tanpa menjalankan test yang menyentuh database:

```bash
go test -tags=integration -run '^$' ./internal/core/notification/repository
```

### Notification Worker

Jalankan satu batch outbox dan retry notification:

```bash
go run ./cmd/worker -once
```

Jalankan worker loop:

```bash
go run ./cmd/worker
```

Worker memakai env `NOTIFICATION_WORKER_INTERVAL_SECONDS` dan `NOTIFICATION_WORKER_BATCH_SIZE`. Default worker hanya mendaftarkan email dispatcher; WhatsApp tidak diproses worker sampai provider WhatsApp tersedia. Email akan memakai SMTP saat `MAIL_HOST` terisi, dan otomatis fallback ke `noop` saat `MAIL_HOST` kosong untuk local test.

---

## 💡 Ide Tambahan & Peningkatan (Value-Added Suggestions)

Berikut adalah beberapa rekomendasi fitur dan teknologi tambahan untuk meningkatkan nilai jual platform Anda:

1.  **Dynamic Custom Domain & Automatic SSL Provisioning**
    *   *Ide:* Izinkan tenant menggunakan domain mereka sendiri (misalnya, `toko-budi.com` alih-alih `budi.platform.com`).
    *   *Solusi:* Integrasikan **Traefik** atau **Caddy Server** dengan API Let's Encrypt untuk secara otomatis menerbitkan dan memperbarui sertifikat SSL ketika tenant menambahkan custom domain mereka.
2.  **Event-Driven Architecture (EDA) dengan Message Broker**
    *   *Ide:* Sinkronisasi data real-time antar modul (contoh: ketika transaksi POS selesai -> tambahkan poin ke Membership -> perbarui data transaksi di CRM).
    *   *Solusi:* Gunakan **RabbitMQ** atau **Apache Kafka** untuk memproses event secara asinkron agar tidak membebani performa request utama HTTP POS.
3.  **Payment Gateway Integration**
    *   *Ide:* Pembayaran online terintegrasi untuk POS, tagihan keanggotaan (Membership), atau checkout Landing Page.
    *   *Solusi:* Sediakan modul payment gateway lokal seperti **Midtrans** atau **Xendit** untuk Indonesia, atau **Stripe** untuk pasar global.
4.  **Tenant Feature Flagging & Billing Control**
    *   *Ide:* Mengontrol akses modul produk berdasarkan paket langganan tenant (misal: Paket Basic hanya dapat Landing Page, Paket Pro mendapat POS & CRM).
    *   *Solusi:* Implementasikan middleware Feature Flagging di level router API Golang, terintegrasi dengan modul langganan/billing tenant.
5.  **Multi-Language / Localization (i18n)**
    *   *Ide:* Aplikasi POS atau Company Profile yang dapat diubah bahasanya sesuai target pasar tenant.
    *   *Solusi:* Integrasikan pustaka i18n Golang (`nicksnyder/go-i18n`) dengan database tenant untuk menyimpan terjemahan dinamis menu atau produk.

## 📄 Lisensi
Hak Cipta © 2026. Seluruh hak cipta dilindungi undang-undang.
