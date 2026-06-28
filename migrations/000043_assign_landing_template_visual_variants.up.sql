ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates DISABLE ROW LEVEL SECURITY;

WITH template_visuals (slug, variant, family, hero_image) AS (
	VALUES
		('enterprise-saas-command-center', 'software_command', 'software', 'https://images.unsplash.com/photo-1551288049-bebda4e38f71?auto=format&fit=crop&w=1800&q=85'),
		('ai-automation-consulting', 'ai_workflow', 'software', 'https://images.unsplash.com/photo-1677442136019-21780ecad995?auto=format&fit=crop&w=1800&q=85'),
		('cloud-infrastructure-platform', 'cloud_architecture', 'software', 'https://images.unsplash.com/photo-1451187580459-43490279c0fa?auto=format&fit=crop&w=1800&q=85'),
		('cybersecurity-trust-center', 'security_trust', 'software', 'https://images.unsplash.com/photo-1563986768609-322da13575f3?auto=format&fit=crop&w=1800&q=85'),
		('fintech-payment-gateway', 'fintech_checkout', 'software', 'https://images.unsplash.com/photo-1563013544-824ae1b704d3?auto=format&fit=crop&w=1800&q=85'),
		('digital-bank-onboarding', 'banking_mobile', 'software', 'https://images.unsplash.com/photo-1563986768494-4dee2763ff3f?auto=format&fit=crop&w=1800&q=85'),
		('insurtech-claims-platform', 'insurance_claims', 'advisory', 'https://images.unsplash.com/photo-1450101499163-c8848c66ca85?auto=format&fit=crop&w=1800&q=85'),
		('healthcare-clinic-suite', 'clinic_booking', 'care', 'https://images.unsplash.com/photo-1576091160399-112ba8d25d1d?auto=format&fit=crop&w=1800&q=85'),
		('telemedicine-growth-hub', 'telemedicine_app', 'care', 'https://images.unsplash.com/photo-1579684385127-1ef15d508118?auto=format&fit=crop&w=1800&q=85'),
		('education-course-academy', 'academy_cohort', 'community', 'https://images.unsplash.com/photo-1523580846011-d3a5bc25702b?auto=format&fit=crop&w=1800&q=85'),
		('corporate-training-portal', 'corporate_learning', 'advisory', 'https://images.unsplash.com/photo-1552664730-d307ca884978?auto=format&fit=crop&w=1800&q=85'),
		('real-estate-premium-launch', 'property_editorial', 'experience', 'https://images.unsplash.com/photo-1600585154340-be6161a56a0c?auto=format&fit=crop&w=1800&q=85'),
		('property-management-system', 'property_dashboard', 'operations', 'https://images.unsplash.com/photo-1560518883-ce09059eeffa?auto=format&fit=crop&w=1800&q=85'),
		('hospitality-resort-experience', 'resort_immersive', 'experience', 'https://images.unsplash.com/photo-1507525428034-b723cf961d3e?auto=format&fit=crop&w=1800&q=85'),
		('restaurant-reservation-engine', 'restaurant_editorial', 'experience', 'https://images.unsplash.com/photo-1517248135467-4c7edcad34c4?auto=format&fit=crop&w=1800&q=85'),
		('ecommerce-brand-launch', 'commerce_product', 'experience', 'https://images.unsplash.com/photo-1441986300917-64674bd600d8?auto=format&fit=crop&w=1800&q=85'),
		('retail-omnichannel-suite', 'retail_operations', 'operations', 'https://images.unsplash.com/photo-1556742049-0cfed4f6a45d?auto=format&fit=crop&w=1800&q=85'),
		('logistics-fleet-visibility', 'fleet_map', 'operations', 'https://images.unsplash.com/photo-1586528116311-ad8dd3c8310d?auto=format&fit=crop&w=1800&q=85'),
		('manufacturing-operations-cloud', 'factory_control', 'operations', 'https://images.unsplash.com/photo-1565043666747-69f6646db940?auto=format&fit=crop&w=1800&q=85'),
		('construction-project-control', 'construction_progress', 'operations', 'https://images.unsplash.com/photo-1503387762-592deb58ef4e?auto=format&fit=crop&w=1800&q=85'),
		('legal-advisory-firm', 'legal_authority', 'advisory', 'https://images.unsplash.com/photo-1589829545856-d10d557cf95f?auto=format&fit=crop&w=1800&q=85'),
		('accounting-tax-advisory', 'finance_advisory', 'advisory', 'https://images.unsplash.com/photo-1554224155-6726b3ff858f?auto=format&fit=crop&w=1800&q=85'),
		('hr-recruitment-platform', 'talent_pipeline', 'software', 'https://images.unsplash.com/photo-1551836022-d5d88e9218df?auto=format&fit=crop&w=1800&q=85'),
		('event-conference-summit', 'event_summit', 'experience', 'https://images.unsplash.com/photo-1505373877841-8d25f7d46678?auto=format&fit=crop&w=1800&q=85'),
		('creative-agency-showcase', 'agency_portfolio', 'experience', 'https://images.unsplash.com/photo-1556761175-b413da4baf72?auto=format&fit=crop&w=1800&q=85'),
		('startup-investor-deck', 'startup_pitch', 'community', 'https://images.unsplash.com/photo-1559136555-9303baea8ebd?auto=format&fit=crop&w=1800&q=85'),
		('nonprofit-impact-campaign', 'impact_campaign', 'community', 'https://images.unsplash.com/photo-1488521787991-ed7bbaae773c?auto=format&fit=crop&w=1800&q=85'),
		('membership-community-hub', 'community_membership', 'community', 'https://images.unsplash.com/photo-1511632765486-a01980e01a18?auto=format&fit=crop&w=1800&q=85'),
		('isp-broadband-package', 'broadband_network', 'operations', 'https://images.unsplash.com/photo-1558494949-ef010cbdcc31?auto=format&fit=crop&w=1800&q=85'),
		('mikrotik-managed-network', 'managed_network', 'operations', 'https://images.unsplash.com/photo-1544197150-b99a580bb7a8?auto=format&fit=crop&w=1800&q=85'),
		('pos-business-suite', 'pos_checkout', 'operations', 'https://images.unsplash.com/photo-1556742502-ec7c0e9f34b1?auto=format&fit=crop&w=1800&q=85'),
		('company-profile-enterprise', 'company_profile', 'advisory', 'https://images.unsplash.com/photo-1497366754035-f200968a6e72?auto=format&fit=crop&w=1800&q=85')
),
updated_section_templates AS (
	UPDATE landing_section_templates t
	SET
		content = coalesce(t.content, '{}'::jsonb) || jsonb_build_object(
			'metadata',
			coalesce(t.content->'metadata', '{}'::jsonb) || jsonb_build_object(
				'templateVariant', tv.variant,
				'designFamily', tv.family
			)
		),
		style = coalesce(t.style, '{}'::jsonb) || jsonb_build_object(
			'variant', tv.variant,
			'family', tv.family,
			'hero', jsonb_build_object(
				'backgroundImage', tv.hero_image,
				'overlay', 'enterprise-readable'
			)
		),
		updated_at = now()
	FROM template_visuals tv
	WHERE t.deleted_at IS NULL
		AND t.content->'metadata'->>'templateSlug' = tv.slug
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
		coalesce(s.content->'source', '{}'::jsonb) || jsonb_build_object(
			'templateSlug', ust.slug,
			'updatedFromVisualVariantAt', now()
		)
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
