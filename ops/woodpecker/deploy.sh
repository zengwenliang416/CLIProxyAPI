#!/usr/bin/env bash
set -Eeuo pipefail

root=/opt/cliproxyapi
compose_file="${root}/docker-compose.yml"
env_file="${root}/deploy.env"
state_file="${root}/state/cpa-key-policy-state.json"
container=cliproxyapi
network=new-api-network
sha="${CI_COMMIT_SHA:-}"
short_sha="${sha:0:12}"
image="local/cliproxyapi:${sha}"
compose_project="cliproxyapi-${short_sha}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="${root}/backups/${timestamp}-${short_sha}"
rollback_container="${container}-rollback-${timestamp}"
canary_container="${container}-canary-${short_sha}"
old_container_present=false
state_file_in_old_container=false
cutover_started=false
old_container_renamed=false
deployment_succeeded=false

log() {
  printf '[cliproxy-deploy] %s\n' "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "required file is missing: $1"
}

cleanup_canary() {
  docker rm -f "${canary_container}" >/dev/null 2>&1 || true
}

require_bind_mount() {
  local source=$1
  local destination=$2

  docker inspect -f \
    '{{range .Mounts}}{{if eq .Type "bind"}}{{printf "%s|%s\n" .Source .Destination}}{{end}}{{end}}' \
    "${container}" | grep -Fqx "${source}|${destination}"
}

rollback() {
  local exit_code=$?
  trap - EXIT INT TERM
  cleanup_canary
  rm -f "${state_file}.new"

  if [[ "${deployment_succeeded}" != true && "${cutover_started}" == true ]]; then
    log "deployment failed; restoring the previous container"

    if [[ "${old_container_renamed}" == true ]]; then
      docker rm -f "${container}" >/dev/null 2>&1 || true
      docker rename "${rollback_container}" "${container}" >/dev/null
      docker start "${container}" >/dev/null
    elif [[ "${old_container_present}" == true ]]; then
      docker start "${container}" >/dev/null
    else
      docker rm -f "${container}" >/dev/null 2>&1 || true
    fi
  fi

  exit "${exit_code}"
}

trap rollback EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

[[ -n "${sha}" ]] || fail "CI_COMMIT_SHA is required"
[[ "${sha}" =~ ^[0-9a-f]{40}$ ]] || fail "CI_COMMIT_SHA must be a full Git SHA"
require_file Dockerfile
require_file config.example.yaml
require_file ops/production/docker-compose.yml
require_file "${root}/config.yaml"
[[ -d "${root}/auth" ]] || fail "auth directory is missing"
[[ -d "${root}/plugins" ]] || fail "plugins directory is missing"
docker network inspect "${network}" >/dev/null

auth_count_before="$(find "${root}/auth" -maxdepth 1 -type f | wc -l | tr -d ' ')"
plugin_count_before="$(find "${root}/plugins" -type f | wc -l | tr -d ' ')"
[[ "${auth_count_before}" -gt 0 ]] || fail "no authentication files were found"

log "building ${image}"
docker build \
  --build-arg "VERSION=ci-${short_sha}" \
  --build-arg "COMMIT=${short_sha}" \
  --build-arg "BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --tag "${image}" \
  .

cleanup_canary
log "starting isolated canary"
docker run -d \
  --name "${canary_container}" \
  --network "${network}" \
  "${image}" \
  ./CLIProxyAPI -config /CLIProxyAPI/config.example.yaml -local-model >/dev/null

canary_healthy=false
for _ in $(seq 1 30); do
  if [[ "$(docker inspect -f '{{.State.Running}}' "${canary_container}")" != true ]]; then
    docker logs "${canary_container}" >&2 || true
    fail "canary exited before becoming healthy"
  fi

  if docker run --rm --network "${network}" curlimages/curl:8.16.0 \
    --fail --silent --show-error --max-time 3 \
    "http://${canary_container}:8317/healthz" >/dev/null; then
    canary_healthy=true
    break
  fi
  sleep 1
done
[[ "${canary_healthy}" == true ]] || fail "canary executable check failed"
cleanup_canary

mkdir -p "${backup_dir}" "${root}/state" "${root}/logs"
chmod 700 "${root}/backups" "${backup_dir}" "${root}/state"

if docker container inspect "${container}" >/dev/null 2>&1; then
  old_container_present=true
  if docker exec "${container}" test -f /CLIProxyAPI/cpa-key-policy-state.json; then
    state_file_in_old_container=true
  fi
  docker inspect "${container}" >"${backup_dir}/container-inspect.json"
  docker image inspect "$(docker inspect -f '{{.Image}}' "${container}")" >"${backup_dir}/image-inspect.json"
  docker inspect -f '{{.Image}}' "${container}" >"${backup_dir}/previous-image-id"
fi

