-- Penagihan tenant pindah ke modul receivable (000144); crm_invoices tidak lagi dipakai.
-- WAJIB sebelum dijalankan di produksi: SELECT count(*) FROM crm_invoices;
-- bila > 0, ekspor (COPY ... TO STDOUT CSV) dan simpan di backup sebelum migrate.
DROP TABLE IF EXISTS crm_invoice_items;
DROP TABLE IF EXISTS crm_invoices;
