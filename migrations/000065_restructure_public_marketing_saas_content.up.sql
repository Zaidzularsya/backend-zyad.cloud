-- Restructure the platform public-marketing landing page for the SaaS platform concept:
--  * Fix broken ordering (footer was at sort 90 with five sections after it).
--  * Fix broken variants (solution-section rendered by ServicesSection, benefits-section by
--    ProblemSection, both showing empty because style.variant was never set).
--  * Rewrite all copy from the legacy "HEY Digital Solution" agency pitch to the actual
--    self-serve SaaS platform offering (landing page builder, CRM, WhatsApp automation,
--    custom domain, POS/membership, team & RBAC) with register/pricing CTAs.
--  * Add a statistics section highlighting the platform's value proposition.
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

-- Phase 1: move every existing section of this page out of the target sort range so the
-- per-section upserts below never trip the unique index on
-- (organization_id, landing_page_id, is_enabled, sort_order).
WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
)
UPDATE landing_page_sections
SET sort_order = sort_order + 1000, updated_at = now()
WHERE landing_page_id = (SELECT id FROM landing_page) AND deleted_at IS NULL;

-- Phase 2: upsert each section with its final type, variant, order, and SaaS copy.
WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id, organization_id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
),
sections(section_key, section_type, name, sort_order, style, content) AS (
	VALUES
	(
		'hero-section', 'hero', 'Hero Section', 10, '{}'::jsonb,
		'{
			"badge": "Platform SaaS All-in-One untuk Bisnis Anda",
			"titleHtml": "Bangun, Kelola, dan Kembangkan <span class=\"text-gradient\">Bisnis Digital Anda</span> dari Satu Platform",
			"description": "Landing page builder, CRM, automasi WhatsApp, custom domain, sampai manajemen tim — semua modul terintegrasi dalam satu dashboard. Mulai gratis, upgrade kapan saja mengikuti pertumbuhan bisnis Anda.",
			"primaryCta": "Mulai Gratis Sekarang",
			"primaryCtaUrl": "/auth/register",
			"secondaryCta": "Lihat Paket & Harga",
			"secondaryCtaUrl": "#pricing",
			"floatingBadges": [
				{"icon": "web", "label": "Landing Page Builder"},
				{"icon": "contacts", "label": "CRM & Leads"},
				{"icon": "language", "label": "Custom Domain + SSL"},
				{"icon": "chat", "label": "WhatsApp Automation"},
				{"icon": "storefront", "label": "POS & Membership"},
				{"icon": "group", "label": "Tim & Role Access"}
			]
		}'::jsonb
	),
	(
		'statistics-section', 'statistics', 'Statistics Section', 20, '{}'::jsonb,
		'{
			"items": [
				{"value": "8+", "label": "Modul Terintegrasi", "description": "Landing page, CRM, media, domain, WhatsApp, POS, membership, dan automation dalam satu langganan."},
				{"value": "< 5 Menit", "label": "Dari Daftar ke Online", "description": "Pilih template, sesuaikan konten dan branding, lalu publish — tanpa menulis kode."},
				{"value": "99.9%", "label": "Target Uptime", "description": "Berjalan di infrastruktur cloud dengan SSL otomatis dan monitoring berkala."}
			]
		}'::jsonb
	),
	(
		'problem-section', 'content', 'Problem Section', 30, '{"variant": "problem"}'::jsonb,
		'{
			"title": "Mengelola Bisnis Online Tidak Seharusnya Serumit Ini",
			"description": "Banyak bisnis terjebak menggabungkan bermacam aplikasi yang tidak saling terhubung — mahal, merepotkan, dan datanya tercecer di mana-mana.",
			"cards": [
				{"icon": "scatter_plot", "title": "Tools Tercecer", "desc": "Website di satu aplikasi, data pelanggan di spreadsheet, follow-up manual lewat chat pribadi.", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container"},
				{"icon": "payments", "title": "Biaya Langganan Menumpuk", "desc": "Bayar banyak aplikasi terpisah setiap bulan padahal yang terpakai hanya sebagian fiturnya.", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container"},
				{"icon": "hourglass_empty", "title": "Lama untuk Go-Online", "desc": "Sekadar butuh landing page saja harus menunggu developer dan proses teknis yang panjang.", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container"},
				{"icon": "link_off", "title": "Data Tidak Terhubung", "desc": "Leads dari form tidak otomatis masuk CRM, follow-up terlewat, peluang penjualan hilang.", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container"}
			]
		}'::jsonb
	),
	(
		'solution-section', 'content', 'Solution Section', 40, '{"variant": "solution"}'::jsonb,
		'{
			"title": "Satu Platform, Semua Kebutuhan Online Bisnis Anda",
			"description": "Cukup tiga langkah untuk membawa bisnis Anda online — tanpa coding, tanpa pusing memikirkan server dan infrastruktur.",
			"ctaText": "Mulai Sekarang — Gratis",
			"ctaUrl": "/auth/register",
			"steps": [
				{"number": 1, "title": "Daftar & Dapatkan Workspace", "desc": "Buat akun dengan Google, workspace Anda langsung siap dengan paket Free.", "highlight": false},
				{"number": 2, "title": "Susun & Publish Landing Page", "desc": "Pilih template, sesuaikan konten dan branding, lalu publish dalam hitungan menit.", "highlight": true},
				{"number": 3, "title": "Kelola & Kembangkan", "desc": "Pantau leads di CRM, otomasi follow-up WhatsApp, dan upgrade paket saat bisnis tumbuh.", "highlight": false}
			]
		}'::jsonb
	),
	(
		'services-section', 'features', 'Services Section', 50, '{}'::jsonb,
		'{
			"title": "Modul Lengkap yang Tumbuh Bersama Bisnis Anda",
			"description": "Aktifkan modul sesuai kebutuhan — semuanya sudah terintegrasi dalam satu dashboard dan satu langganan.",
			"services": [
				{"icon": "web", "title": "Landing Page Builder", "desc": "Template siap pakai, editor per-section, SEO meta, dan publish instan dengan versioning."},
				{"icon": "contacts", "title": "CRM & Lead Management", "desc": "Leads dari form landing page otomatis masuk CRM — kelola kontak dan pipeline penjualan."},
				{"icon": "language", "title": "Custom Domain + SSL", "desc": "Sambungkan domain milik Anda dengan sertifikat SSL yang di-provision otomatis."},
				{"icon": "chat", "title": "WhatsApp Automation", "desc": "Notifikasi dan follow-up otomatis ke pelanggan langsung dari platform."},
				{"icon": "perm_media", "title": "Media Manager", "desc": "Kelola gambar dan aset digital dengan kuota penyimpanan sesuai paket."},
				{"icon": "storefront", "title": "POS, Membership & Automation", "desc": "Modul lanjutan untuk penjualan, keanggotaan, dan alur kerja otomatis skala bisnis."}
			]
		}'::jsonb
	),
	(
		'benefits-section', 'content', 'Benefits Section', 60, '{"variant": "benefits"}'::jsonb,
		'{
			"title": "Fokus ke Bisnis, Sisanya Biar Platform yang Urus",
			"description": "Semua urusan teknis — server, keamanan, update — sudah ditangani. Anda tinggal pakai.",
			"benefits": [
				{"icon": "rocket_launch", "title": "Online dalam Hitungan Menit", "desc": "Tanpa coding, tanpa sewa developer, tanpa setup server sendiri."},
				{"icon": "payments", "title": "Satu Langganan, Semua Modul", "desc": "Berhenti membayar banyak aplikasi terpisah — upgrade hanya saat benar-benar butuh."},
				{"icon": "security", "title": "Aman & Terkelola", "desc": "SSL otomatis, data terisolasi per workspace, dan akses tim diatur berbasis role."}
			]
		}'::jsonb
	),
	(
		'pricing-section', 'pricing', 'Pricing Section', 70, '{}'::jsonb,
		'{
			"title": "Pilih Paket Sesuai Skala Bisnis Anda",
			"source": "platform_catalog",
			"billingInterval": "monthly"
		}'::jsonb
	),
	(
		'faq-section', 'faq', 'FAQ Section', 80, '{}'::jsonb,
		'{
			"title": "Pertanyaan yang Sering Diajukan",
			"items": [
				{"question": "Apakah ada paket gratis?", "answer": "Ada. Paket Free dapat dipakai tanpa batas waktu untuk mencoba platform dengan 1 landing page aktif. Anda bisa upgrade kapan saja saat butuh kapasitas lebih."},
				{"question": "Bagaimana cara upgrade paket?", "answer": "Buka menu Plan & Billing di dashboard, pilih paket tujuan, selesaikan pembayaran — fitur baru langsung aktif otomatis setelah pembayaran terverifikasi."},
				{"question": "Apakah bisa memakai domain sendiri?", "answer": "Bisa. Mulai paket Growth Anda dapat menyambungkan custom domain dengan sertifikat SSL yang di-provision otomatis."},
				{"question": "Apakah data bisnis saya aman?", "answer": "Setiap workspace terisolasi satu sama lain, akses anggota tim diatur berbasis role, dan seluruh koneksi dienkripsi dengan SSL."},
				{"question": "Bisakah berhenti berlangganan kapan saja?", "answer": "Bisa. Penghentian dijadwalkan di akhir periode berjalan — akses tetap aktif sampai periode berakhir dan data Anda tetap tersimpan."}
			]
		}'::jsonb
	),
	(
		'cta-section', 'cta', 'CTA Section', 90, '{}'::jsonb,
		'{
			"title": "Siap Membawa Bisnis Anda Online Hari Ini?",
			"description": "Daftar gratis, publish landing page pertama Anda dalam hitungan menit, dan upgrade saat bisnis Anda bertumbuh.",
			"primaryButtonText": "Daftar Gratis",
			"primaryUrl": "/auth/register",
			"secondaryButtonText": "Lihat Paket",
			"secondaryUrl": "#pricing"
		}'::jsonb
	),
	(
		'footer-section', 'footer', 'Footer Section', 100, '{}'::jsonb,
		'{
			"brandName": "Zyad Cloud",
			"description": "Platform SaaS all-in-one untuk membangun dan mengembangkan bisnis digital — landing page, CRM, automasi, dan banyak lagi dalam satu dashboard.",
			"columns": [
				{"title": "Produk", "links": [
					{"label": "Paket & Harga", "href": "#pricing"},
					{"label": "Modul & Fitur", "href": "#solusi"},
					{"label": "FAQ", "href": "#faq"}
				]},
				{"title": "Mulai", "links": [
					{"label": "Daftar Gratis", "href": "/auth/register"},
					{"label": "Masuk", "href": "/auth/login"}
				]}
			],
			"copyright": "© 2026 Zyad Cloud. All rights reserved."
		}'::jsonb
	)
)
INSERT INTO landing_page_sections (
	organization_id, landing_page_id, section_key, section_type, name, sort_order, style, content
)
SELECT
	lp.organization_id,
	lp.id,
	s.section_key,
	s.section_type,
	s.name,
	s.sort_order,
	s.style,
	s.content
FROM landing_page lp
CROSS JOIN sections s
ON CONFLICT (organization_id, landing_page_id, lower(section_key)) WHERE deleted_at IS NULL
DO UPDATE SET
	section_type = EXCLUDED.section_type,
	name = EXCLUDED.name,
	sort_order = EXCLUDED.sort_order,
	style = EXCLUDED.style,
	content = EXCLUDED.content,
	is_enabled = true,
	updated_at = now();

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
