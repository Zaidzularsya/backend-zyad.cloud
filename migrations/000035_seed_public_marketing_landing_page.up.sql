ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

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
landing_page_existing AS (
	SELECT id, organization_id FROM landing_pages 
	WHERE organization_id = (SELECT id FROM platform_org) 
	  AND slug = 'public-marketing' 
	LIMIT 1
),
landing_page_new AS (
	INSERT INTO landing_pages (
		organization_id, name, title, slug, page_type, status, visibility, is_homepage, published_version
	)
	SELECT
		id,
		'Platform Marketing Landing Page',
		'HEY Digital Solution - Partner Eksekusi Teknologi',
		'public-marketing',
		'homepage',
		'published',
		'public',
		true,
		1
	FROM platform_org
	WHERE NOT EXISTS (SELECT 1 FROM landing_page_existing)
	RETURNING id, organization_id
),
landing_page AS (
	SELECT id, organization_id FROM landing_page_existing
	UNION ALL
	SELECT id, organization_id FROM landing_page_new
),
sections_data (section_key, section_type, name, sort_order, content) AS (
	VALUES
		(
			'hero-section', 
			'hero', 
			'Hero Section', 
			10, 
			'{"badge": "", "titleHtml": "Punya Jaringan Klien? Jadikan HEY <span class=\"text-secondary\">Tim Teknis di Belakang Anda.</span>", "description": "Anda fokus mencari peluang dan bernegosiasi. Kami fokus memikirkan arsitektur, coding, dan memastikan aplikasi berjalan lancar. Mari kolaborasi tanpa repot merekrut programmer sendiri.", "primaryCta": "Saya Punya Calon Klien", "secondaryCta": "Saya Butuh Sistem", "primaryCtaUrl": "https://wa.me/6281223329453", "secondaryCtaUrl": "https://wa.me/6281223329453"}'::jsonb
		),
		(
			'problem-section', 
			'content', 
			'Problem Section', 
			20, 
			'{"title": "Banyak Peluang Project Digital, Tapi Tidak Semua Orang Punya Tim Teknis.", "description": "Membangun tim IT internal mahal dan berisiko. Menyerahkan ke freelancer lepas seringkali berakhir dengan kode yang berantakan.", "cards": [{"icon": "person_off", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container", "title": "Sulit Rekrut Programmer", "desc": "Mencari talent IT yang handal, bisa dipercaya, dan harganya masuk akal sangat memakan waktu."}, {"icon": "account_balance_wallet", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container", "title": "Biaya Overhead Tinggi", "desc": "Harus menggaji bulanan meski project sedang sepi. Beban operasional perusahaan meningkat tajam."}, {"icon": "gavel", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container", "title": "Risiko Project Gagal", "desc": "Salah estimasi waktu, salah pilih teknologi, ujung-ujungnya klien kecewa dan reputasi Anda hancur."}, {"icon": "trending_down", "bgClass": "bg-error-container/50", "iconClass": "text-on-error-container", "title": "Kehilangan Momentum", "desc": "Menolak project potensial karena merasa tidak sanggup mengeksekusi secara teknis."}]}'::jsonb
		),
		(
			'solution-section', 
			'services', 
			'Solution Section', 
			30, 
			'{"title": "HEY Menjadi Partner Eksekusi Digital di Belakang Anda.", "description": "Kami memposisikan diri sebagai ''dapur'' teknologi Anda. Bawa ide atau masalah klien ke kami, dan kami siapkan solusi end-to-end nya.", "ctaText": "Pelajari Model Kerjasama", "ctaUrl": "https://wa.me/6281223329453", "steps": [{"number": 1, "title": "Konsultasi & Booking Meeting", "desc": "Diskusi awal via CS HEY dan finalisasi kebutuhan lewat meeting bersama tim teknis kami.", "highlight": true}, {"number": 2, "title": "Quotation & Kesepakatan Deal", "desc": "Penerbitan penawaran, invoice, serta persetujuan Syarat & Ketentuan yang berlaku.", "highlight": false}, {"number": 3, "title": "Development & Reporting", "desc": "Proses coding dan pengerjaan aplikasi beserta laporan progress rutin secara transparan.", "highlight": false}, {"number": 4, "title": "Pelunasan & Deployment", "desc": "Pembayaran lunas dari klien yang dilanjutkan dengan serah terima (deployment) aplikasi.", "highlight": false}]}'::jsonb
		),
		(
			'services-section', 
			'features', 
			'Services Section', 
			40, 
			'{"title": "Solusi Digital yang Bisa Anda Tawarkan ke Calon Klien", "description": "Berbekal pengalaman teknis kami, Anda bisa dengan percaya diri menawarkan berbagai solusi kompleks ini.", "services": [{"icon": "language", "title": "Website & Company Profile", "desc": "Dari landing page cepat hingga corporate portal dengan CMS custom yang aman."}, {"icon": "dashboard", "title": "Custom Dashboard", "desc": "Sistem manajemen internal, ERP mini, atau admin panel khusus untuk operasional spesifik."}, {"icon": "how_to_reg", "title": "Sistem Registrasi & Tiketing", "desc": "Portal pendaftaran event, membership, atau sistem reservasi yang stabil menangani traffic tinggi."}, {"icon": "chat", "title": "WhatsApp Automation", "desc": "Integrasi API WhatsApp untuk notifikasi otomatis, chatbot, dan broadcast ke pelanggan."}, {"icon": "smart_toy", "title": "AI Admin / Bot Custom", "desc": "Implementasi AI untuk memproses data, menjawab FAQ kompleks, atau otomatisasi tugas repetitif."}, {"icon": "monitoring", "title": "Reporting & Data Viz", "desc": "Menyajikan data mentah menjadi grafik interaktif dan laporan otomatis yang mudah dibaca manajemen."}]}'::jsonb
		),
		(
			'benefits-section', 
			'features', 
			'Benefits Section', 
			50, 
			'{"title": "Keuntungan Menjadi Partner HEY", "description": "Kolaborasi saling menguntungkan. Anda bawa project, kami sediakan dukungan penuh dari awal hingga selesai.", "benefits": [{"icon": "dashboard_customize", "title": "Dashboard Partner", "desc": "Akses portal khusus untuk memantau fee project, mendaftarkan lead, melihat kalender progres, dan info aturan kerja."}, {"icon": "payments", "title": "Project Fee", "desc": "Dapatkan fee dari project yang berhasil deal."}, {"icon": "code_off", "title": "Technical Backup", "desc": "Tidak perlu bisa coding. HEY bantu bagian teknis."}, {"icon": "description", "title": "Proposal Support", "desc": "Kami bantu susun solusi, estimasi, dan materi penawaran."}, {"icon": "present_to_all", "title": "Demo Material", "desc": "Partner bisa membawa contoh solusi untuk menjelaskan ke calon klien."}, {"icon": "groups", "title": "Meeting Support", "desc": "HEY bisa ikut membantu sesi diskusi kebutuhan dengan calon klien."}, {"icon": "all_inclusive", "title": "Long-Term Collaboration", "desc": "Partner bisa membawa peluang project berikutnya secara berulang."}]}'::jsonb
		),
		(
			'demo-section', 
			'portfolio', 
			'Demo Portfolio', 
			60, 
			'{"title": "Jelajahi Demo SaaS Kami", "description": "Berikan calon klien Anda gambaran nyata bagaimana sistem kami bekerja. Demo interaktif ini menunjukkan kapabilitas arsitektur dan kualitas desain yang bisa Anda tawarkan.", "features": [{"id": "dashboard", "title": "Admin Dashboard", "desc": "Pantau metrik, kelola pengguna, dan atur operasional bisnis dari satu tempat.", "icon": "dashboard"}, {"id": "ticketing", "title": "Ticketing System", "desc": "Kelola event, jual tiket, dan check-in barcode dalam hitungan detik.", "icon": "confirmation_number"}, {"id": "crm", "title": "CRM & Sales", "desc": "Lacak prospek, kelola pipeline, dan jangan lewatkan satu deal pun.", "icon": "trending_up"}]}'::jsonb
		),
		(
			'faq-section', 
			'faq', 
			'FAQ Section', 
			70, 
			'{"title": "FAQ", "items": [{"question": "Apakah partner harus bisa coding?", "answer": "Tidak. Partner cukup membawa peluang dan membantu komunikasi bisnis. Tim HEY membantu bagian teknis."}, {"question": "Apakah HEY sudah punya banyak client?", "answer": "HEY masih tahap awal. Karena itu kami fokus pada project yang jelas, realistis, dan bisa dikerjakan dengan rapi."}, {"question": "Berapa fee partner?", "answer": "Fee disepakati berdasarkan nilai project dan kontribusi partner. Skema awal bisa berupa persentase project atau referral fee."}, {"question": "Apakah HEY bisa ikut meeting dengan calon klien?", "answer": "Bisa. HEY dapat membantu sesi discovery, menjelaskan solusi, dan menyusun proposal teknis."}, {"question": "Project seperti apa yang cocok?", "answer": "Project kecil-menengah seperti landing page, dashboard admin, sistem registrasi, otomasi WhatsApp, membership, invoice tracking, dan laporan operasional."}, {"question": "Kapan fee partner dibayarkan?", "answer": "Fee dibayarkan setelah pembayaran dari klien masuk sesuai kesepakatan project."}]}'::jsonb
		),
		(
			'cta-section', 
			'cta', 
			'CTA Section', 
			80, 
			'{"title": "Punya Calon Klien yang Butuh Solusi Digital?", "description": "Jangan lewatkan peluang hanya karena kendala teknis. Mari diskusi santai, ceritakan masalah klien Anda, dan kita cari solusi teknisnya bersama.", "primaryButtonText": "Konsultasi Ide Project", "secondaryButtonText": "Konsultasi Project Saya", "primaryUrl": "https://wa.me/6281223329453", "secondaryUrl": "https://wa.me/6281223329453"}'::jsonb
		),
		(
			'footer-section', 
			'footer', 
			'Footer Section', 
			90, 
			'{"brandName": "HEY Digital Solution", "description": "Partner eksekusi teknologi terpercaya untuk B2B. Membangun fondasi digital yang kuat, aman, dan scalable untuk inovasi bisnis Anda.", "columns": [{"title": "Perusahaan", "links": [{"label": "Tentang Kami", "href": "#"}, {"label": "Solusi IT", "href": "#"}, {"label": "Program Reseller", "href": "#"}]}, {"title": "Legal & Resource", "links": [{"label": "Dokumentasi API", "href": "#"}, {"label": "Kebijakan Privasi", "href": "#"}, {"label": "SaaS Demo Portal", "href": "#demo-portal"}]}], "copyright": "© 2024 HEY Digital Solution. All rights reserved."}'::jsonb
		)
)
INSERT INTO landing_page_sections (
	organization_id, landing_page_id, section_key, section_type, name, sort_order, content
)
SELECT 
	lp.organization_id,
	lp.id,
	s.section_key,
	s.section_type,
	s.name,
	s.sort_order,
	s.content
FROM landing_page lp
CROSS JOIN sections_data s
ON CONFLICT (organization_id, landing_page_id, lower(section_key)) WHERE deleted_at IS NULL
DO UPDATE SET
	content = EXCLUDED.content,
	sort_order = EXCLUDED.sort_order,
	updated_at = now();

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
