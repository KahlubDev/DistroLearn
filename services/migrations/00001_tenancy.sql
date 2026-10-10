-- +goose Up
-- Schema for the ADR 0006 two-tenant isolation test, and no more.
--
-- Two tables only. `tenants` carries the `region` field ADR 0002 and ADR 0003 require on
-- every tenant. `labs` is the tenant-scoped table the RLS test reads, since system.md names
-- labs as the per-tenant record.

CREATE TABLE tenants (
    id         uuid PRIMARY KEY,
    name       text        NOT NULL,
    region     text        NOT NULL DEFAULT 'eu-central-1',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE labs (
    id         uuid PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX labs_tenant_id_idx ON labs (tenant_id);

-- RLS is a backstop, per ADR 0003. The policy reads the tenant from app.tenant_id, which
-- the application sets with set_config(..., is_local => true) inside every transaction.
--
-- NULLIF(..., '') matters twice over. When the setting is absent, current_setting with
-- missing_ok returns NULL rather than raising, so the comparison yields NULL and the row
-- is filtered out. That is the guarantee the test relies on: a transaction that forgets to
-- set the tenant reads nothing, instead of reading the previous tenant's rows.
ALTER TABLE labs ENABLE ROW LEVEL SECURITY;

CREATE POLICY labs_tenant_isolation ON labs
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- The application role is created by docker/initdb from POSTGRES_APP_USER, so its password
-- never appears in this file. It is deliberately not the owner and has NOBYPASSRLS, so RLS
-- applies to it.
--
-- Goose takes no bind parameters in SQL files, so the role name arrives through a database
-- setting rather than a literal. docker/initdb puts POSTGRES_APP_USER there on first init,
-- which means the grants below follow the same variable as the role creation, the service
-- DSNs, and the PgBouncer userlist. Without that setting the role is `app`, which is the
-- default everywhere else. GRANT takes an identifier and not an expression, hence format %I
-- and EXECUTE; %I quotes, so a role named "app; drop" is a name and not syntax.
--
-- +goose StatementBegin
DO $grants$
DECLARE
    app_role text := COALESCE(NULLIF(current_setting('distrolearn.app_role', true), ''), 'app');
BEGIN
    EXECUTE format('GRANT USAGE ON SCHEMA public TO %I', app_role);
    EXECUTE format('GRANT SELECT, INSERT ON labs TO %I', app_role);
    EXECUTE format('GRANT SELECT, INSERT ON tenants TO %I', app_role);
END
$grants$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS labs;
DROP TABLE IF EXISTS tenants;