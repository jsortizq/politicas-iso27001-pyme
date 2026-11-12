-- 004_assets_and_soa_assessment.up.sql
-- 1. Aligns soa_entries with the assessment fields exposed by the API.
-- 2. Adds the asset inventory (tenant-scoped, RLS-protected).
-- 3. Tightens the application role's privileges (least privilege).

-- ──────────────────────────────────────────────
-- SoA: evidence and effectiveness detail
-- ──────────────────────────────────────────────

ALTER TABLE soa_entries
    ADD COLUMN implementation_evidence TEXT NOT NULL DEFAULT '',
    ADD COLUMN effectiveness_pct       SMALLINT NOT NULL DEFAULT 0
        CHECK (effectiveness_pct BETWEEN 0 AND 100),
    ADD COLUMN effectiveness_evidence  TEXT NOT NULL DEFAULT '',
    ADD COLUMN responsible_role        TEXT NOT NULL DEFAULT '',
    ADD COLUMN review_date             DATE;

-- ──────────────────────────────────────────────
-- ASSET INVENTORY
-- ──────────────────────────────────────────────

CREATE TABLE assets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    asset_type      TEXT NOT NULL
        CHECK (asset_type IN ('information', 'software', 'hardware', 'service', 'people', 'facility')),
    description     TEXT NOT NULL DEFAULT '',
    owner           TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    classification  TEXT NOT NULL DEFAULT 'internal'
        CHECK (classification IN ('public', 'internal', 'confidential', 'restricted')),

    -- Valuation per security property (1 = very low … 5 = very high)
    confidentiality SMALLINT NOT NULL DEFAULT 1 CHECK (confidentiality BETWEEN 1 AND 5),
    integrity       SMALLINT NOT NULL DEFAULT 1 CHECK (integrity BETWEEN 1 AND 5),
    availability    SMALLINT NOT NULL DEFAULT 1 CHECK (availability BETWEEN 1 AND 5),
    criticality     SMALLINT GENERATED ALWAYS AS
        (GREATEST(confidentiality, integrity, availability)) STORED,

    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX idx_assets_tenant ON assets (tenant_id);
CREATE INDEX idx_assets_criticality ON assets (tenant_id, criticality DESC);

CREATE TRIGGER trg_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE assets FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_assets ON assets
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON assets TO app_user;

-- ──────────────────────────────────────────────
-- Least privilege for the application role
-- ──────────────────────────────────────────────

-- The audit trail is append-only from the application.
REVOKE UPDATE, DELETE ON audit_log FROM app_user;

-- Reference catalog is maintained by migrations only.
REVOKE INSERT, UPDATE, DELETE ON control_catalog FROM app_user;

-- Tenants are provisioned, never altered or removed, by the application.
REVOKE UPDATE, DELETE ON tenants FROM app_user;

-- Migration bookkeeping is not application data.
DO $$
BEGIN
    IF to_regclass('public.schema_migrations') IS NOT NULL THEN
        REVOKE ALL ON schema_migrations FROM app_user;
    END IF;
END
$$;
