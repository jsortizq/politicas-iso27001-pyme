-- 005_identity.up.sql
-- Identity: users sign in through OIDC and belong to one or more
-- organizations (one users row per organization).

-- ──────────────────────────────────────────────
-- current_tenant_id(): NULL when no tenant is set
-- ──────────────────────────────────────────────
-- The previous version raised an exception when app.tenant_id was unset,
-- which also aborted queries that are legitimately allowed by another
-- policy (the sign-in lookup below). Returning NULL is just as strict for
-- tenant data: `tenant_id = NULL` matches no row and fails every WITH CHECK.

CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS UUID AS $$
    SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid
$$ LANGUAGE sql STABLE;

-- ──────────────────────────────────────────────
-- Roles: add the read-only auditor
-- ──────────────────────────────────────────────

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('owner', 'admin', 'editor', 'auditor', 'viewer'));

-- E-mails are stored in lower case; make the existing rows consistent.
UPDATE users SET email = lower(email) WHERE email <> lower(email);
ALTER TABLE users ADD CONSTRAINT users_email_lowercase CHECK (email = lower(email));

-- One identity maps to at most one member of an organization.
CREATE UNIQUE INDEX idx_users_tenant_oidc_sub
    ON users (tenant_id, oidc_sub) WHERE oidc_sub IS NOT NULL;

-- ──────────────────────────────────────────────
-- Sign-in lookup
-- ──────────────────────────────────────────────
-- At sign-in the organization is not known yet, so the tenant policy cannot
-- apply. After the ID token has been verified, the API sets app.auth_sub
-- (and app.auth_email, only when the provider asserts the e-mail is
-- verified) for one transaction and may read exactly the rows of that
-- identity: memberships already bound to the subject, and pending
-- invitations addressed to the verified e-mail. Read-only: binding an
-- invitation goes through the tenant policy like any other write.

CREATE POLICY auth_lookup_users ON users FOR SELECT USING (
    (oidc_sub IS NOT NULL
        AND oidc_sub = NULLIF(current_setting('app.auth_sub', true), ''))
    OR
    (oidc_sub IS NULL
        AND email = NULLIF(current_setting('app.auth_email', true), ''))
);
