ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates DISABLE ROW LEVEL SECURITY;

WITH template_sections AS (
	SELECT
		p.organization_id,
		p.slug,
		p.name AS page_template_name,
		p.page_type,
		s.section_key,
		s.section_type,
		s.name AS section_name,
		s.sort_order,
		(s.content - 'source') || jsonb_build_object(
			'metadata',
			coalesce(s.content->'metadata', '{}'::jsonb) || jsonb_build_object(
				'templateSlug', p.slug,
				'sectionKey', s.section_key,
				'pageType', p.page_type
			)
		) AS template_content,
		coalesce(s.style, '{}'::jsonb) || jsonb_build_object(
			'template', jsonb_build_object(
				'slug', p.slug,
				'sectionKey', s.section_key,
				'sortOrder', s.sort_order
			)
		) AS template_style
	FROM landing_pages p
	JOIN landing_page_sections s
		ON s.organization_id = p.organization_id
		AND s.landing_page_id = p.id
		AND s.deleted_at IS NULL
	WHERE p.deleted_at IS NULL
		AND (
			p.is_template = true
			OR p.settings->>'kind' = 'landing_page_template'
		)
),
removed_stale AS (
	DELETE FROM landing_section_templates t
	USING template_sections ts
	WHERE t.organization_id = ts.organization_id
		AND t.content->'metadata'->>'templateSlug' = ts.slug
		AND t.content->'metadata'->>'sectionKey' = ts.section_key
	RETURNING t.id
),
inserted_templates AS (
	INSERT INTO landing_section_templates (
		organization_id,
		name,
		description,
		section_type,
		content,
		style,
		created_at,
		updated_at
	)
	SELECT
		ts.organization_id,
		ts.page_template_name || ' - ' || ts.section_name,
		'Master section template for landing page template "' || ts.page_template_name || '".',
		ts.section_type,
		ts.template_content,
		ts.template_style,
		now(),
		now()
	FROM template_sections ts
	ORDER BY ts.slug, ts.sort_order
	RETURNING
		id,
		organization_id,
		content->'metadata'->>'templateSlug' AS slug,
		content->'metadata'->>'sectionKey' AS section_key,
		content,
		style
)
UPDATE landing_page_sections s
SET
	content = it.content || jsonb_build_object(
		'source',
		jsonb_build_object(
			'sectionTemplateId', it.id,
			'templateSlug', it.slug,
			'copiedAt', now()
		)
	),
	style = it.style,
	updated_at = now()
FROM landing_pages p
JOIN inserted_templates it
	ON it.organization_id = p.organization_id
	AND it.slug = p.slug
WHERE s.organization_id = p.organization_id
	AND s.landing_page_id = p.id
	AND s.section_key = it.section_key
	AND s.deleted_at IS NULL
	AND p.deleted_at IS NULL
	AND (
		p.is_template = true
		OR p.settings->>'kind' = 'landing_page_template'
	);

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates ENABLE ROW LEVEL SECURITY;
