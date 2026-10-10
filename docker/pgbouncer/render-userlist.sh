#!/bin/sh
# Writes the PgBouncer userlist and config at runtime, then execs PgBouncer directly.
#
# The image's own entrypoint derives the userlist from DATABASE_URL, but it stores an MD5
# hash. auth_type = scram-sha-256 cannot use an MD5 secret: PgBouncer would have no way to
# verify a client's SCRAM response, and none to authenticate to Postgres. So the userlist is
# written here as plaintext, which serves both the client-facing SCRAM exchange and the
# backend SCRAM exchange with Postgres.
#
# The config is rendered from pgbouncer.ini.template for the same reason. PgBouncer reads no
# environment variables, so [databases] has to name POSTGRES_DB literally in whatever file
# it is started with.
#
# Plaintext never reaches the repository: it comes from the environment, and the files are
# created with mode 0600 inside container-local /tmp.
set -eu

: "${PGBOUNCER_APP_USER:?PGBOUNCER_APP_USER must be set}"
: "${PGBOUNCER_APP_PASSWORD:?PGBOUNCER_APP_PASSWORD must be set}"
: "${PGBOUNCER_ADMIN_PASSWORD:?PGBOUNCER_ADMIN_PASSWORD must be set}"
: "${PGBOUNCER_DATABASE:?PGBOUNCER_DATABASE must be set}"

USERLIST="${AUTH_FILE:-/tmp/userlist.txt}"
CONFIG="${PGBOUNCER_CONFIG:-/tmp/pgbouncer.ini}"
TEMPLATE="${PGBOUNCER_TEMPLATE:-/etc/pgbouncer/pgbouncer.ini.template}"

umask 077
{
	printf '"%s" "%s"\n' "${PGBOUNCER_APP_USER}" "${PGBOUNCER_APP_PASSWORD}"
	# A separate admin entry for SHOW CONFIG and SHUTDOWN, so the role the services use is
	# not the role that can administer the pooler. Its own password, so holding the app
	# credential does not grant the pooler console. Local compose only; the admin user is
	# not a Postgres role and exists nowhere in Postgres.
	printf '"pgbouncer_admin" "%s"\n' "${PGBOUNCER_ADMIN_PASSWORD}"
} > "${USERLIST}"

# The database name appears twice: as the alias clients connect to, and as the target.
sed "s|@@DATABASE@@|${PGBOUNCER_DATABASE}|g" "${TEMPLATE}" > "${CONFIG}"

exec /usr/bin/pgbouncer "${CONFIG}"