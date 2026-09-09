-- Reverts 'header' from the allowed landing section types.
-- NOTE: this fails if any landing_page_sections / landing_section_templates row
-- still uses section_type = 'header'. Delete those rows first.

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
			'footer'
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
			'footer'
		)
	);
