-- 005_identity.down.sql

DROP POLICY IF EXISTS auth_lookup_users ON users;
DROP INDEX IF EXISTS idx_users_tenant_oidc_sub;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_lowercase;

UPDATE users SET role = 'viewer' WHERE role = 'auditor';
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('owner', 'admin', 'editor', 'viewer'));

CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS UUID AS $$
BEGIN
    RETURN current_setting('app.tenant_id', true)::UUID;
EXCEPTION
    WHEN OTHERS THEN
        RAISE EXCEPTION 'app.tenant_id not set — multi-tenant context required';
END;
$$ LANGUAGE plpgsql STABLE;
