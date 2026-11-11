-- 003_rls_policies.down.sql

-- Drop all RLS policies
DROP POLICY IF EXISTS tenant_isolation_mgmt_reviews ON management_reviews;
DROP POLICY IF EXISTS tenant_isolation_findings ON audit_findings;
DROP POLICY IF EXISTS tenant_isolation_audits ON audits;
DROP POLICY IF EXISTS tenant_isolation_doc_controls ON document_controls;
DROP POLICY IF EXISTS tenant_isolation_documents ON documents;
DROP POLICY IF EXISTS tenant_isolation_risk_controls ON risk_controls;
DROP POLICY IF EXISTS tenant_isolation_risks ON risks;
DROP POLICY IF EXISTS tenant_isolation_soa ON soa_entries;
DROP POLICY IF EXISTS tenant_isolation_audit_log ON audit_log;
DROP POLICY IF EXISTS tenant_isolation_users ON users;

-- Disable RLS
ALTER TABLE management_reviews DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_findings DISABLE ROW LEVEL SECURITY;
ALTER TABLE audits DISABLE ROW LEVEL SECURITY;
ALTER TABLE document_controls DISABLE ROW LEVEL SECURITY;
ALTER TABLE documents DISABLE ROW LEVEL SECURITY;
ALTER TABLE risk_controls DISABLE ROW LEVEL SECURITY;
ALTER TABLE risks DISABLE ROW LEVEL SECURITY;
ALTER TABLE soa_entries DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log DISABLE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;

-- Revoke grants
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE USAGE, SELECT ON SEQUENCES FROM app_user;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM app_user;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM app_user;
REVOKE USAGE ON SCHEMA public FROM app_user;

DROP ROLE IF EXISTS app_user;
DROP FUNCTION IF EXISTS current_tenant_id();
