-- Adds 'header' to the allowed landing section types so the tenant navigation
-- header can be a real per-page section (rendered sticky at the top, content
-- synthesized from branding + the location=header menu, same pattern as footer).

ALTER TABLE landing_page_sections
	DROP CONSTRAINT IF EXISTS landing_page_sections_type_check;
ALTER TABLE landing_page_sections
	ADD CONSTRAINT landing_page_sections_type_check CHECK (
		section_type IN (
			'hero',
			'about',
			'features',
			'services',
			'product_showcase',
			'content',
			'gallery',
			'portfolio',
			'testimonial',
			'pricing',
			'faq',
			'cta',
			'form',
			'contact',
			'newsletter',
			'partner_logos',
			'statistics',
			'footer',
			'header'
		)
	);

ALTER TABLE landing_section_templates
	DROP CONSTRAINT IF EXISTS landing_section_templates_type_check;
ALTER TABLE landing_section_templates
	ADD CONSTRAINT landing_section_templates_type_check CHECK (
		section_type IN (
			'hero',
			'about',
			'features',
			'services',
			'product_showcase',
			'content',
			'gallery',
			'portfolio',
			'testimonial',
			'pricing',
			'faq',
			'cta',
			'form',
			'contact',
			'newsletter',
			'partner_logos',
			'statistics',
			'footer',
			'header'
		)
	);
