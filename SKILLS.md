# SKILLS.md - Peta Kapabilitas dan Matriks Hak Akses Platform Zyad Cloud

Dokumen ini mendokumentasikan kapabilitas teknis (skills), modul aplikasi, integrasi eksternal, dan struktur kontrol akses (permission matrix) yang didukung oleh platform Multi-Tenant Zyad Cloud.

> ⚠️ **Disclaimer status implementasi**: dokumen ini menjelaskan *arah kapabilitas platform* — bukan
> semuanya sudah berjalan di kode saat ini. Setiap item ditandai statusnya:
> ✅ **LIVE** (sudah berjalan), 🟡 **PARTIAL** (adapter/skeleton ada, belum lengkap end-to-end), atau
> 🗺️ **ROADMAP** (belum diimplementasikan, arah bisnis saja). Jangan jadikan bagian tanpa disclaimer
> eksplisit sebagai representasi kode aktual — cek [README.md](README.md) dan
> [docs/module-map.md](docs/module-map.md) untuk status implementasi yang diverifikasi terhadap kode.

---

## 1. Modul Aplikasi & Fitur Inti (Core & Tenant Modules)

Setiap tenant pada platform Zyad Cloud dapat mengaktifkan atau menonaktifkan modul produk berikut secara dinamis sesuai kebutuhan bisnis mereka:

### A. Arsitektur Multi-Tenancy (Multi-Tenant Core) — ✅ LIVE
*   **Data Isolation**: Isolasi data antar tenant lewat kolom `organization_id` di setiap tabel domain bisnis, ditambah Row-Level Security PostgreSQL **khusus untuk tabel `landing_*`** (tabel lain mengandalkan filter manual di repository — lihat `docs/database-context.md`).
*   **Database-per-Tenant**: 🗺️ ROADMAP — kolom `data_placement` di tabel `organizations` menyiapkan skema untuk ini, tapi belum diimplementasikan di level koneksi database. Saat ini platform memakai shared database untuk semua tenant.
*   **Tenant Resolution**: ✅ LIVE — lewat subdomain, custom domain, atau session login. (Bukan lewat header `X-Tenant-ID` seperti disebutkan sebelumnya — resolusi tenant tidak memakai header itu di implementasi saat ini.)

### B. CRM (Customer Relationship Management) — 🟡 PARTIAL (backend LIVE untuk 10 resource, frontend baru Fase 1 — lihat `docs/reference-crm.md` dan `docs/module-map.md`)
Semua endpoint backend DAN frontend untuk 10 resource (Lead/Contact/Company/Pipeline/Deal/Activity/Quotation/Invoice CRM/Integration) sudah live di `/api/v1/app/crm/*` dan menu "CRM" tenant, khusus organization tipe customer. Item di bawah masih arah bisnis untuk sub-fitur yang belum digarap (import/export/merge, field-level price/amount masking, entity picker):
*   **Lead & Contact Management**: Melacak prospek potensial dan mengelola database kontak pelanggan secara terpusat.
*   **Sales Pipeline**: Visualisasi tahapan penjualan (sales stages) menggunakan Kanban Board dinamis.
*   **Interaction Logs**: Pencatatan riwayat interaksi pelanggan (chat, email, transaksi kasir) secara otomatis.
*   **Customer Segmentation**: Pengelompokan pelanggan berdasarkan loyalitas dan aktivitas belanja.

### C. Landing Page & Company Profile Generator
*   **Landing Page Builder** — ✅ LIVE, modul paling matang di platform (`internal/modules/landing`): CMS section-based, publish/schedule, revision, branding, domain binding, form/lead capture, footer & reusable navigation, analitik dasar.
*   **Optimasi SEO Bawaan** — 🟡 PARTIAL: meta tags per halaman sudah ada; sitemap otomatis dan schema markup terstruktur belum diverifikasi cakupannya.
*   **Company Profile Page (portofolio/blog/tim)** — 🗺️ ROADMAP: varian produk landing page ini belum diimplementasikan, masih arah bisnis.

### D. Keanggotaan & Loyalty Program (Membership) — 🗺️ ROADMAP
Belum ada implementasi. Item di bawah ini arah bisnis, bukan fitur yang tersedia:
*   **Tiering System**: Pembagian tingkat keanggotaan pelanggan (misalnya: Silver, Gold, Platinum).
*   **Reward Point**: Akumulasi poin reward otomatis berdasarkan transaksi belanja online maupun kasir fisik.
*   **Voucher & Kupon**: Penerbitan voucher diskon digital yang terintegrasi dengan checkout pembayaran.

### E. Point of Sales (POS) — 🗺️ ROADMAP
Belum ada implementasi. Item di bawah ini arah bisnis, bukan fitur yang tersedia:
*   **Antarmuka Kasir Cepat**: Tampilan kasir yang dioptimalkan untuk perangkat tablet maupun desktop.
*   **Manajemen Inventori**: Pengelolaan stok secara real-time, sistem mutasi stok antar cabang, dan peringatan dini stok menipis.
*   **Multi-Outlet**: Skalabilitas pengelolaan kasir untuk banyak cabang toko dalam satu tenant.

