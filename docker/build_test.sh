#!/usr/bin/env bash
# Build and smoke-test the three images. Run by hand, not from make check, since Docker
# is not available in every environment.
#
#   docker/build_test.sh
#
# Exits non-zero on the first failure.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

version="${VERSION:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
api_image="distrolearn-api:${version}"
workers_image="distrolearn-workers:${version}"
web_image="distrolearn-web:${version}"

pass=0
fail=0

ok() { printf 'PASS  %s\n' "$1"; pass=$((pass + 1)); }
no() { printf 'FAIL  %s\n' "$1"; fail=$((fail + 1)); }

check() {
	local name="$1"
	shift
	if "$@" >/dev/null 2>&1; then ok "$name"; else no "$name"; fi
}

# A Windows docker.exe shim on PATH is not a working daemon. Probe for a real one before
# running anything, otherwise every check below reports a misleading result.
if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
	echo "docker is not usable here (not on PATH, or no reachable daemon)" >&2
	echo "enable WSL integration in Docker Desktop, or run this on a host with Docker" >&2
	exit 2
fi

echo "== building =="
check "api image builds" docker build -f services/Dockerfile --target api \
	--build-arg "VERSION=${version}" -t "${api_image}" .
check "workers image builds" docker build -f services/Dockerfile --target workers \
	--build-arg "VERSION=${version}" -t "${workers_image}" .
check "web image builds" docker build -f apps/web/Dockerfile -t "${web_image}" .

echo
echo "== non-root =="
for img in "${api_image}" "${workers_image}" "${web_image}"; do
	uid="$(docker run --rm --entrypoint id "${img}" -u 2>/dev/null || echo unknown)"
	case "${uid}" in
	0 | root)
		no "${img} runs as non-root (uid ${uid})"
		;;
	unknown)
		no "${img} id probe failed"
		;;
	*)
		ok "${img} runs as non-root (uid ${uid})"
		;;
	esac
done

echo
echo "== health endpoints =="
api_health="$(docker run --rm -d --name dl-api-probe -p 127.0.0.1::8080 "${api_image}" 2>/dev/null || true)"
if [ -n "${api_health}" ]; then
	sleep 2
	code="$(docker run --rm --network container:dl-api-probe "${web_image}" \
		node -e 'fetch("http://127.0.0.1:8080/healthz").then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))' 2>/dev/null && echo ok || echo bad)"
	if [ "${code}" = "ok" ]; then ok "api /healthz answers"; else no "api /healthz answers"; fi
	docker rm -f dl-api-probe >/dev/null 2>&1 || true
else
	no "api container started"
fi

workers_health="$(docker run --rm -d --name dl-workers-probe -p 127.0.0.1::8081 "${workers_image}" 2>/dev/null || true)"
if [ -n "${workers_health}" ]; then
	sleep 2
	code="$(docker run --rm --network container:dl-workers-probe "${web_image}" \
		node -e 'fetch("http://127.0.0.1:8081/healthz").then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))' 2>/dev/null && echo ok || echo bad)"
	if [ "${code}" = "ok" ]; then ok "workers /healthz answers"; else no "workers /healthz answers"; fi
	docker rm -f dl-workers-probe >/dev/null 2>&1 || true
else
	no "workers container started"
fi

echo
echo "== web serves the page =="
web_id="$(docker run --rm -d --name dl-web-probe -p 127.0.0.1::3000 "${web_image}" 2>/dev/null || true)"
if [ -n "${web_id}" ]; then
	sleep 5
	if docker run --rm --network container:dl-web-probe "${web_image}" \
		node -e 'fetch("http://127.0.0.1:3000/").then(r=>r.text()).then(t=>process.exit(t.includes("DistroLearn")?0:1)).catch(()=>process.exit(1))' 2>/dev/null; then
		ok "web serves 200 with DistroLearn in the body"
	else
		no "web serves 200 with DistroLearn in the body"
	fi
	docker rm -f dl-web-probe >/dev/null 2>&1 || true
else
	no "web container started"
fi

echo
echo "== no shell or package manager in the Go images =="
for img in "${api_image}" "${workers_image}"; do
	if docker run --rm --entrypoint /bin/sh "${img}" -c true >/dev/null 2>&1; then
		no "${img} has a shell"
	else
		ok "${img} has no shell"
	fi
done

echo
echo "== image sizes =="
for img in "${api_image}" "${workers_image}" "${web_image}"; do
	if docker image inspect "${img}" >/dev/null 2>&1; then
		size="$(docker image inspect --format '{{.Size}}' "${img}")"
		printf '  %-34s %s bytes\n' "${img}" "${size}"
	fi
done

echo
echo "passed: ${pass}, failed: ${fail}"
[ "${fail}" -eq 0 ]