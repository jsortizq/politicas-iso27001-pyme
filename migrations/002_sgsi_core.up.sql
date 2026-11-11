-- 002_sgsi_core.up.sql
-- Core SGSI tables: controls, assessments, SoA, documents
-- Implements ADR-004: three-dimension assessment model

-- ──────────────────────────────────────────────
-- CONTROL CATALOG (reference data, not tenant-scoped)
-- ──────────────────────────────────────────────

CREATE TABLE control_catalog (
    id          TEXT PRIMARY KEY,             -- e.g. "A.5.1"
    standard    TEXT NOT NULL DEFAULT 'ISO/IEC 27001:2022',
    category    TEXT NOT NULL,                -- A.5, A.6, A.7, A.8
    clause_en   TEXT NOT NULL,
    clause_es   TEXT NOT NULL,
    guidance    TEXT,                         -- implementation guidance
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ──────────────────────────────────────────────
-- STATEMENT OF APPLICABILITY (per tenant)
-- ──────────────────────────────────────────────

CREATE TABLE soa_entries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    control_id      TEXT NOT NULL REFERENCES control_catalog(id),
    applicable      BOOLEAN NOT NULL DEFAULT TRUE,
    justification   TEXT NOT NULL DEFAULT '',

    -- ADR-004: Three-dimension assessment
    -- Dimension 1: Applicability (applicable field above)
    -- Dimension 2: Implementation status
    implementation_status TEXT NOT NULL DEFAULT 'not_implemented'
        CHECK (implementation_status IN ('not_implemented', 'partial', 'implemented')),
    implementation_pct    SMALLINT NOT NULL DEFAULT 0 CHECK (implementation_pct BETWEEN 0 AND 100),

    -- Dimension 3: Effectiveness
    effectiveness         TEXT DEFAULT 'not_measured'
        CHECK (effectiveness IN ('not_measured', 'ineffective', 'partially_effective', 'effective')),
    last_effectiveness_review TIMESTAMPTZ,

    evidence_refs   TEXT[] NOT NULL DEFAULT '{}',    -- S3 object keys
    owner_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, control_id)
);

CREATE INDEX idx_soa_tenant ON soa_entries (tenant_id);
CREATE INDEX idx_soa_status ON soa_entries (tenant_id, implementation_status);

CREATE TRIGGER trg_soa_entries_updated_at
    BEFORE UPDATE ON soa_entries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ──────────────────────────────────────────────
-- RISK REGISTER
-- ──────────────────────────────────────────────

CREATE TABLE risks (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            TEXT NOT NULL,            -- e.g. "R-001"
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    asset           TEXT NOT NULL DEFAULT '',
    threat          TEXT NOT NULL DEFAULT '',
    vulnerability   TEXT NOT NULL DEFAULT '',

    -- Risk assessment (likelihood × impact = risk level)
    likelihood      SMALLINT NOT NULL DEFAULT 1 CHECK (likelihood BETWEEN 1 AND 5),
    impact          SMALLINT NOT NULL DEFAULT 1 CHECK (impact BETWEEN 1 AND 5),
    risk_level      SMALLINT GENERATED ALWAYS AS (likelihood * impact) STORED,

    treatment       TEXT NOT NULL DEFAULT 'accept'
        CHECK (treatment IN ('accept', 'mitigate', 'transfer', 'avoid')),
    treatment_plan  TEXT NOT NULL DEFAULT '',

    -- Residual risk after treatment
    residual_likelihood SMALLINT CHECK (residual_likelihood BETWEEN 1 AND 5),
    residual_impact     SMALLINT CHECK (residual_impact BETWEEN 1 AND 5),

    owner_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'in_treatment', 'accepted', 'closed')),
    review_date     DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE INDEX idx_risks_tenant ON risks (tenant_id);
CREATE INDEX idx_risks_level ON risks (tenant_id, risk_level DESC);

CREATE TRIGGER trg_risks_updated_at
    BEFORE UPDATE ON risks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Risk ↔ Control mapping
CREATE TABLE risk_controls (
    risk_id     UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    soa_entry_id UUID NOT NULL REFERENCES soa_entries(id) ON DELETE CASCADE,
    PRIMARY KEY (risk_id, soa_entry_id)
);

-- ──────────────────────────────────────────────
-- DOCUMENTS & EVIDENCE
-- ──────────────────────────────────────────────

CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    doc_type        TEXT NOT NULL CHECK (doc_type IN ('policy', 'procedure', 'record', 'evidence', 'template', 'report')),
    version         TEXT NOT NULL DEFAULT '1.0',
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'review', 'approved', 'obsolete')),
    storage_key     TEXT,                    -- S3 object key
    mime_type       TEXT,
    file_size       BIGINT,
    approved_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at     TIMESTAMPTZ,
    retention_until DATE,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_docs_tenant ON documents (tenant_id);
CREATE INDEX idx_docs_type ON documents (tenant_id, doc_type);
CREATE INDEX idx_docs_status ON documents (tenant_id, status);

CREATE TRIGGER trg_documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Document ↔ Control mapping
CREATE TABLE document_controls (
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    soa_entry_id UUID NOT NULL REFERENCES soa_entries(id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, soa_entry_id)
);

-- ──────────────────────────────────────────────
-- INTERNAL AUDITS
-- ──────────────────────────────────────────────

CREATE TABLE audits (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    audit_type      TEXT NOT NULL DEFAULT 'internal'
        CHECK (audit_type IN ('internal', 'external', 'surveillance')),
    status          TEXT NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'in_progress', 'completed', 'cancelled')),
    scope           TEXT NOT NULL DEFAULT '',
    lead_auditor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    planned_date    DATE,
    completed_date  DATE,
    summary         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audits_tenant ON audits (tenant_id);

CREATE TRIGGER trg_audits_updated_at
    BEFORE UPDATE ON audits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Audit findings (nonconformities)
CREATE TABLE audit_findings (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    audit_id        UUID NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    soa_entry_id    UUID REFERENCES soa_entries(id) ON DELETE SET NULL,
    severity        TEXT NOT NULL DEFAULT 'observation'
        CHECK (severity IN ('major_nc', 'minor_nc', 'observation', 'opportunity')),
    description     TEXT NOT NULL,
    corrective_action TEXT NOT NULL DEFAULT '',
    due_date        DATE,
    status          TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'in_progress', 'closed', 'overdue')),
    closed_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_findings_audit ON audit_findings (audit_id);
CREATE INDEX idx_findings_tenant ON audit_findings (tenant_id);
CREATE INDEX idx_findings_status ON audit_findings (tenant_id, status);

CREATE TRIGGER trg_findings_updated_at
    BEFORE UPDATE ON audit_findings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ──────────────────────────────────────────────
-- MANAGEMENT REVIEW
-- ──────────────────────────────────────────────

CREATE TABLE management_reviews (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    review_date     DATE NOT NULL,
    attendees       TEXT[] NOT NULL DEFAULT '{}',
    agenda          TEXT NOT NULL DEFAULT '',
    decisions       JSONB NOT NULL DEFAULT '[]',
    action_items    JSONB NOT NULL DEFAULT '[]',
    minutes_doc_id  UUID REFERENCES documents(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_mgmt_review_tenant ON management_reviews (tenant_id);

CREATE TRIGGER trg_mgmt_reviews_updated_at
    BEFORE UPDATE ON management_reviews
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
