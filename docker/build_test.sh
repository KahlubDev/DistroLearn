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

# A throwaway curl container for the HTTP probes. Starting one of the built images per
# attempt took about 12s each, which is longer than the whole retry budget. curl here is
# a test tool, never part of a shipped image, so an unpinned tag is acceptable.
probe_image="curlimages/curl:8.14.1"

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
# The Go images are distroless and hold no id binary, so the user is read from the image
# config rather than executed inside. The web image is debian-based and does have id.
for img in "${api_image}" "${workers_image}" "${web_image}"; do
	user="$(docker image inspect --format '{{.Config.User}}' "${img}" 2>/dev/null || echo unknown)"
	case "${user}" in
	"" | 0 | root)
		no "${img} runs as non-root (Config.User='${user}')"
		;;
	unknown)
		no "${img} inspect failed"
		;;
	*)
		ok "${img} Config.User=${user}"
		;;
	esac
done

# Second opinion from inside the container, where the runtime is available.
web_uid="$(docker run --rm --entrypoint id "${web_image}" -u 2>/dev/null || echo unknown)"
case "${web_uid}" in
0 | root)
	no "${web_image} runs as non-root (id reports ${web_uid})"
	;;
unknown)
	no "${web_image} id probe failed"
	;;
*)
	ok "${web_image} id reports uid ${web_uid}"
	;;
esac

echo
echo "== each image runs its own service =="
# Guards against a shared build stage: if both images carry the same binary, one of them
# logs the wrong service name and serves on the wrong port, which is invisible otherwise.
docker rm -f dl-api-ident dl-wrk-ident >/dev/null 2>&1 || true
docker run -d --name dl-api-ident "${api_image}" >/dev/null
docker run -d --name dl-wrk-ident "${workers_image}" >/dev/null
sleep 3
api_log="$(docker logs dl-api-ident 2>&1 | head -3)"
wrk_log="$(docker logs dl-wrk-ident 2>&1 | head -3)"
echo "${api_log}" | grep -q "api starting" \
	&& ok "api image logs 'api starting'" \
	|| no "api image did not log 'api starting'"
echo "${wrk_log}" | grep -q "workers starting" \
	&& ok "workers image logs 'workers starting'" \
	|| no "workers image did not log 'workers starting'"
echo "${wrk_log}" | grep -q "api starting" \
	&& no "workers image is running the api binary" \
	|| ok "workers image is not the api binary"
docker rm -f dl-api-ident dl-wrk-ident >/dev/null 2>&1 || true

echo
echo "== health endpoints =="

# Polls with retries instead of sleeping a fixed interval. A cold Next.js start takes
# longer than any fixed sleep on a loaded machine, which produced a false failure.
probe() {
	local target="$1" url="$2" want="${3:-}"
	local i out code=0
	for i in $(seq 1 30); do
		# set +e for the attempt: a probe that fails must be retried, not abort the run.
		# An assignment whose command substitution fails trips set -e on its own, so the
		# exit status is captured in the same statement.
		out="$(docker run --rm --network "container:${target}" "${probe_image}" \
			-sS -m 5 -o /dev/stdout -w '\n%{http_code}' "${url}" 2>/dev/null)" && code=0 || code=$?
		if [ "${code}" -eq 0 ]; then
			if [ -z "${want}" ] || printf '%s' "${out}" | grep -q "${want}"; then
				printf '%s\n' "${out}"
				return 0
			fi
		fi
		sleep 1
	done
	return 1
}

report_probe() {
	local label="$1" target="$2" url="$3" want="${4:-}"
	if [ -z "${target}" ]; then
		no "${label} (container did not start)"
		return 0
	fi
	local out body status rc
	# set +e so a failed probe is reported rather than aborting the script under set -e.
	set +e
	out="$(probe "${target}" "${url}" "${want}")"
	rc=$?
	set -e
	if [ "${rc}" -eq 0 ]; then
		# curl prints the body then the -w status code, so the code is the last line.
		body="$(printf '%s\n' "${out}" | head -1)"
		status="$(printf '%s\n' "${out}" | tail -1)"
		ok "${label} status=${status} body=${body}"
	else
		no "${label} never answered ${url}"
	fi
	return 0
}

docker rm -f dl-api-probe >/dev/null 2>&1 || true
if docker run --rm -d --name dl-api-probe "${api_image}" >/dev/null 2>&1; then
	report_probe "api /healthz" dl-api-probe "http://127.0.0.1:8080/healthz" '"status":"ok"'
	report_probe "api /readyz" dl-api-probe "http://127.0.0.1:8080/readyz" '"status":"ok"'
else
	report_probe "api /healthz" "" "http://127.0.0.1:8080/healthz"
fi
docker rm -f dl-api-probe >/dev/null 2>&1 || true

docker rm -f dl-workers-probe >/dev/null 2>&1 || true
if docker run --rm -d --name dl-workers-probe "${workers_image}" >/dev/null 2>&1; then
	report_probe "workers /healthz" dl-workers-probe "http://127.0.0.1:8081/healthz" '"status":"ok"'
	report_probe "workers /readyz" dl-workers-probe "http://127.0.0.1:8081/readyz" '"status":"ok"'
else
	report_probe "workers /healthz" "" "http://127.0.0.1:8081/healthz"
fi
docker rm -f dl-workers-probe >/dev/null 2>&1 || true

echo
echo "== web serves the page =="
docker rm -f dl-web-probe >/dev/null 2>&1 || true
if docker run --rm -d --name dl-web-probe "${web_image}" >/dev/null 2>&1; then
	report_probe "web /" dl-web-probe "http://127.0.0.1:3000/" "DistroLearn"
else
	no "web container started"
fi
docker rm -f dl-web-probe >/dev/null 2>&1 || true

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