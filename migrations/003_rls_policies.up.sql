-- 003_rls_policies.up.sql
-- Row-Level Security policies for multi-tenant isolation (ADR-002)
--
-- Strategy: every tenant-scoped table gets RLS enabled.
-- The application sets current_setting('app.tenant_id') per connection.
-- All policies use this setting to filter rows.

-- ──────────────────────────────────────────────
-- Helper: current tenant ID from session variable
-- ──────────────────────────────────────────────

CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS UUID AS $$
BEGIN
    RETURN current_setting('app.tenant_id', true)::UUID;
EXCEPTION
    WHEN OTHERS THEN
        RAISE EXCEPTION 'app.tenant_id not set — multi-tenant context required';
END;
$$ LANGUAGE plpgsql STABLE;

-- ──────────────────────────────────────────────
-- Application role (non-superuser for RLS enforcement)
-- ──────────────────────────────────────────────

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user NOLOGIN;
    END IF;
END
$$;

-- Grant table access to app_user
GRANT USAGE ON SCHEMA public TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;

-- Ensure future tables also get grants
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO app_user;

-- ──────────────────────────────────────────────
-- Enable RLS on all tenant-scoped tables
-- ──────────────────────────────────────────────

-- Users
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_users ON users
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Audit log
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_audit_log ON audit_log
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- SoA entries
ALTER TABLE soa_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE soa_entries FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_soa ON soa_entries
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Risks
ALTER TABLE risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE risks FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_risks ON risks
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Risk controls (join table — check via risk's tenant)
ALTER TABLE risk_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_controls FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_risk_controls ON risk_controls
    USING (
        EXISTS (SELECT 1 FROM risks WHERE risks.id = risk_controls.risk_id AND risks.tenant_id = current_tenant_id())
    );

-- Documents
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_documents ON documents
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Document controls (join table)
ALTER TABLE document_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_controls FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_doc_controls ON document_controls
    USING (
        EXISTS (SELECT 1 FROM documents WHERE documents.id = document_controls.document_id AND documents.tenant_id = current_tenant_id())
    );

-- Audits
ALTER TABLE audits ENABLE ROW LEVEL SECURITY;
ALTER TABLE audits FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_audits ON audits
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Audit findings
ALTER TABLE audit_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_findings FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_findings ON audit_findings
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Management reviews
ALTER TABLE management_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE management_reviews FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_mgmt_reviews ON management_reviews
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- ──────────────────────────────────────────────
-- control_catalog is reference data: no RLS needed
-- tenants table: accessed by admin, not tenant-scoped
-- ──────────────────────────────────────────────
