INSERT INTO roles (
	role_name,
	slug,
	description,
	is_system,
	created_at,
	updated_at
)
VALUES (
	'member',
	'member',
	'Default role for self-registered Google account users',
	false,
	now(),
	now()
)
ON CONFLICT (slug)
DO UPDATE SET
	role_name = EXCLUDED.role_name,
	description = EXCLUDED.description,
	updated_at = now();
