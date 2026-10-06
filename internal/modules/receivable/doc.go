// Package receivable adalah modul penagihan tenant ke pelanggannya sendiri:
// account, invoice, kiriman, dan pembayaran. Ini BUKAN billing platform
// (billing_invoices = tagihan platform ke tenant). Modul ini tidak mengimpor
// internal/modules/crm; CRM yang boleh memanggil service receivable.
package receivable
