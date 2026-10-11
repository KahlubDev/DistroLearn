#!/bin/bash
# Runs on first initialisation of an empty Postgres volume. Creates the role named by
# POSTGRES_APP_USER that the services and PgBouncer authenticate as, so its password comes
# from the environment and never appears in a committed file.
set -euo pipefail

: "${POSTGRES_APP_USER:?POSTGRES_APP_USER must be set}"
: "${POSTGRES_APP_PASSWORD:?POSTGRES_APP_PASSWORD must be set}"

psql --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" \
	--set=ON_ERROR_STOP=1 \
	--set=app_user="${POSTGRES_APP_USER}" \
	--set=app_password="${POSTGRES_APP_PASSWORD}" \
	--set=dbname="${POSTGRES_DB}" <<'SQL'
-- \gexec renders the password through format %L, so quoting is the server's problem and
-- not this script's. NOBYPASSRLS matters: without it the app role would skip the policies
-- the tenant isolation test depends on.
SELECT format(
	'CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD %L',
	:'app_user', :'app_password'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec

-- Publish the role name to 00001_tenancy.sql, which grants to it. A per-database setting
-- rather than a session one, so it reaches the migration connection from the API, which is
-- a different session from this script. The setting holds a role name and no credential.
SELECT format('ALTER DATABASE %I SET "distrolearn.app_role" TO %L', :'dbname', :'app_user')
\gexec
SQL