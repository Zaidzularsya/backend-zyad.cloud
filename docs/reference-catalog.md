# Reference — Katalog Produk Tenant (`catalog`)

## Tujuan

Tenant menyimpan daftar produk/jasa yang dijualnya (kategori, SKU, satuan, harga dasar, pajak %) agar
sales bisa memilih item saat menyusun quotation CRM (Rilis 2 S3) alih-alih mengetik bebas.

Modul ini **berbeda** dari modul `product`:

| | `catalog` | `product` |
|---|---|---|
| Pemilik data | Tenant (`organization_id`, RLS) | Platform Zyad |
| Isi | Produk/jasa yang dijual tenant ke customer-nya | Plan langganan Zyad, fitur, harga plan, entitlement |
| Route | `/api/v1/app/catalog/*` | `/api/v1/platform/product/*` |

## Tabel (migration `000138`)

- `catalog_product_categories`: `name varchar(100)`, `position int`, audit, soft delete. Nama unik per
  organisasi (case-insensitive) di antara baris yang belum dihapus.
- `catalog_products`: `category_id` (nullable), `sku varchar(64)` (nullable), `name varchar(200)`,
  `description`, `unit varchar(30)` default `pcs`, `base_price numeric(18,2)` ≥ 0,
  `tax_percent numeric(5,2)` 0–100, `currency char(3)` default `IDR`, `is_active`, audit, soft delete.

Kedua tabel memakai `apply_organization_rls`; setiap query repository berjalan dalam transaksi dengan
`set_config('app.organization_id', …)` dan filter `organization_id` eksplisit.

## Aturan

- **SKU** opsional. Bila diisi, unik per organisasi secara case-insensitive (`NET-50` = `net-50`) di antara
  produk yang belum dihapus. Dua produk tanpa SKU boleh. Produk yang dihapus membebaskan SKU-nya.
  Duplikat → 409 `PRODUCT_SKU_EXISTS`.
- **Harga & pajak** dikirim/diterima sebagai string desimal: harga `^\d{1,16}(\.\d{1,2})?$`, pajak 0–100
  maks 2 desimal. Frontend menormalkan format Indonesia (`Rp 1.500.000,50`) sebelum mengirim.
- **Hapus kategori** (soft delete) mengosongkan `category_id` produk di dalamnya; produk tetap ada.
- **Produk nonaktif** tetap tampil di halaman Produk, tetapi tidak muncul untuk `is_active=true`
  (dipakai picker quotation).
- `FindByIDs` (dipakai quotation untuk snapshot) mengembalikan produk nonaktif tetapi tidak yang terhapus;
  quotation yang menolak produk nonaktif melakukannya di service CRM.

## Permission (migration `000139`)

| Permission | organization_owner / super_admin | member |
|---|---|---|
| `catalog_product.read` | ✓ | ✓ |
| `catalog_product.create` | ✓ | — |
| `catalog_product.update` | ✓ | — |
| `catalog_product.delete` | ✓ | — |

Guard tenant sama dengan CRM: `RequireActiveTenant`, `RequireCustomerOrPlatformTenant`,
`RequireEntitlement("crm.enabled")`.

## Endpoint (`/api/v1/app/catalog`)

| Method & path | Permission | Catatan |
|---|---|---|
| `GET /products` | `catalog_product.read` | Query `q` (nama/SKU), `category_id`, `is_active` (`true`/`false`), `page`, `per_page` (default 20, maks 100) |
| `POST /products` | `catalog_product.create` | `{category_id?, sku?, name, description?, unit?, base_price?, tax_percent?, is_active?}`; `is_active` default `true` |
| `GET /products/:id` | `catalog_product.read` | |
| `PATCH /products/:id` | `catalog_product.update` | Field opsional; string kosong mengosongkan `category_id`/`sku`/`description` |
| `DELETE /products/:id` | `catalog_product.delete` | Soft delete |
| `GET /categories` | `catalog_product.read` | Urut `position`, lalu nama |
| `POST /categories` | `catalog_product.create` | `{name, position?}` |
| `PATCH /categories/:id` | `catalog_product.update` | `{name?, position?}` |
| `DELETE /categories/:id` | `catalog_product.delete` | Soft delete + kosongkan kategori produk |

Error: 422 `VALIDATION_ERROR`, 409 `PRODUCT_SKU_EXISTS` / `CATEGORY_NAME_EXISTS`,
404 `PRODUCT_NOT_FOUND` / `CATEGORY_NOT_FOUND` (termasuk akses lintas tenant).

## Di luar scope

Varian, bundle, stok, harga bertingkat/per pelanggan, multi-mata uang, model TM Forum
(Specification → Offering → Price).
