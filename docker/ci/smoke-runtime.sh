#!/usr/bin/env bash
set -Eeuo pipefail

: "${E2E_IMAGE:?E2E_IMAGE is required}"

artifact_dir="${E2E_ARTIFACT_DIR:-${RUNNER_TEMP:-/tmp}/evolution-go-runtime}"
mkdir -p "$artifact_dir"
raw_log="$artifact_dir/application.raw.log"
safe_log="$artifact_dir/application.sanitized.log"
summary="$artifact_dir/summary.json"

run_suffix="${GITHUB_RUN_ID:-local}-$(openssl rand -hex 4)"
network="evogo-smoke-${run_suffix}"
postgres_container="evogo-postgres-${run_suffix}"
redis_container="evogo-redis-${run_suffix}"
app_container="evogo-api-${run_suffix}"
postgres_password="$(openssl rand -hex 24)"
global_api_key="$(openssl rand -hex 32)"
host_port=18080

for secret in "$postgres_password" "$global_api_key"; do
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    printf '::add-mask::%s\n' "$secret"
  fi
done

cleanup() {
  docker logs "$app_container" >"$raw_log" 2>&1 || true
  if [[ -f "$raw_log" ]]; then
    sed \
      -e "s/${postgres_password}/[REDACTED_POSTGRES_PASSWORD]/g" \
      -e "s/${global_api_key}/[REDACTED_GLOBAL_API_KEY]/g" \
      -E -e 's#(postgres(ql)?://[^:[:space:]]+:)[^@[:space:]]+#\1[REDACTED]#g' \
	  -E -e 's/[0-9a-fA-F]{8}-[0-9a-fA-F-]{27,}/[REDACTED_INSTANCE_ID]/g' \
	  -E -e 's/[0-9]{8,}(:[0-9]+)?@(s\.whatsapp\.net|lid)/[REDACTED_JID]/g' \
	  -E -e 's#data:image/png;base64,[A-Za-z0-9+/=]+#[REDACTED_QR_IMAGE]#g' \
      "$raw_log" >"$safe_log"
    rm -f "$raw_log"
  else
    : >"$safe_log"
  fi
  docker rm -f "$app_container" "$redis_container" "$postgres_container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$network" >/dev/null

docker run -d --name "$postgres_container" --network "$network" \
  --network-alias postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD="$postgres_password" \
  -e POSTGRES_DB=postgres \
  postgres:16-alpine >/dev/null

# Redis is checked as a deployment dependency only. Evolution Go does not use it.
docker run -d --name "$redis_container" --network "$network" \
  --network-alias redis \
  redis:7-alpine redis-server --save '' --appendonly no >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$postgres_container" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$postgres_container" pg_isready -U postgres -d postgres >/dev/null
docker exec "$redis_container" redis-cli ping | grep -qx PONG

database_base="postgresql://postgres:${postgres_password}@postgres:5432"
docker run -d --name "$app_container" --network "$network" \
  -p "127.0.0.1:${host_port}:4000" \
  -e SERVER_PORT=4000 \
  -e CLIENT_NAME=evolution-ci \
  -e GLOBAL_API_KEY="$global_api_key" \
  -e POSTGRES_AUTH_DB="${database_base}/evogo_auth?sslmode=disable" \
  -e POSTGRES_USERS_DB="${database_base}/evogo_users?sslmode=disable" \
  -e DATABASE_SAVE_MESSAGES=false \
  -e CONNECT_ON_STARTUP=false \
  -e WEBHOOK_FILES=false \
  -e OS_NAME=Linux \
  -e AMQP_URL= \
  -e AMQP_GLOBAL_ENABLED=false \
  -e NATS_URL= \
  -e NATS_GLOBAL_ENABLED=false \
  -e MINIO_ENABLED=false \
  -e QRCODE_MAX_COUNT=5 \
  -e LOGTYPE=json \
  -e WADEBUG=INFO \
  "$E2E_IMAGE" >/dev/null

base_url="http://127.0.0.1:${host_port}"
ready=false
for _ in $(seq 1 90); do
  if curl --fail --silent --show-error "$base_url/server/ok" | jq -e '.status == "ok"' >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! docker inspect -f '{{.State.Running}}' "$app_container" 2>/dev/null | grep -qx true; then
    echo 'API container exited before readiness' >&2
    exit 1
  fi
  sleep 2
done
[[ "$ready" == true ]]

jq -n \
  --arg postgres healthy \
  --arg redis healthy \
  --arg api ready \
  --arg license_scope 'not exercised; protocol gate runs at service layer' \
  '{postgres:$postgres,redis:$redis,api:$api,license_scope:$license_scope}' >"$summary"

echo 'Runtime image smoke passed: PostgreSQL healthy, Redis healthy, API ready'
