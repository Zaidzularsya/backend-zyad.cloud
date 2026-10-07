// Package receivable adalah modul penagihan tenant ke pelanggannya sendiri:
// account, invoice, kiriman, dan pembayaran. Ini BUKAN billing platform
// (modul billing lama, tagihan platform ke tenant, dipensiunkan di R4-S5; sekarang penagihan platform
// juga lewat modul ini). Modul ini tidak mengimpor
// internal/modules/crm; CRM yang boleh memanggil service receivable.
package receivable
