-- 002_sgsi_core.down.sql
DROP TRIGGER IF EXISTS trg_mgmt_reviews_updated_at ON management_reviews;
DROP TRIGGER IF EXISTS trg_findings_updated_at ON audit_findings;
DROP TRIGGER IF EXISTS trg_audits_updated_at ON audits;
DROP TRIGGER IF EXISTS trg_documents_updated_at ON documents;
DROP TRIGGER IF EXISTS trg_risks_updated_at ON risks;
DROP TRIGGER IF EXISTS trg_soa_entries_updated_at ON soa_entries;

DROP TABLE IF EXISTS management_reviews;
DROP TABLE IF EXISTS audit_findings;
DROP TABLE IF EXISTS audits;
DROP TABLE IF EXISTS document_controls;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS risk_controls;
DROP TABLE IF EXISTS risks;
DROP TABLE IF EXISTS soa_entries;
DROP TABLE IF EXISTS control_catalog;
