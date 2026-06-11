# SKILLS.md - Peta Kapabilitas dan Matriks Hak Akses Platform Zyad Cloud

Dokumen ini mendokumentasikan kapabilitas teknis (skills), modul aplikasi, integrasi eksternal, dan struktur kontrol akses (permission matrix) yang didukung oleh platform Multi-Tenant Zyad Cloud.

---

## 1. Modul Aplikasi & Fitur Inti (Core & Tenant Modules)

Setiap tenant pada platform Zyad Cloud dapat mengaktifkan atau menonaktifkan modul produk berikut secara dinamis sesuai kebutuhan bisnis mereka:

### A. Arsitektur Multi-Tenancy (Multi-Tenant Core)
*   **Data Isolation (Row-Level Security)**: Memisahkan data antar tenant dalam basis data yang sama (`Shared Database, Shared Schema`) menggunakan `Tenant ID` untuk menekan penggunaan resource pada tenant berskala kecil-menengah.
*   **Database-per-Tenant**: Koneksi basis data terpisah secara fisik untuk tenant berskala Enterprise yang membutuhkan standar keamanan dan kepatuhan data yang ketat.
*   **Tenant Resolution**: Identifikasi identitas tenant secara dinamis melalui subdomain request (`tenant.platform.com`), kustomisasi domain (`www.tenantcustom.com`), atau melalui HTTP header (`X-Tenant-ID`).

### B. CRM (Customer Relationship Management)
*   **Lead & Contact Management**: Melacak prospek potensial dan mengelola database kontak pelanggan secara terpusat.
*   **Sales Pipeline**: Visualisasi tahapan penjualan (sales stages) menggunakan Kanban Board dinamis.
*   **Interaction Logs**: Pencatatan riwayat interaksi pelanggan (chat, email, transaksi kasir) secara otomatis.
*   **Customer Segmentation**: Pengelompokan pelanggan berdasarkan loyalitas dan aktivitas belanja.

### C. Landing Page & Company Profile Generator
*   **CMS Dinamis**: Editor templat landing page promo terintegrasi.
*   **Optimasi SEO Bawaan**: Otomatisasi pembuatan sitemap, pengelolaan meta tags, dan schema markup.
*   **Portofolio & Blog**: Pengelolaan artikel, info layanan, portofolio tim, didukung dengan mekanisme caching performa tinggi.

### D. Keanggotaan & Loyalty Program (Membership)
*   **Tiering System**: Pembagian tingkat keanggotaan pelanggan (misalnya: Silver, Gold, Platinum).
*   **Reward Point**: Akumulasi poin reward otomatis berdasarkan transaksi belanja online maupun kasir fisik.
*   **Voucher & Kupon**: Penerbitan voucher diskon digital yang terintegrasi dengan checkout pembayaran.

### E. Point of Sales (POS)
*   **Antarmuka Kasir Cepat**: Tampilan kasir yang dioptimalkan untuk perangkat tablet maupun desktop.
*   **Manajemen Inventori**: Pengelolaan stok secara real-time, sistem mutasi stok antar cabang, dan peringatan dini stok menipis.
*   **Multi-Outlet**: Skalabilitas pengelolaan kasir untuk banyak cabang toko dalam satu tenant.

---

## 2. Kapabilitas Integrasi Platform (External Skills/Integrations)

Platform ini memiliki pustaka adapter siap pakai di bawah direktori `internal/platform/` untuk menghubungkan aplikasi ke infrastruktur pihak ketiga:

*   **MikroTik**: Mengendalikan dan berintegrasi dengan router MikroTik (manajemen bandwidth, konfigurasi IP, profiling jaringan).
*   **Xendit (Payment Gateway)**: Memproses pembayaran online instan (Virtual Account, E-Wallet, QRIS, Kartu Kredit), memproses callback webhook, dan melakukan rekonsiliasi pembayaran otomatis.
*   **WhatsApp Gateway**: Mengirimkan notifikasi OTP, tagihan bulanan (billing), konfirmasi pembayaran, serta chat pengingat otomatis.
*   **Redis Engine**: Digunakan untuk penyimpanan session token, rotasi Refresh Token JWT, pengimplementasian rate limiting, serta caching query basis data.
*   **Storage (S3-Compatible)**: Pengunggahan dan pengelolaan aset gambar/dokumen secara aman menggunakan local storage atau S3-compatible API (AWS S3, MinIO, Cloudflare R2).

---

## 3. Matriks Hak Akses & Permission

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
