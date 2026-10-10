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

-- The application role is `app`, created by docker/initdb from an environment variable so
-- no password lives in this file. It is deliberately not the owner and has NOBYPASSRLS, so
-- RLS applies to it.
GRANT USAGE ON SCHEMA public TO app;
GRANT SELECT, INSERT ON labs TO app;
GRANT SELECT, INSERT ON tenants TO app;

-- +goose Down
DROP TABLE IF EXISTS labs;
DROP TABLE IF EXISTS tenants;