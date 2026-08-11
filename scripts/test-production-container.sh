#!/usr/bin/env bash
set -euo pipefail

: "${IMAGE:?IMAGE is required}"
: "${EXPECTED_COMMIT:?EXPECTED_COMMIT is required}"
: "${EXPECTED_ARCH:?EXPECTED_ARCH is required}"

suffix=${GITHUB_RUN_ID:-local}-$$
container=owl-invites-gate3-$suffix
volume=owl-invites-gate3-data-$suffix
work=$(mktemp -d "${RUNNER_TEMP:-/tmp}/owl-invites-container.XXXXXX")
secret_file=$work/owl-invites-secret
secret=4c64fb646f28c3cf57d320675a546e290173f4ab91786764

cleanup() {
  docker rm --force "$container" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT

printf '%s\n' "$secret" > "$secret_file"
chmod 0444 "$secret_file"
docker volume create "$volume" >/dev/null

digest_placeholder=$(printf '0%.0s' {1..64})
OWL_INVITES_IMAGE_DIGEST=$digest_placeholder \
OWL_INVITES_SECRET_KEY_HOST_FILE=$secret_file \
BASE_URL=https://invites.example \
TRUSTED_PROXIES=10.0.0.0/8 \
SMTP_HOST=smtp.example \
SMTP_FROM=noreply@example.com \
  docker compose -f docker-compose.production.yml config --quiet

docker run --detach \
  --name "$container" \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777 \
  --mount "type=volume,source=$volume,target=/data" \
  --mount "type=bind,source=$secret_file,target=/run/secrets/owl_invites_secret_key,readonly" \
  --env ENV=production \
  --env PORT=8080 \
  --env BASE_URL=https://invites.example \
  --env DB_DRIVER=sqlite \
  --env DB_DSN=/data/openrsvp.db \
  --env UPLOADS_DIR=/data/uploads \
  --env OWL_INVITES_SECRET_KEY_FILE=/run/secrets/owl_invites_secret_key \
  --env ALLOW_SIGNUPS=false \
  --env NOTIFICATION_EMAIL_PROVIDER=smtp \
  --env SMTP_HOST=smtp.example \
  --env SMTP_FROM=noreply@example.com \
  "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$container" wget -qO- http://127.0.0.1:8080/health/ready > "$work/readiness.json" 2>/dev/null; then
    break
  fi
  if [[ "$(docker inspect --format '{{.State.Running}}' "$container")" != true ]]; then
    docker logs "$container" >&2
    exit 1
  fi
  sleep 1
done
jq -e '.status == "ok" and .database == "connected"' "$work/readiness.json" >/dev/null
docker exec "$container" wget -qO- http://127.0.0.1:8080/health > "$work/liveness.json"
jq -e --arg commit "$EXPECTED_COMMIT" '.status == "ok" and .commit == $commit' "$work/liveness.json" >/dev/null
docker exec "$container" wget -qO- http://127.0.0.1:8080/setup > "$work/frontend.html"
grep -Fqi '<!doctype html>' "$work/frontend.html"
grep -Fq '/_app/immutable/' "$work/frontend.html"

docker inspect "$container" > "$work/inspect.json"
jq -e '.[0].Config.User == "10001:10001"' "$work/inspect.json" >/dev/null
jq -e '.[0].HostConfig.ReadonlyRootfs == true' "$work/inspect.json" >/dev/null
jq -e '.[0].HostConfig.CapDrop | index("ALL") != null' "$work/inspect.json" >/dev/null
jq -e '.[0].HostConfig.SecurityOpt | any(. == "no-new-privileges:true")' "$work/inspect.json" >/dev/null
jq -e '.[0].HostConfig.Tmpfs["/tmp"] != null' "$work/inspect.json" >/dev/null
jq -e 'any(.[0].Mounts[]; .Destination == "/data" and .RW == true)' "$work/inspect.json" >/dev/null
jq -e 'all(.[0].Mounts[]; .Destination != "/var/run/docker.sock")' "$work/inspect.json" >/dev/null
jq -e '[.[0].NetworkSettings.Ports[]? | select(. != null)] | length == 0' "$work/inspect.json" >/dev/null

test "$(docker exec "$container" id -u)" = 10001
test "$(docker exec "$container" awk '/CapEff/ {print $2}' /proc/1/status)" = 0000000000000000
if docker exec "$container" touch /root-filesystem-must-remain-read-only 2>/dev/null; then
  echo "container root filesystem was writable" >&2
  exit 1
fi
docker exec "$container" touch /data/writable-state-probe
docker exec "$container" touch /tmp/writable-temp-probe

docker exec "$container" owl-invites version --json > "$work/version.json"
jq -e --arg commit "$EXPECTED_COMMIT" '.version == "ci" and .commit == $commit and .buildState == "clean"' "$work/version.json" >/dev/null
test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMAGE")" = "$EXPECTED_COMMIT"
test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$IMAGE")" = ci
test "$(docker image inspect --format '{{.Architecture}}' "$IMAGE")" = "$EXPECTED_ARCH"
docker exec "$container" test -x /usr/local/bin/owl-invites
if docker exec "$container" test -e /usr/local/bin/openrsvp; then
  echo "production image still ships the retired openrsvp executable" >&2
  exit 1
fi

if docker logs "$container" 2>&1 | grep -Fq "$secret"; then
  echo "container logs leaked OWL_INVITES_SECRET_KEY" >&2
  exit 1
fi
started=$(date +%s)
docker stop --time 10 "$container" >/dev/null
elapsed=$(( $(date +%s) - started ))
test "$elapsed" -le 10
test "$(docker inspect --format '{{.State.ExitCode}}' "$container")" = 0
docker logs "$container" 2>&1 | grep -F 'server stopped gracefully' >/dev/null

echo "production container acceptance passed"
