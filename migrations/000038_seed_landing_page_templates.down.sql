ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
template_slugs (slug) AS (
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
)
DELETE FROM landing_section_templates t
USING platform_org po, template_slugs s
WHERE t.organization_id = po.id
	AND t.content->'metadata'->>'templateSlug' = s.slug;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
template_slugs (slug) AS (
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
)
DELETE FROM landing_pages p
USING platform_org po, template_slugs s
WHERE p.organization_id = po.id
	AND p.slug = s.slug
	AND p.settings->>'kind' = 'landing_page_template';

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates ENABLE ROW LEVEL SECURITY;
