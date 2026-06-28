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

WITH platform_org_existing AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
platform_org_new AS (
	INSERT INTO organizations (type, slug, name, status, data_placement)
	SELECT 'platform', 'zyad-cloud-platform', 'Zyad Cloud Platform', 'active', 'shared'
	WHERE NOT EXISTS (SELECT 1 FROM platform_org_existing)
	RETURNING id
),
platform_org AS (
	SELECT id FROM platform_org_existing
	UNION ALL
	SELECT id FROM platform_org_new
),
templates_data (
	sort_order,
	slug,
	name,
	description,
	page_type,
	industry,
	audience,
	hero_title,
	hero_description,
	primary_cta,
	secondary_cta,
	accent_color,
	secondary_color,
	surface_color,
	visual_prompt
) AS (
	VALUES
		(10, 'enterprise-saas-command-center', 'Enterprise SaaS Command Center', 'Landing page untuk SaaS B2B dengan dashboard, proof enterprise, dan demo pipeline.', 'product_service', 'SaaS', 'COO, CTO, dan founder B2B', 'Operate every team from one intelligent command center', 'Satukan workflow, approval, analytics, dan customer lifecycle dalam satu platform yang siap scale lintas tenant.', 'Book enterprise demo', 'View platform tour', '#2563EB', '#0F172A', '#EFF6FF', 'Clean enterprise dashboard with analytics cards, workflow timeline, and team activity stream'),
		(20, 'ai-automation-consulting', 'AI Automation Consulting', 'Template konsultasi AI dan workflow automation dengan nuansa premium dan futuristik.', 'lead_capture', 'AI Consulting', 'Business owner dan operations lead', 'Turn repetitive work into AI-powered operating leverage', 'Identifikasi proses manual, bangun automation yang terukur, dan bantu tim bergerak lebih cepat tanpa kehilangan kontrol.', 'Audit my workflow', 'See automation map', '#7C3AED', '#111827', '#F5F3FF', 'AI workflow canvas with nodes, approval gates, and executive reporting panel'),
		(30, 'cloud-infrastructure-platform', 'Cloud Infrastructure Platform', 'Template cloud infrastructure untuk managed service, migration, dan observability.', 'product_service', 'Cloud Infrastructure', 'CTO, DevOps, dan IT manager', 'Deploy resilient cloud infrastructure without the operational drag', 'Dari assessment sampai monitoring, bantu tim Anda menjalankan workload kritikal dengan reliability dan visibility enterprise.', 'Plan cloud migration', 'Explore architecture', '#0284C7', '#0B1220', '#E0F2FE', 'Cloud architecture diagram with region cards, uptime metrics, and observability traces'),
		(40, 'cybersecurity-trust-center', 'Cybersecurity Trust Center', 'Template cybersecurity dengan trust signal kuat, risk score, dan incident readiness.', 'lead_capture', 'Cybersecurity', 'CISO, IT manager, dan founder', 'Build customer trust with security that is visible and measurable', 'Perkuat posture keamanan, audit access, dan incident response agar bisnis siap menghadapi risiko digital.', 'Run security assessment', 'Download checklist', '#DC2626', '#111827', '#FEF2F2', 'Security operations center with risk score, compliance checklist, and alert timeline'),
		(50, 'fintech-payment-gateway', 'Fintech Payment Gateway', 'Template payment gateway dengan conversion, compliance, dan developer-first messaging.', 'product_service', 'Fintech', 'Merchant, product lead, dan developer', 'Accept payments faster with enterprise-grade reliability', 'Integrasikan pembayaran, rekonsiliasi, webhook, dan settlement dalam pengalaman checkout yang cepat dan aman.', 'Start integration', 'Read API docs', '#059669', '#082F49', '#ECFDF5', 'Payment dashboard with checkout preview, transaction feed, and settlement graph'),
		(60, 'digital-bank-onboarding', 'Digital Bank Onboarding', 'Template onboarding bank digital untuk akuisisi customer dan verifikasi cepat.', 'lead_capture', 'Digital Banking', 'Growth dan compliance team', 'Launch a digital banking onboarding flow customers can trust', 'Percepat registrasi, verifikasi, edukasi produk, dan activation journey dengan landing page yang jelas dan kredibel.', 'Start onboarding', 'View compliance flow', '#0D9488', '#111827', '#F0FDFA', 'Mobile banking onboarding screens with KYC steps and trust badges'),
		(70, 'insurtech-claims-platform', 'Insurtech Claims Platform', 'Template insurtech untuk klaim, policy, dan customer service automation.', 'product_service', 'Insurtech', 'Insurance operator dan partnership team', 'Make insurance claims clear, fast, and customer-friendly', 'Bantu nasabah memahami polis, submit klaim, dan melacak proses dengan komunikasi yang transparan.', 'Simplify claims', 'See policy journey', '#4F46E5', '#172554', '#EEF2FF', 'Insurance claim tracker with document checklist and customer support timeline'),
		(80, 'healthcare-clinic-suite', 'Healthcare Clinic Suite', 'Template klinik dan layanan kesehatan dengan booking, layanan, dan kepercayaan pasien.', 'lead_capture', 'Healthcare', 'Clinic owner dan hospital marketer', 'Modernize patient booking with a trusted digital front door', 'Tampilkan layanan, jadwal dokter, paket pemeriksaan, dan form appointment dalam experience yang rapi.', 'Book appointment', 'View services', '#0891B2', '#164E63', '#ECFEFF', 'Clinic landing page with appointment widget, doctor cards, and care pathway'),
		(90, 'telemedicine-growth-hub', 'Telemedicine Growth Hub', 'Template telemedicine dengan app-first hero dan patient acquisition.', 'lead_capture', 'Telemedicine', 'Healthtech founder dan growth team', 'Bring care closer with a telemedicine experience built for conversion', 'Jelaskan konsultasi online, subscription, dan follow-up pasien dengan visual mobile yang kuat.', 'Start consultation', 'See patient journey', '#16A34A', '#14532D', '#F0FDF4', 'Telemedicine mobile app screens with doctor video card and health metrics'),
		(100, 'education-course-academy', 'Education Course Academy', 'Template course academy untuk cohort, webinar, dan program sertifikasi.', 'promo_event', 'Education', 'Course creator dan training provider', 'Sell premium learning programs with a high-trust academy page', 'Bangun minat calon peserta lewat kurikulum, mentor, outcome, testimoni, dan seat scarcity.', 'Reserve my seat', 'Preview curriculum', '#EA580C', '#1F2937', '#FFF7ED', 'Online academy interface with curriculum modules, mentor cards, and progress indicators'),
		(110, 'corporate-training-portal', 'Corporate Training Portal', 'Template corporate learning untuk HR, L&D, dan enterprise training.', 'product_service', 'Corporate Learning', 'HR director dan L&D manager', 'Upskill teams with training programs executives can measure', 'Tawarkan program training terstruktur dengan kompetensi, dashboard progress, dan laporan ROI.', 'Discuss training plan', 'View program tracks', '#9333EA', '#312E81', '#FAF5FF', 'Corporate learning dashboard with competency matrix and team progress cards'),
		(120, 'real-estate-premium-launch', 'Real Estate Premium Launch', 'Template launching properti premium dengan booking visit dan visual elegan.', 'campaign', 'Real Estate', 'Developer properti dan sales director', 'Launch premium properties with a page that feels investment-grade', 'Tampilkan lokasi, fasilitas, unit unggulan, dan lead capture untuk calon pembeli bernilai tinggi.', 'Schedule private tour', 'Download brochure', '#B45309', '#1C1917', '#FFFBEB', 'Luxury property hero with unit gallery, location map, and investment highlights'),
		(130, 'property-management-system', 'Property Management System', 'Template SaaS manajemen properti untuk apartemen, kos, dan commercial building.', 'product_service', 'Property SaaS', 'Property manager dan building owner', 'Run every property operation from one connected workspace', 'Kelola tenant, invoice, maintenance, occupancy, dan laporan unit secara realtime.', 'Request PMS demo', 'See operations flow', '#2563EB', '#1E293B', '#EFF6FF', 'Property management dashboard with occupancy grid, maintenance tickets, and billing cards'),
		(140, 'hospitality-resort-experience', 'Hospitality Resort Experience', 'Template hotel/resort premium dengan booking, experience, dan gallery cinematic.', 'campaign', 'Hospitality', 'Hotel owner dan resort marketer', 'Create a resort booking experience guests remember before arrival', 'Gabungkan kamar, experience, dining, dan seasonal package dalam landing page immersive.', 'Check availability', 'Explore experiences', '#0F766E', '#134E4A', '#F0FDFA', 'Premium resort scene with booking widget, experience cards, and elegant room gallery'),
		(150, 'restaurant-reservation-engine', 'Restaurant Reservation Engine', 'Template restoran modern untuk reservation, menu highlight, dan event dining.', 'lead_capture', 'Restaurant', 'Restaurant owner dan F&B manager', 'Fill more tables with a reservation page built for appetite and action', 'Tampilkan signature menu, chef story, dining package, dan booking cepat untuk pelanggan baru.', 'Reserve a table', 'View signature menu', '#E11D48', '#18181B', '#FFF1F2', 'Restaurant reservation interface with menu cards, table availability, and chef highlight'),
		(160, 'ecommerce-brand-launch', 'Ecommerce Brand Launch', 'Template launching brand e-commerce dengan product storytelling dan conversion.', 'campaign', 'Ecommerce', 'DTC founder dan brand manager', 'Launch products with storytelling that converts attention into orders', 'Bangun product desire lewat hero visual, benefits, reviews, bundle offer, dan checkout CTA.', 'Shop launch offer', 'See product benefits', '#DB2777', '#111827', '#FDF2F8', 'Product launch page with hero product render, review cards, and bundle offer panel'),
		(170, 'retail-omnichannel-suite', 'Retail Omnichannel Suite', 'Template retail omnichannel untuk inventory, member, POS, dan loyalty.', 'product_service', 'Retail', 'Retail owner dan operations manager', 'Connect store, inventory, and loyalty into one retail growth engine', 'Tawarkan sistem retail yang menyatukan POS, stok, pelanggan, campaign, dan laporan cabang.', 'See retail suite', 'Calculate ROI', '#0891B2', '#0F172A', '#ECFEFF', 'Retail operations dashboard with store map, inventory alerts, and loyalty metrics'),
		(180, 'logistics-fleet-visibility', 'Logistics Fleet Visibility', 'Template logistik untuk fleet visibility, tracking, SLA, dan dispatch.', 'product_service', 'Logistics', 'Logistics director dan fleet operator', 'Move every shipment with visibility your customers can feel', 'Tampilkan tracking, dispatch, SLA monitoring, proof of delivery, dan analytics operasional.', 'Optimize my fleet', 'See tracking demo', '#F97316', '#1F2937', '#FFF7ED', 'Fleet tracking dashboard with route map, delivery timeline, and SLA cards'),
		(190, 'manufacturing-operations-cloud', 'Manufacturing Operations Cloud', 'Template manufacturing untuk production visibility, maintenance, dan quality.', 'product_service', 'Manufacturing', 'Plant manager dan operations director', 'Bring factory operations into a measurable digital control room', 'Pantau output, downtime, quality issue, maintenance, dan supply signal dalam satu halaman penawaran.', 'Map factory workflow', 'View control room', '#475569', '#0F172A', '#F8FAFC', 'Manufacturing control room with production line metrics and maintenance board'),
		(200, 'construction-project-control', 'Construction Project Control', 'Template konstruksi untuk project management, progress, budget, dan vendor.', 'product_service', 'Construction', 'Contractor dan project owner', 'Control construction projects with real-time progress and budget clarity', 'Jelaskan kapabilitas project tracking, vendor coordination, dokumentasi lapangan, dan reporting owner.', 'Discuss project control', 'View reporting sample', '#CA8A04', '#1C1917', '#FEFCE8', 'Construction project dashboard with progress timeline, budget chart, and site photo log'),
		(210, 'legal-advisory-firm', 'Legal Advisory Firm', 'Template firma hukum premium dengan trust, expertise, dan consultation booking.', 'company_profile', 'Legal', 'Law firm partner dan legal consultant', 'Present legal expertise with clarity, authority, and calm confidence', 'Tampilkan practice area, case approach, partner profile, dan consultation CTA tanpa terasa kaku.', 'Book legal consultation', 'Explore practice areas', '#1D4ED8', '#111827', '#EFF6FF', 'Elegant legal firm page with practice cards, partner profile, and consultation scheduler'),
		(220, 'accounting-tax-advisory', 'Accounting Tax Advisory', 'Template akuntansi dan pajak untuk advisory, compliance, dan retainer.', 'company_profile', 'Accounting', 'SME owner dan finance manager', 'Make finance, tax, and compliance feel organized from day one', 'Bangun kepercayaan lewat paket layanan, timeline compliance, client proof, dan CTA konsultasi.', 'Get compliance review', 'View service packages', '#047857', '#0F172A', '#ECFDF5', 'Finance advisory dashboard with tax calendar, compliance checklist, and retainer packages'),
		(230, 'hr-recruitment-platform', 'HR Recruitment Platform', 'Template HR/recruitment untuk talent pipeline, employer branding, dan ATS.', 'lead_capture', 'HR Tech', 'HR manager dan recruitment agency', 'Hire stronger teams with a recruiting engine candidates enjoy', 'Tampilkan job funnel, screening automation, employer brand, dan analytics hiring.', 'Improve hiring funnel', 'See candidate flow', '#7C3AED', '#1E1B4B', '#F5F3FF', 'Recruitment pipeline board with candidate cards, interview schedule, and hiring metrics'),
		(240, 'event-conference-summit', 'Event Conference Summit', 'Template event premium untuk konferensi, seminar, dan summit.', 'promo_event', 'Event', 'Event organizer dan community lead', 'Sell out a high-value event with a landing page built for momentum', 'Tampilkan speaker, agenda, ticket tier, sponsor, venue, dan urgency dalam alur yang smooth.', 'Get ticket', 'View agenda', '#EA580C', '#111827', '#FFF7ED', 'Conference landing page with speaker cards, agenda timeline, and ticket tiers'),
		(250, 'creative-agency-showcase', 'Creative Agency Showcase', 'Template creative agency dengan portfolio, campaign proof, dan pitch CTA.', 'portfolio_case_study', 'Creative Agency', 'Agency owner dan studio director', 'Show the work, the thinking, and the business impact in one sharp pitch', 'Bawa case study, service stack, process, dan result metric ke landing page agency yang memorable.', 'Start a project', 'View case studies', '#BE123C', '#18181B', '#FFF1F2', 'Agency portfolio wall with campaign cards, process steps, and impact metrics'),
		(260, 'startup-investor-deck', 'Startup Investor Deck', 'Template startup untuk fundraising, waitlist, dan investor narrative.', 'lead_capture', 'Startup', 'Founder dan investor relations', 'Turn your startup narrative into an investor-ready growth page', 'Jelaskan problem, traction, product, market, team, dan ask secara ringkas namun kredibel.', 'Request investor brief', 'Join waitlist', '#2563EB', '#111827', '#EFF6FF', 'Startup pitch page with traction metrics, product preview, and investor brief section'),
		(270, 'nonprofit-impact-campaign', 'Nonprofit Impact Campaign', 'Template nonprofit untuk campaign, donation, dan impact reporting.', 'campaign', 'Nonprofit', 'Foundation dan campaign manager', 'Move supporters from empathy to action with measurable impact stories', 'Tampilkan cerita penerima manfaat, progress campaign, donation CTA, dan transparansi penggunaan dana.', 'Support the mission', 'Read impact report', '#16A34A', '#14532D', '#F0FDF4', 'Impact campaign page with donation progress, beneficiary stories, and transparency cards'),
		(280, 'membership-community-hub', 'Membership Community Hub', 'Template komunitas membership untuk subscription, benefit, event, dan member portal.', 'lead_capture', 'Membership', 'Community builder dan association manager', 'Grow a paid community with benefits members can immediately understand', 'Tampilkan benefit, tier, event rutin, member stories, dan onboarding ke portal komunitas.', 'Join the community', 'Compare membership tiers', '#9333EA', '#1E1B4B', '#FAF5FF', 'Community membership page with tier cards, event calendar, and member dashboard preview'),
		(290, 'isp-broadband-package', 'ISP Broadband Package', 'Template ISP broadband untuk paket internet, coverage, dan lead rumah/bisnis.', 'pricing', 'ISP', 'ISP lokal dan sales network', 'Sell internet packages with speed, coverage, and trust upfront', 'Tampilkan paket, area coverage, benefit, instalasi, dan form cek lokasi secara jelas.', 'Check coverage', 'Compare packages', '#0284C7', '#082F49', '#E0F2FE', 'Broadband package page with speed cards, coverage map, and installation timeline'),
		(300, 'mikrotik-managed-network', 'MikroTik Managed Network', 'Template managed network MikroTik untuk hotspot, router, monitoring, dan support.', 'product_service', 'Managed Network', 'Network operator dan reseller IT', 'Offer managed MikroTik networks with enterprise-level clarity', 'Jelaskan monitoring, voucher hotspot, bandwidth control, maintenance, dan SLA support.', 'Audit my network', 'See network stack', '#0D9488', '#134E4A', '#F0FDFA', 'Managed network dashboard with router health, hotspot vouchers, and bandwidth chart'),
		(310, 'pos-business-suite', 'POS Business Suite', 'Template POS untuk retail/F&B dengan cashier, inventory, report, dan loyalty.', 'product_service', 'POS', 'Retail dan F&B owner', 'Run sales, inventory, and loyalty from one business-ready POS suite', 'Tawarkan POS yang mudah dipakai kasir, kuat untuk stok, dan informatif untuk owner.', 'Try POS demo', 'View feature tour', '#EA580C', '#1F2937', '#FFF7ED', 'POS dashboard with cashier screen, inventory cards, and loyalty profile'),
		(320, 'company-profile-enterprise', 'Company Profile Enterprise', 'Template company profile enterprise untuk layanan, trust, portfolio, dan contact.', 'company_profile', 'Company Profile', 'Enterprise service company', 'Present your company with a profile page that feels mature and credible', 'Tampilkan positioning, layanan, portfolio, sertifikasi, tim, dan jalur kontak dalam desain premium.', 'Talk to our team', 'View company profile', '#334155', '#0F172A', '#F8FAFC', 'Enterprise company profile page with service cards, certification badges, and portfolio grid')
),
template_visuals (slug, variant, family, hero_image) AS (
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
inserted_pages AS (
	INSERT INTO landing_pages (
		organization_id,
		name,
		title,
		slug,
		page_type,
		status,
		visibility,
		is_homepage,
		published_version,
		published_at,
		settings,
		seo,
		created_at,
		updated_at
	)
	SELECT
		po.id,
		td.name,
		td.hero_title,
		td.slug,
		td.page_type,
		'published',
		'public',
		false,
		1,
		now(),
		jsonb_build_object(
			'kind', 'landing_page_template',
			'template', jsonb_build_object(
				'slug', td.slug,
				'industry', td.industry,
				'audience', td.audience,
				'sortOrder', td.sort_order,
				'isTenantTemplate', true,
				'tags', jsonb_build_array('enterprise', 'animated', 'conversion', lower(replace(td.industry, ' ', '-')))
			),
			'ui', jsonb_build_object(
				'cardVariant', 'enterprise-glass',
				'previewAspectRatio', '16:10',
				'motion', 'fade-up-stagger'
			)
		),
		jsonb_build_object(
			'meta_title', td.hero_title,
			'meta_description', td.hero_description,
			'robots', jsonb_build_object('index', false, 'follow', false),
			'open_graph', jsonb_build_object(
				'title', td.hero_title,
				'description', td.hero_description
			)
		),
		now(),
		now()
	FROM templates_data td
	CROSS JOIN platform_org po
	ORDER BY td.sort_order
	RETURNING id, organization_id, slug
),
section_blueprints (section_key, section_type, name, sort_order) AS (
	VALUES
		('hero', 'hero', 'Animated Enterprise Hero', 10),
		('trust-strip', 'partner_logos', 'Trust Strip', 20),
		('value-pillars', 'features', 'Value Pillars', 30),
		('proof-statistics', 'statistics', 'Proof Statistics', 40),
		('process', 'services', 'How It Works', 50),
		('pricing', 'pricing', 'Offer Packages', 60),
		('faq', 'faq', 'Buyer FAQ', 70),
		('final-cta', 'cta', 'Final CTA', 80)
),
section_instances AS (
	SELECT
		ip.organization_id,
		ip.id AS landing_page_id,
		td.slug,
		td.name AS template_name,
		td.description AS template_description,
		td.industry,
		td.audience,
		td.hero_title,
		td.hero_description,
		td.primary_cta,
		td.secondary_cta,
		td.accent_color,
		td.secondary_color,
		td.surface_color,
		td.visual_prompt,
		tv.variant,
		tv.family,
		tv.hero_image,
		sb.section_key,
		sb.section_type,
		sb.name AS section_name,
		sb.sort_order,
		CASE sb.section_key
			WHEN 'hero' THEN jsonb_build_object(
				'eyebrow', td.industry || ' Template',
				'title', td.hero_title,
				'description', td.hero_description,
				'primaryCta', td.primary_cta,
				'secondaryCta', td.secondary_cta,
				'trustBadges', jsonb_build_array('Enterprise ready', 'Responsive design', 'Lead capture ready'),
				'visual', jsonb_build_object('type', 'dashboard_mockup', 'prompt', td.visual_prompt, 'motion', 'floating-cards-parallax')
			)
			WHEN 'trust-strip' THEN jsonb_build_object(
				'title', 'Built for teams that need clarity before they commit',
				'logos', jsonb_build_array('Northstar Group', 'Vertex Labs', 'Summit Works', 'Orbit Digital'),
				'motion', 'fade-up-stagger'
			)
			WHEN 'value-pillars' THEN jsonb_build_object(
				'title', 'Everything a buyer needs to understand the offer quickly',
				'description', 'Template ini menyusun value, proof, dan CTA dalam alur enterprise yang mudah dipahami.',
				'items', jsonb_build_array(
					jsonb_build_object('icon', 'shield_check', 'title', 'Credible by default', 'description', 'Trust signal, benefit, dan proof disusun rapi untuk buyer yang butuh keyakinan.'),
					jsonb_build_object('icon', 'sparkles', 'title', 'Animated with purpose', 'description', 'Micro-interaction membantu arah baca dan memperkuat persepsi premium.'),
					jsonb_build_object('icon', 'chart_bar', 'title', 'Conversion focused', 'description', 'CTA dan form diarahkan untuk menghasilkan lead yang mudah ditindaklanjuti.')
				)
			)
			WHEN 'proof-statistics' THEN jsonb_build_object(
				'items', jsonb_build_array(
					jsonb_build_object('label', 'Faster launch', 'value', '14d', 'description', 'Template siap dipakai sebagai base tenant baru.'),
					jsonb_build_object('label', 'Sections included', 'value', '8+', 'description', 'Hero, trust, feature, proof, pricing, FAQ, dan CTA.'),
					jsonb_build_object('label', 'Responsive', 'value', '100%', 'description', 'Disiapkan untuk desktop, tablet, dan mobile.')
				)
			)
			WHEN 'process' THEN jsonb_build_object(
				'title', 'A simple flow from interest to qualified lead',
				'steps', jsonb_build_array(
					jsonb_build_object('number', 1, 'title', 'Discover', 'description', 'Visitor memahami masalah, outcome, dan value utama.'),
					jsonb_build_object('number', 2, 'title', 'Validate', 'description', 'Proof, feature, dan social trust menjawab keraguan.'),
					jsonb_build_object('number', 3, 'title', 'Convert', 'description', 'CTA dan form mengarahkan visitor menjadi lead.')
				)
			)
			WHEN 'pricing' THEN jsonb_build_object(
				'title', 'Package layout ready for tenant offers',
				'plans', jsonb_build_array(
					jsonb_build_object('name', 'Starter', 'priceLabel', 'Custom', 'features', jsonb_build_array('Landing page setup', 'Basic lead form', 'Responsive sections')),
					jsonb_build_object('name', 'Growth', 'priceLabel', 'Recommended', 'features', jsonb_build_array('Full template customization', 'Analytics ready', 'CTA optimization')),
					jsonb_build_object('name', 'Enterprise', 'priceLabel', 'Contact us', 'features', jsonb_build_array('Custom integration', 'Advanced workflow', 'Priority support'))
				)
			)
			WHEN 'faq' THEN jsonb_build_object(
				'title', 'Questions before getting started',
				'items', jsonb_build_array(
					jsonb_build_object('question', 'Apakah template ini bisa disesuaikan brand tenant?', 'answer', 'Bisa. Warna, copywriting, CTA, section, dan asset visual tersimpan sebagai JSON sehingga dapat diubah dari editor.'),
					jsonb_build_object('question', 'Apakah cocok untuk campaign dan company profile?', 'answer', 'Cocok. Page type dan metadata template membantu UI memilih flow yang sesuai.'),
					jsonb_build_object('question', 'Apakah template ini sudah mobile friendly?', 'answer', 'Struktur section dirancang untuk layout responsive dan animasi ringan.')
				)
			)
			ELSE jsonb_build_object(
				'title', 'Ready to turn this template into a tenant landing page?',
				'description', 'Gunakan template ini sebagai starting point, lalu sesuaikan brand, offer, dan form lead tenant.',
				'primaryButtonText', td.primary_cta,
				'secondaryButtonText', td.secondary_cta
			)
		END
		|| jsonb_build_object(
			'metadata', jsonb_build_object(
				'templateSlug', td.slug,
				'sectionKey', sb.section_key,
				'industry', td.industry,
				'templateVariant', tv.variant,
				'designFamily', tv.family
			)
		) AS content,
		jsonb_build_object(
			'variant', tv.variant,
			'family', tv.family,
			'hero', jsonb_build_object(
				'backgroundImage', tv.hero_image,
				'overlay', 'enterprise-readable'
			),
			'colors', jsonb_build_object(
				'primary', td.accent_color,
				'secondary', td.secondary_color,
				'accent', td.accent_color,
				'background', '#FFFFFF',
				'surface', td.surface_color,
				'text', '#0F172A',
				'muted', '#64748B'
			),
			'typography', jsonb_build_object('headingFont', 'Inter', 'bodyFont', 'Inter', 'letterSpacing', '0'),
			'shape', jsonb_build_object('buttonRadius', '8px', 'cardRadius', '8px'),
			'motion', jsonb_build_object('entrance', 'fade-up-stagger', 'hero', 'floating-cards-parallax', 'hover', 'lift-shadow')
		) AS style
	FROM inserted_pages ip
	JOIN templates_data td ON td.slug = ip.slug
	JOIN template_visuals tv ON tv.slug = td.slug
	CROSS JOIN section_blueprints sb
),
inserted_section_templates AS (
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
		organization_id,
		template_name || ' - ' || section_name,
		template_description,
		section_type,
		content,
		style,
		now(),
		now()
	FROM section_instances
	ORDER BY slug, sort_order
	RETURNING
		id,
		organization_id,
		content->'metadata'->>'templateSlug' AS slug,
		content->'metadata'->>'sectionKey' AS section_key,
		section_type,
		content,
		style
)
INSERT INTO landing_page_sections (
	organization_id,
	landing_page_id,
	section_key,
	section_type,
	name,
	sort_order,
	is_enabled,
	content,
	style,
	created_at,
	updated_at
)
SELECT
	si.organization_id,
	si.landing_page_id,
	si.section_key,
	si.section_type,
	si.section_name,
	si.sort_order,
	true,
	ist.content || jsonb_build_object(
		'source', jsonb_build_object(
			'sectionTemplateId', ist.id,
			'templateSlug', si.slug,
			'copiedAt', now()
		)
	),
	ist.style,
	now(),
	now()
FROM section_instances si
JOIN inserted_section_templates ist
	ON ist.organization_id = si.organization_id
	AND ist.slug = si.slug
	AND ist.section_key = si.section_key
ORDER BY si.slug, si.sort_order;

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates ENABLE ROW LEVEL SECURITY;
