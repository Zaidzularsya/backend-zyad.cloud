WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'pipeline.read', 'pipeline.create', 'pipeline.update', 'pipeline.delete',
		'pipeline.restore', 'pipeline.archive', 'pipeline.configure_stage',
		'deal.read', 'deal.create', 'deal.update', 'deal.delete',
		'deal.move_stage', 'deal.close_won', 'deal.close_lost', 'deal.approve_discount'
	)
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN (
	'pipeline.read', 'pipeline.create', 'pipeline.update', 'pipeline.delete',
	'pipeline.restore', 'pipeline.archive', 'pipeline.configure_stage',
	'deal.read', 'deal.create', 'deal.update', 'deal.delete',
	'deal.move_stage', 'deal.close_won', 'deal.close_lost', 'deal.approve_discount'
);
