#!/usr/bin/env bash
# Brings up the local stack and runs the ADR 0006 two-tenant pooled-connection test against
# a real PgBouncer in front of a real Postgres. Run by hand, not from make check, since
# Docker is not available in every environment.
#
#   cp .env.example .env
#   docker/compose_test.sh
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

if [ ! -f .env ]; then
	echo "no .env. Run: cp .env.example .env" >&2
	exit 2
fi
if ! docker info >/dev/null 2>&1; then
	echo "docker is not usable here" >&2
	exit 2
fi

pass=0
fail=0
ok() { printf 'PASS  %s\n' "$1"; pass=$((pass + 1)); }
no() { printf 'FAIL  %s\n' "$1"; fail=$((fail + 1)); }

echo "== up =="
docker compose up -d --wait --wait-timeout 300
docker compose ps

echo
echo "== every service healthy =="
unhealthy="$(docker compose ps --format '{{.Service}} {{.Health}}' | awk '$2 != "healthy" {print $1}')"
if [ -z "${unhealthy}" ]; then
	ok "all services report healthy"
else
	no "not healthy: ${unhealthy}"
fi

set -a
# shellcheck disable=SC1091
. ./.env
set +a

echo
echo "== pgbouncer admin credential is separate =="
# The app role must not reach SHOW CONFIG or SHUTDOWN. Distinct passwords are what makes that
# hold; separate userlist entries on one password would not.
userlist="$(docker compose exec -T pgbouncer cat /tmp/userlist.txt 2>/dev/null || true)"
app_pw="$(printf '%s\n' "${userlist}" | sed -n "s/^\"${POSTGRES_APP_USER:-app}\" \"\(.*\)\"\$/\1/p")"
admin_pw="$(printf '%s\n' "${userlist}" | sed -n 's/^"pgbouncer_admin" "\(.*\)"$/\1/p')"
if [ -n "${app_pw}" ] && [ -n "${admin_pw}" ] && [ "${app_pw}" != "${admin_pw}" ]; then
	ok "pgbouncer_admin has its own password"
else
	no "pgbouncer_admin password is missing or equals the app password"
fi

echo
echo "== pgbouncer pool_mode =="
# Read from the running pooler, not from the ini file, so this proves what loaded. The admin
# password, not the app one: pgbouncer_admin has its own credential.
mode="$(docker compose exec -T -e PGB_ADMIN_PW="${PGBOUNCER_ADMIN_PASSWORD:?PGBOUNCER_ADMIN_PASSWORD is not in .env}" postgres sh -c \
	"PGPASSWORD=\${PGB_ADMIN_PW} psql -h pgbouncer -p 6432 -U pgbouncer_admin -d pgbouncer -tAc 'SHOW CONFIG'" 2>/dev/null \
	| awk -F'|' '$1 ~ /pool_mode/ {gsub(/ /,"",$2); print $2}')"
echo "pool_mode = ${mode}"
if [ "${mode}" = "transaction" ]; then
	ok "pool_mode is transaction"
else
	no "pool_mode is ${mode:-unknown}, want transaction"
fi

echo
echo "== postgres version =="
pgver="$(docker compose exec -T postgres sh -c \
	"PGPASSWORD=\${POSTGRES_PASSWORD} psql -U \${POSTGRES_USER} -d \${POSTGRES_DB} -tAc 'SHOW server_version'" 2>/dev/null | tr -d '[:space:]')"
echo "server_version = ${pgver}"
case "${pgver}" in
18.6*) ok "postgres is 18.6" ;;
*) no "postgres is ${pgver}, want 18.6" ;;
esac

echo
echo "== tenant isolation, three variants (ADR 0006) =="
# Ephemeral host ports, discovered rather than hardcoded, so two stacks can run at once.
pgb_port="$(docker compose port pgbouncer 6432 | sed 's/.*://')"
pg_port="$(docker compose port postgres 5432 | sed 's/.*://')"

app_url="postgres://${POSTGRES_APP_USER:-app}:${POSTGRES_APP_PASSWORD}@127.0.0.1:${pgb_port}/${POSTGRES_DB:-distrolearn}?sslmode=disable"
admin_url="postgres://${POSTGRES_USER:-distrolearn}:${POSTGRES_PASSWORD}@127.0.0.1:${pg_port}/${POSTGRES_DB:-distrolearn}?sslmode=disable"

if (cd services && TEST_APP_DSN="${app_url}" TEST_ADMIN_DSN="${admin_url}" \
	go test ./internal/db/... -count=1 -run TestTenantIsolation -v 2>&1 | tee /tmp/compose_test_variants.log | grep -E '^(--- )?(PASS|FAIL|ok)'); then
	ok "all three tenant isolation variants passed"
else
	no "tenant isolation variants failed"
	grep -E 'tenant_isolation_test.go' /tmp/compose_test_variants.log | head -5 || true
fi

echo
echo "passed: ${pass}, failed: ${fail}"
[ "${fail}" -eq 0 ]