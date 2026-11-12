-- 004_assets_and_soa_assessment.down.sql

GRANT UPDATE, DELETE ON tenants TO app_user;
GRANT INSERT, UPDATE, DELETE ON control_catalog TO app_user;
GRANT UPDATE, DELETE ON audit_log TO app_user;

DROP TABLE IF EXISTS assets;

ALTER TABLE soa_entries
    DROP COLUMN IF EXISTS review_date,
    DROP COLUMN IF EXISTS responsible_role,
    DROP COLUMN IF EXISTS effectiveness_evidence,
    DROP COLUMN IF EXISTS effectiveness_pct,
    DROP COLUMN IF EXISTS implementation_evidence;
