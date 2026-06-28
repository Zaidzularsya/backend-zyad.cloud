ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates DISABLE ROW LEVEL SECURITY;

WITH template_slugs (slug) AS (
	VALUES
		('enterprise-saas-command-center'),
		('ai-automation-consulting'),
		('cloud-infrastructure-platform'),
		('cybersecurity-trust-center'),
		('fintech-payment-gateway'),
		('digital-bank-onboarding'),
		('insurtech-claims-platform'),
		('healthcare-clinic-suite'),
		('telemedicine-growth-hub'),
		('education-course-academy'),
		('corporate-training-portal'),
		('real-estate-premium-launch'),
		('property-management-system'),
		('hospitality-resort-experience'),
		('restaurant-reservation-engine'),
		('ecommerce-brand-launch'),
		('retail-omnichannel-suite'),
		('logistics-fleet-visibility'),
		('manufacturing-operations-cloud'),
		('construction-project-control'),
		('legal-advisory-firm'),
		('accounting-tax-advisory'),
		('hr-recruitment-platform'),
		('event-conference-summit'),
		('creative-agency-showcase'),
		('startup-investor-deck'),
		('nonprofit-impact-campaign'),
		('membership-community-hub'),
		('isp-broadband-package'),
		('mikrotik-managed-network'),
		('pos-business-suite'),
		('company-profile-enterprise')
),
updated_section_templates AS (
	UPDATE landing_section_templates t
	SET
		content = jsonb_set(
			jsonb_set(coalesce(t.content, '{}'::jsonb), '{metadata,templateVariant}', 'null'::jsonb, true) #- '{metadata,templateVariant}',
			'{metadata,designFamily}',
			'null'::jsonb,
			true
		) #- '{metadata,designFamily}',
		style = (coalesce(t.style, '{}'::jsonb) || jsonb_build_object('variant', 'enterprise')) - 'family' - 'hero',
		updated_at = now()
	FROM template_slugs ts
	WHERE t.deleted_at IS NULL
		AND t.content->'metadata'->>'templateSlug' = ts.slug
	RETURNING
		t.organization_id,
		t.content->'metadata'->>'templateSlug' AS slug,
		t.content->'metadata'->>'sectionKey' AS section_key,
		t.content,
		t.style
)
UPDATE landing_page_sections s
SET
	content = ust.content || jsonb_build_object(
		'source',
		coalesce(s.content->'source', '{}'::jsonb) - 'updatedFromVisualVariantAt'
	),
	style = ust.style,
	updated_at = now()
FROM landing_pages p
JOIN updated_section_templates ust
	ON ust.organization_id = p.organization_id
	AND ust.slug = p.slug
WHERE s.organization_id = p.organization_id
	AND s.landing_page_id = p.id
	AND s.section_key = ust.section_key
	AND s.deleted_at IS NULL
	AND p.deleted_at IS NULL
	AND (
		p.is_template = true
		OR p.settings->>'kind' = 'landing_page_template'
	);

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates ENABLE ROW LEVEL SECURITY;
