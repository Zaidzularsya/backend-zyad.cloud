DELETE FROM notification_templates
WHERE code IN (
	'receivable.payout_account_submitted',
	'receivable.payout_account_verified',
	'receivable.payout_account_rejected',
	'receivable.payout_account_assigned'
)
	AND channel = 'email'
	AND locale = 'id-ID'
	AND version = 1
	AND is_system = true;
