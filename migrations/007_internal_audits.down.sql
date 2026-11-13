-- 007_internal_audits.down.sql

DROP INDEX IF EXISTS idx_findings_control;

ALTER TABLE audit_findings DROP CONSTRAINT IF EXISTS audit_findings_closed_consistent;
UPDATE audit_findings SET status = 'in_progress' WHERE status = 'implemented';
ALTER TABLE audit_findings DROP CONSTRAINT audit_findings_status_check;
ALTER TABLE audit_findings ADD CONSTRAINT audit_findings_status_check
    CHECK (status IN ('open', 'in_progress', 'closed', 'overdue'));

ALTER TABLE audit_findings
    DROP CONSTRAINT IF EXISTS audit_findings_audit_number_key,
    DROP CONSTRAINT IF EXISTS audit_findings_number_positive,
    DROP COLUMN created_by_email,
    DROP COLUMN closed_by_email,
    DROP COLUMN reopen_note,
    DROP COLUMN verification,
    DROP COLUMN responsible,
    DROP COLUMN root_cause,
    DROP COLUMN evidence,
    DROP COLUMN clause,
    DROP COLUMN control_id,
    DROP COLUMN number,
    ADD COLUMN soa_entry_id UUID REFERENCES soa_entries(id) ON DELETE SET NULL;

DROP TABLE IF EXISTS audit_controls;

ALTER TABLE audits
    DROP COLUMN created_by_email,
    DROP COLUMN cancel_reason,
    DROP COLUMN lead_auditor,
    DROP COLUMN criteria;
