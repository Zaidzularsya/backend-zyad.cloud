-- Fase 3 isolir PPPoE: outbox tugas isolate/restore per PPP secret.
-- Constraint unik (organization_id, id) pada secrets dibutuhkan FK komposit dari tabel tugas.
ALTER TABLE mikrotik_ppp_secrets
	ADD CONSTRAINT mikrotik_ppp_secrets_org_id_unique UNIQUE (organization_id, id);

CREATE TABLE IF NOT EXISTS mikrotik_isolation_tasks (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	secret_id uuid NOT NULL,
	action varchar(10) NOT NULL,
	priority integer NOT NULL DEFAULT 0,
	status varchar(10) NOT NULL DEFAULT 'pending',
	attempts integer NOT NULL DEFAULT 0,
	next_attempt_at timestamp without time zone NOT NULL DEFAULT now(),
	locked_until timestamp without time zone,
	last_error text NOT NULL DEFAULT '',
	last_error_code varchar(40) NOT NULL DEFAULT '',
	tenant_notified_at timestamp without time zone,
	customer_notified_at timestamp without time zone,
	created_by_event varchar(40) NOT NULL DEFAULT '',
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	started_at timestamp without time zone,
	finished_at timestamp without time zone,
	CONSTRAINT mikrotik_isolation_tasks_action_check CHECK (action IN ('isolate', 'restore')),
	CONSTRAINT mikrotik_isolation_tasks_status_check CHECK (status IN ('pending', 'running', 'done', 'failed', 'cancelled')),
	CONSTRAINT fk_mikrotik_isolation_tasks_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_isolation_tasks_secret
		FOREIGN KEY (organization_id, secret_id) REFERENCES mikrotik_ppp_secrets(organization_id, id) ON DELETE CASCADE
);

-- Satu tugas aktif per secret; riwayat (done/failed/cancelled) boleh banyak.
CREATE UNIQUE INDEX IF NOT EXISTS uq_mikrotik_isolation_tasks_active_secret
	ON mikrotik_isolation_tasks(secret_id) WHERE status IN ('pending', 'running');

CREATE INDEX IF NOT EXISTS idx_mikrotik_isolation_tasks_queue
	ON mikrotik_isolation_tasks(organization_id, status, priority DESC, next_attempt_at);

SELECT apply_organization_rls('mikrotik_isolation_tasks'::regclass);
