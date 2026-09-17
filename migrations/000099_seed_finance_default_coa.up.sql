WITH header_rows(account_code, account_name, category_code, normal_balance) AS (
	VALUES
		('1100', 'Aset Lancar', 'current_asset', 'debit'),
		('1200', 'Aset Tetap', 'fixed_asset', 'debit'),
		('1300', 'Aset Lainnya', 'other_asset', 'debit'),
		('2100', 'Kewajiban Jangka Pendek', 'current_liability', 'credit'),
		('2200', 'Kewajiban Jangka Panjang', 'long_term_liability', 'credit'),
		('3100', 'Ekuitas', 'equity', 'credit'),
		('4100', 'Pendapatan Usaha', 'revenue', 'credit'),
		('4200', 'Pendapatan Lain-lain', 'other_income', 'credit'),
		('5100', 'Beban Pokok', 'cogs', 'debit'),
		('5200', 'Beban Operasional', 'operating_expense', 'debit'),
		('5300', 'Beban Lain-lain', 'other_expense', 'debit')
)
INSERT INTO finance_accounts (account_code, account_name, account_category_id, is_header, normal_balance)
SELECT header_rows.account_code, header_rows.account_name, finance_account_categories.id, true, header_rows.normal_balance
FROM header_rows
JOIN finance_account_categories ON finance_account_categories.code = header_rows.category_code
ON CONFLICT (account_code) WHERE deleted_at IS NULL DO NOTHING;

WITH leaf_rows(account_code, account_name, category_code, normal_balance) AS (
	VALUES
		('1101', 'Kas', 'current_asset', 'debit'),
		('1102', 'Bank', 'current_asset', 'debit'),
		('1103', 'Piutang Usaha', 'current_asset', 'debit'),
		('1104', 'PPN Masukan', 'current_asset', 'debit'),
		('1105', 'Biaya Dibayar Dimuka', 'current_asset', 'debit'),
		('1201', 'Peralatan Kantor', 'fixed_asset', 'debit'),
		('1202', 'Akumulasi Penyusutan Peralatan Kantor', 'fixed_asset', 'credit'),
		('1301', 'Aset Lain-lain', 'other_asset', 'debit'),
		('2101', 'Utang Usaha', 'current_liability', 'credit'),
		('2102', 'PPN Keluaran', 'current_liability', 'credit'),
		('2103', 'PPh 21 Terutang', 'current_liability', 'credit'),
		('2104', 'PPh 23 Terutang', 'current_liability', 'credit'),
		('2105', 'Utang Pajak Lainnya', 'current_liability', 'credit'),
		('2106', 'Biaya Yang Masih Harus Dibayar', 'current_liability', 'credit'),
		('2201', 'Utang Jangka Panjang', 'long_term_liability', 'credit'),
		('3101', 'Modal Disetor', 'equity', 'credit'),
		('3102', 'Laba Ditahan', 'equity', 'credit'),
		('4101', 'Pendapatan Langganan SaaS', 'revenue', 'credit'),
		('4102', 'Pendapatan Jasa Lainnya', 'revenue', 'credit'),
		('4201', 'Pendapatan Lain-lain', 'other_income', 'credit'),
		('5101', 'Beban Hosting & Infrastruktur', 'cogs', 'debit'),
		('5201', 'Beban Gaji & Tunjangan', 'operating_expense', 'debit'),
		('5202', 'Beban Sewa Kantor', 'operating_expense', 'debit'),
		('5203', 'Beban Utilitas', 'operating_expense', 'debit'),
		('5204', 'Beban Pemasaran', 'operating_expense', 'debit'),
		('5205', 'Beban Penyusutan', 'operating_expense', 'debit'),
		('5206', 'Beban Administrasi & Umum', 'operating_expense', 'debit'),
		('5207', 'Beban Lain-lain Operasional', 'operating_expense', 'debit'),
		('5301', 'Beban Pajak Penghasilan Badan', 'other_expense', 'debit')
)
INSERT INTO finance_accounts (account_code, account_name, account_category_id, parent_account_id, is_header, normal_balance)
SELECT leaf_rows.account_code, leaf_rows.account_name, category.id, header.id, false, leaf_rows.normal_balance
FROM leaf_rows
JOIN finance_account_categories category ON category.code = leaf_rows.category_code
JOIN finance_accounts header ON header.account_category_id = category.id AND header.is_header = true
ON CONFLICT (account_code) WHERE deleted_at IS NULL DO NOTHING;
