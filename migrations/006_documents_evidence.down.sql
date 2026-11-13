-- 006_documents_evidence.down.sql
-- Restores the shape of migration 002. Uploaded versions and evidence
-- records are dropped; their files remain in object storage.

DROP TABLE IF EXISTS evidence_controls;
DROP TABLE IF EXISTS evidences;
DROP TABLE IF EXISTS document_controls;
DROP TABLE IF EXISTS document_versions;
DROP FUNCTION IF EXISTS reject_file_mutation();

UPDATE documents SET doc_type = 'record' WHERE doc_type = 'other';
ALTER TABLE documents DROP CONSTRAINT documents_doc_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_doc_type_check
    CHECK (doc_type IN ('policy', 'procedure', 'record', 'evidence', 'template', 'report'));

ALTER TABLE documents
    DROP COLUMN created_by_email,
    DROP COLUMN next_review,
    DROP COLUMN owner,
    ADD COLUMN version     TEXT NOT NULL DEFAULT '1.0',
    ADD COLUMN storage_key TEXT,
    ADD COLUMN mime_type   TEXT,
    ADD COLUMN file_size   BIGINT,
    ADD COLUMN approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN approved_at TIMESTAMPTZ,
    ADD COLUMN metadata    JSONB NOT NULL DEFAULT '{}';

CREATE TABLE document_controls (
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    soa_entry_id UUID NOT NULL REFERENCES soa_entries(id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, soa_entry_id)
);

ALTER TABLE document_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_controls FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_doc_controls ON document_controls
    USING (
        EXISTS (SELECT 1 FROM documents WHERE documents.id = document_controls.document_id AND documents.tenant_id = current_tenant_id())
    );

GRANT SELECT, INSERT, UPDATE, DELETE ON document_controls TO app_user;
