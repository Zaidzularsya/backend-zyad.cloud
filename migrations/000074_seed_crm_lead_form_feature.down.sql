WITH seeded_features AS (
	SELECT id
	FROM product_features
	WHERE feature_key = 'crm.lead_form'
)
DELETE FROM product_plan_entitlements
WHERE feature_id IN (SELECT id FROM seeded_features);

DELETE FROM product_features
WHERE feature_key = 'crm.lead_form';
