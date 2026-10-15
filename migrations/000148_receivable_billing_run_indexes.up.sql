-- Indeks query item contract jatuh tempo untuk billing run harian (S5).
CREATE INDEX IF NOT EXISTS idx_receivable_contract_items_due
	ON receivable_contract_items(organization_id, payment_timing, next_period_start, next_period_end);
CREATE INDEX IF NOT EXISTS idx_receivable_contracts_active
	ON receivable_contracts(organization_id, status, end_date);