cp -a "${root}/config.yaml" "${backup_dir}/config.yaml"
cp -a "${root}/auth" "${backup_dir}/auth"
cp -a "${root}/plugins" "${backup_dir}/plugins"
[[ -f "${state_file}" ]] && cp -a "${state_file}" "${backup_dir}/cpa-key-policy-state.json"

install -m 0644 ops/production/docker-compose.yml "${compose_file}"
printf 'CLI_PROXY_IMAGE=%s\n' "${image}" >"${env_file}"
chmod 600 "${env_file}"
docker compose --env-file "${env_file}" -f "${compose_file}" config --quiet

if [[ "${old_container_present}" == true ]]; then
  log "waiting up to 60 seconds for active requests to drain"
  for _ in $(seq 1 60); do
    active_connections="$(
      docker exec "${container}" sh -c \
        "awk 'NR > 1 && \$2 ~ /:207D$/ && \$4 == \"01\" { count++ } END { print count+0 }' /proc/net/tcp /proc/net/tcp6" \
        2>/dev/null || printf '0'
    )"
    [[ "${active_connections}" == 0 ]] && break
    sleep 1
  done

  cutover_started=true
  docker stop --time 30 "${container}" >/dev/null
  if [[ "${state_file_in_old_container}" == true ]]; then
    docker cp "${container}:/CLIProxyAPI/cpa-key-policy-state.json" "${state_file}.new" >/dev/null
    install -m 0600 "${state_file}.new" "${state_file}"
    rm -f "${state_file}.new"
  elif [[ ! -f "${state_file}" ]]; then
    printf '{"version":1,"keys":[],"updated_at":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"${state_file}"
    chmod 600 "${state_file}"
  fi
  docker rename "${container}" "${rollback_container}"
  old_container_renamed=true
else
  cutover_started=true
  [[ -f "${state_file}" ]] || {
    printf '{"version":1,"keys":[],"updated_at":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"${state_file}"
    chmod 600 "${state_file}"
  }
fi

log "starting production container"
docker compose \
  --project-name "${compose_project}" \
  --env-file "${env_file}" \
  -f "${compose_file}" \
  up -d

healthy=false
for _ in $(seq 1 60); do
  if [[ "$(docker inspect -f '{{.State.Running}}' "${container}" 2>/dev/null || true)" != true ]]; then
    docker logs "${container}" >&2 || true
    fail "production container exited during startup"
  fi

  if docker run --rm --network "container:${container}" curlimages/curl:8.16.0 \
    --fail --silent --show-error --max-time 3 http://127.0.0.1:8317/healthz >/dev/null; then
    healthy=true
    break
  fi
  sleep 1
done
[[ "${healthy}" == true ]] || fail "production health check failed"

auth_count_after="$(docker exec "${container}" sh -c 'find /root/.cli-proxy-api -maxdepth 1 -type f | wc -l' | tr -d ' ')"
plugin_count_after="$(docker exec "${container}" sh -c 'find /CLIProxyAPI/plugins -type f | wc -l' | tr -d ' ')"
[[ "${auth_count_after}" == "${auth_count_before}" ]] || fail "authentication file count changed"
[[ "${plugin_count_after}" == "${plugin_count_before}" ]] || fail "plugin file count changed"

[[ "$(docker inspect -f '{{.Config.Image}}' "${container}")" == "${image}" ]] ||
  fail "production container is not using the requested image"
[[ "$(docker inspect -f '{{.HostConfig.Memory}}' "${container}")" == 6442450944 ]] ||
  fail "production memory limit changed"
[[ "$(docker inspect -f '{{.HostConfig.MemorySwap}}' "${container}")" == 6442450944 ]] ||
  fail "production swap limit changed"
[[ "$(docker inspect -f '{{.HostConfig.NanoCpus}}' "${container}")" == 4000000000 ]] ||
  fail "production CPU limit changed"
[[ "$(docker inspect -f '{{.HostConfig.PidsLimit}}' "${container}")" == 512 ]] ||
  fail "production PID limit changed"
[[ "$(docker inspect -f '{{index .HostConfig.LogConfig.Config "max-size"}}' "${container}")" == 50m ]] ||
  fail "production log max-size changed"
[[ "$(docker inspect -f '{{index .HostConfig.LogConfig.Config "max-file"}}' "${container}")" == 5 ]] ||
  fail "production log max-file changed"

require_bind_mount "${root}/config.yaml" /CLIProxyAPI/config.yaml
require_bind_mount "${root}/auth" /root/.cli-proxy-api
require_bind_mount "${root}/logs" /CLIProxyAPI/logs
require_bind_mount "${root}/plugins" /CLIProxyAPI/plugins
require_bind_mount "${state_file}" /CLIProxyAPI/cpa-key-policy-state.json

docker run --rm --network "${network}" curlimages/curl:8.16.0 \
  --fail --silent --show-error --max-time 5 http://cliproxyapi:8317/healthz >/dev/null

deployment_succeeded=true
log "deployment succeeded: image=${image} backup=${backup_dir} rollback_container=${rollback_container}"
