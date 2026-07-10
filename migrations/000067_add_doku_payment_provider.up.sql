ALTER TABLE billing_payments
	DROP CONSTRAINT IF EXISTS billing_payments_provider_check;
ALTER TABLE billing_payments
	ADD CONSTRAINT billing_payments_provider_check CHECK (
		provider IN ('manual', 'xendit', 'midtrans', 'doku')
	);

ALTER TABLE billing_payment_events
	DROP CONSTRAINT IF EXISTS billing_payment_events_provider_check;
ALTER TABLE billing_payment_events
	ADD CONSTRAINT billing_payment_events_provider_check CHECK (
		provider IN ('manual', 'xendit', 'midtrans', 'doku')
	);
