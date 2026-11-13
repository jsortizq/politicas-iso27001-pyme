-- 006_documents_evidence.up.sql
-- Controlled documents with immutable versions and an approval trail
-- (ISO/IEC 27001 clause 7.5), and evidence files linked to controls.
--
-- The documents table from migration 002 kept a single file per document.
-- No API ever wrote to it, so it is reshaped here: files move to
-- document_versions and links point to catalog controls.

-- ──────────────────────────────────────────────
-- DOCUMENTS
-- ──────────────────────────────────────────────

DROP TABLE document_controls;

ALTER TABLE documents
    DROP COLUMN version,
    DROP COLUMN storage_key,
    DROP COLUMN mime_type,
    DROP COLUMN file_size,
    DROP COLUMN approved_by,
    DROP COLUMN approved_at,
    DROP COLUMN metadata,
    ADD COLUMN owner            TEXT NOT NULL DEFAULT '',
    ADD COLUMN next_review      DATE,
    ADD COLUMN created_by_email TEXT NOT NULL DEFAULT '';

-- Evidence has its own table now.
DELETE FROM documents WHERE doc_type = 'evidence';
ALTER TABLE documents DROP CONSTRAINT documents_doc_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_doc_type_check
    CHECK (doc_type IN ('policy', 'procedure', 'record', 'template', 'report', 'other'));

-- Each uploaded file is a version. Versions are never overwritten: a
-- change is a new version, and the previous ones stay downloadable.
CREATE TABLE document_versions (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_id       UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    number            INTEGER NOT NULL CHECK (number >= 1),
    status            TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'in_review', 'approved', 'rejected', 'superseded')),
    change_summary    TEXT NOT NULL DEFAULT '',

    file_name         TEXT NOT NULL,
    mime_type         TEXT NOT NULL DEFAULT 'application/octet-stream',
    file_size         BIGINT NOT NULL CHECK (file_size >= 0),
    sha256            CHAR(64) NOT NULL,
    storage_key       TEXT NOT NULL UNIQUE,

    uploaded_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    uploaded_by_email TEXT NOT NULL DEFAULT '',   -- kept if the member is removed
    uploaded_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    reviewed_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by_email TEXT NOT NULL DEFAULT '',
    reviewed_at       TIMESTAMPTZ,
    review_note       TEXT NOT NULL DEFAULT '',

    UNIQUE (document_id, number)
);

CREATE INDEX idx_doc_versions_document ON document_versions (document_id, number DESC);
CREATE INDEX idx_doc_versions_tenant ON document_versions (tenant_id);

-- A document has at most one version in force.
CREATE UNIQUE INDEX idx_doc_versions_one_approved
    ON document_versions (document_id) WHERE status = 'approved';

CREATE TABLE document_controls (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    control_id  TEXT NOT NULL REFERENCES control_catalog(id),
    PRIMARY KEY (document_id, control_id)
);

CREATE INDEX idx_document_controls_control ON document_controls (tenant_id, control_id);

-- ──────────────────────────────────────────────
-- EVIDENCE
-- ──────────────────────────────────────────────

CREATE TABLE evidences (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    collected_on      DATE NOT NULL DEFAULT CURRENT_DATE,  -- when the evidence was obtained
    valid_until       DATE,                                -- after this date it must be renewed

    file_name         TEXT NOT NULL,
    mime_type         TEXT NOT NULL DEFAULT 'application/octet-stream',
    file_size         BIGINT NOT NULL CHECK (file_size >= 0),
    sha256            CHAR(64) NOT NULL,
    storage_key       TEXT NOT NULL UNIQUE,

    uploaded_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    uploaded_by_email TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_evidences_tenant ON evidences (tenant_id, created_at DESC);

CREATE TRIGGER trg_evidences_updated_at
    BEFORE UPDATE ON evidences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE evidence_controls (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    evidence_id UUID NOT NULL REFERENCES evidences(id) ON DELETE CASCADE,
    control_id  TEXT NOT NULL REFERENCES control_catalog(id),
    PRIMARY KEY (evidence_id, control_id)
);

CREATE INDEX idx_evidence_controls_control ON evidence_controls (tenant_id, control_id);

-- ──────────────────────────────────────────────
-- Stored files are immutable
-- ──────────────────────────────────────────────
-- The file behind a version or an evidence record can never be swapped:
-- the database rejects any change to what identifies it.

CREATE OR REPLACE FUNCTION reject_file_mutation() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.sha256 IS DISTINCT FROM OLD.sha256
        OR NEW.storage_key IS DISTINCT FROM OLD.storage_key
        OR NEW.file_size IS DISTINCT FROM OLD.file_size
        OR NEW.file_name IS DISTINCT FROM OLD.file_name THEN
        RAISE EXCEPTION 'stored files are immutable; upload a new version instead'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_doc_versions_immutable_file
    BEFORE UPDATE ON document_versions
    FOR EACH ROW EXECUTE FUNCTION reject_file_mutation();

CREATE TRIGGER trg_evidences_immutable_file
    BEFORE UPDATE ON evidences
    FOR EACH ROW EXECUTE FUNCTION reject_file_mutation();

-- ──────────────────────────────────────────────
-- Row-Level Security
-- ──────────────────────────────────────────────

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['document_versions', 'document_controls', 'evidences', 'evidence_controls']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation_%s ON %I
                USING (tenant_id = current_tenant_id())
                WITH CHECK (tenant_id = current_tenant_id())', t, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO app_user', t);
    END LOOP;
END
$$;
