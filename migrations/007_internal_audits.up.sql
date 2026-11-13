-- 007_internal_audits.up.sql
-- Internal audits (ISO/IEC 27001 clause 9.2) and the follow-up of their
-- findings through corrective action (clause 10.2).
--
-- The audits and audit_findings tables exist since migration 002 but no
-- API ever wrote to them. They are extended here; existing rows, if any,
-- are kept and brought to the new shape.

-- ──────────────────────────────────────────────
-- AUDITS
-- ──────────────────────────────────────────────

ALTER TABLE audits
    ADD COLUMN criteria         TEXT NOT NULL DEFAULT '',   -- what the audit checks against
    ADD COLUMN lead_auditor     TEXT NOT NULL DEFAULT '',   -- may be external, so free text
    ADD COLUMN cancel_reason    TEXT NOT NULL DEFAULT '',
    ADD COLUMN created_by_email TEXT NOT NULL DEFAULT '';

-- Controls within the scope of an audit.
CREATE TABLE audit_controls (
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    audit_id   UUID NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    control_id TEXT NOT NULL REFERENCES control_catalog(id),
    PRIMARY KEY (audit_id, control_id)
);

CREATE INDEX idx_audit_controls_control ON audit_controls (tenant_id, control_id);

ALTER TABLE audit_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_controls FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_audit_controls ON audit_controls
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON audit_controls TO app_user;

-- ──────────────────────────────────────────────
-- FINDINGS AND CORRECTIVE ACTION
-- ──────────────────────────────────────────────

ALTER TABLE audit_findings
    DROP COLUMN soa_entry_id,
    ADD COLUMN number           INTEGER,                    -- 1, 2, 3… within the audit
    ADD COLUMN control_id       TEXT REFERENCES control_catalog(id),
    ADD COLUMN clause           TEXT NOT NULL DEFAULT '',   -- e.g. "7.5.3" for clauses 4–10
    ADD COLUMN evidence         TEXT NOT NULL DEFAULT '',   -- objective evidence observed
    ADD COLUMN root_cause       TEXT NOT NULL DEFAULT '',
    ADD COLUMN responsible      TEXT NOT NULL DEFAULT '',
    ADD COLUMN verification     TEXT NOT NULL DEFAULT '',   -- how effectiveness was verified
    ADD COLUMN reopen_note      TEXT NOT NULL DEFAULT '',
    ADD COLUMN closed_by_email  TEXT NOT NULL DEFAULT '',
    ADD COLUMN created_by_email TEXT NOT NULL DEFAULT '';

UPDATE audit_findings f SET number = n.rn
FROM (
    SELECT id, row_number() OVER (PARTITION BY audit_id ORDER BY created_at, id) AS rn
    FROM audit_findings
) n
WHERE n.id = f.id;

ALTER TABLE audit_findings
    ALTER COLUMN number SET NOT NULL,
    ADD CONSTRAINT audit_findings_number_positive CHECK (number >= 1),
    ADD CONSTRAINT audit_findings_audit_number_key UNIQUE (audit_id, number);

-- "overdue" is a consequence of the due date, not a state to store.
UPDATE audit_findings SET status = 'open' WHERE status = 'overdue';
ALTER TABLE audit_findings DROP CONSTRAINT audit_findings_status_check;
ALTER TABLE audit_findings ADD CONSTRAINT audit_findings_status_check
    CHECK (status IN ('open', 'in_progress', 'implemented', 'closed'));

-- A closed finding always records when it was closed.
UPDATE audit_findings SET closed_at = COALESCE(closed_at, updated_at) WHERE status = 'closed';
UPDATE audit_findings SET closed_at = NULL WHERE status <> 'closed';
ALTER TABLE audit_findings ADD CONSTRAINT audit_findings_closed_consistent
    CHECK ((status = 'closed') = (closed_at IS NOT NULL));

CREATE INDEX idx_findings_control ON audit_findings (tenant_id, control_id) WHERE control_id IS NOT NULL;
