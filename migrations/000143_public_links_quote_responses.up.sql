-- Directory token → dokumen. TIDAK memakai RLS: pencarian token terjadi
-- sebelum tenant diketahui (pola mailbox_directory, 000135). Semua operasi
-- ber-scope tetap memfilter organization_id secara eksplisit di repository.
CREATE TABLE IF NOT EXISTS public_links (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	document_type varchar(30) NOT NULL,
	document_id uuid NOT NULL,
	token_hash bytea NOT NULL,
	token_enc text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	revoked_at timestamp without time zone NULL,
	last_viewed_at timestamp without time zone NULL,
	view_count integer NOT NULL DEFAULT 0,
	created_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT public_links_document_type_check CHECK (document_type IN ('quotation', 'invoice'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_public_links_token_hash ON public_links(token_hash);
CREATE UNIQUE INDEX IF NOT EXISTS idx_public_links_active_document
	ON public_links(organization_id, document_type, document_id) WHERE revoked_at IS NULL;