---

## 2. Kapabilitas Integrasi Platform (External Skills/Integrations)

Platform ini memiliki pustaka adapter siap pakai di bawah direktori `internal/platform/` untuk menghubungkan aplikasi ke infrastruktur pihak ketiga:

*   **MikroTik** — 🗺️ ROADMAP: hanya `doc.go` (scaffold kosong) di `internal/platform/mikrotik`, belum ada implementasi manajemen bandwidth/IP/profiling jaringan.
*   **Payment Gateway** — 🟡 PARTIAL: gateway aktif saat ini adalah **DOKU** (`internal/platform/doku`, via modul `billing`) — Virtual Account, checkout, callback webhook, dan fallback rekonsiliasi via Check Status API sudah berjalan. *(Xendit tidak dipakai — folder `internal/platform/xendit` tidak ada di kode.)*
*   **WhatsApp Gateway** — 🟡 PARTIAL: adapter client (`internal/platform/whatsapp`) sudah ada, tapi worker notifikasi (`cmd/worker`) belum memprosesnya — saat ini hanya email dispatcher yang aktif di worker.
*   **Redis Engine** — 🟡 PARTIAL: **dipastikan** dipakai untuk session storage. Cache invalidation dasar sudah ada (`internal/platform/redis/cache_invalidation.go`), tapi rate limiting dan caching query umum belum diimplementasikan — masih *future improvement*.
*   **Storage** — 🟡 PARTIAL: local storage provider (`internal/platform/storage/local.go`) sudah berjalan untuk media upload. Adapter S3-compatible (AWS S3, MinIO, Cloudflare R2) **belum ada di kode** — masih roadmap.

---

## 3. Matriks Hak Akses & Permission

> 🗺️ **ROADMAP untuk sebagian besar tabel di bawah**: struktur scope (A) dan konsep permission
> `resource.action` (implementasi custom Go, bukan Casbin) sudah **✅ LIVE** di `internal/core/permission`.
> Tapi daftar modul di tabel B (Lead, Customer, Contact, Deal, Pipeline, dst.) adalah permission untuk
> modul CRM yang **belum diimplementasikan** (lihat 🗺️ ROADMAP di bagian B atas). Permission yang sudah
> live saat ini mengikuti modul nyata: `landing.*`, `organization.*`, `user.*`, `role.*`, `billing.*`.

Akses kontrol ke resource sistem diatur menggunakan format permission `resource.action` dikombinasikan dengan batas jangkauan data (scope).

### A. Level Tingkatan Scope (Data Boundaries)
Batas jangkauan data yang boleh diakses atau diubah oleh suatu role:
*   `none`: Tidak memiliki hak akses.
*   `own`: Hanya dapat mengakses data yang dimiliki oleh pengguna itu sendiri.
*   `team`: Dapat mengakses data milik seluruh anggota dalam tim yang sama.
*   `branch`: Mengakses data dalam batasan satu kantor cabang (outlet).
*   `department`: Mengakses data dalam departemen kerja tertentu.
*   `organization`: Mengakses seluruh data dalam satu organisasi/tenant.
*   `all`: Hak akses global lintas tenant (hanya untuk Super Admin platform).

### B. Daftar Permission Standard
Daftar modul beserta actions default yang tersedia untuk dikonfigurasi dalam Permission Matrix:

| Modul | Resource Penamaan | Actions yang Didukung |
| :--- | :--- | :--- |
| **Lead** | `lead` | `read`, `create`, `update`, `delete`, `restore`, `assign`, `convert`, `import`, `export`, `merge` |
| **Customer** | `customer` | `read`, `create`, `update`, `delete`, `restore`, `archive`, `merge`, `export`, `import` |
| **Contact** | `contact` | `read`, `create`, `update`, `delete`, `restore`, `export`, `import` |
| **Company** | `company` | `read`, `create`, `update`, `delete`, `restore`, `archive`, `merge` |
| **Deal** | `deal` | `read`, `create`, `update`, `delete`, `move_stage`, `close_won`, `close_lost`, `view_value`, `approve_discount` |
| **Pipeline** | `pipeline` | `read`, `create`, `update`, `delete`, `configure_stage`, `archive` |
| **Activity** | `activity` | `read`, `create`, `update`, `delete`, `complete`, `cancel`, `assign` |
| **Quotation** | `quotation` | `read`, `create`, `update`, `delete`, `send`, `approve`, `reject`, `view_price`, `approve_discount` |
| **Invoice** | `invoice` | `read`, `create`, `update`, `delete`, `send`, `mark_paid`, `cancel`, `view_amount` |
| **Integration** | `integration` | `read`, `create`, `update`, `delete`, `connect`, `view_secret`, `update_secret` |
| **User & Role** | `user` / `role` | `user.read`, `user.create`, `user.assign_role`, `role.manage`, `role.assign_permission` |
