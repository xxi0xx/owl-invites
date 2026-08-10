#!/usr/bin/env bash
set -euo pipefail

: "${PG_ADMIN_DSN:?PG_ADMIN_DSN is required}"
: "${SERVER_BINARY:?SERVER_BINARY is required}"
: "${CLI_BINARY:?CLI_BINARY is required}"

source_database=owl_invites_gate3_source
restore_database=owl_invites_gate3_restored
source_dsn="postgres://openrsvp:openrsvp@127.0.0.1:5432/${source_database}?sslmode=disable"
restore_dsn="postgres://openrsvp:openrsvp@127.0.0.1:5432/${restore_database}?sslmode=disable"
secret=4c64fb646f28c3cf57d320675a546e290173f4ab91786764
wrong_secret=f84a173ee20738df4867a3d18439aa9f63a81cf46c996d9f
work=$(mktemp -d "${RUNNER_TEMP:-/tmp}/owl-invites-postgres-restore.XXXXXX")
source_uploads=$work/source-uploads
uploads_archive=$work/uploads.tar
restored_uploads=$work/restored-uploads
state=$work/fixture.json
dump=$work/database.dump
metadata=$work/metadata.json
server_log=$work/server.log
server_pid=

mkdir -p "$source_uploads"
chmod 700 "$work" "$source_uploads"

stop_server() {
  if [[ -z "$server_pid" ]] || ! kill -0 "$server_pid" 2>/dev/null; then
    server_pid=
    return
  fi
  local stopped_pid=$server_pid
  kill -TERM "$stopped_pid"
  if ! timeout 10s tail --pid="$stopped_pid" -f /dev/null; then
    echo "server did not stop within 10 seconds" >&2
    return 1
  fi
  wait "$stopped_pid"
  server_pid=
}
trap stop_server EXIT

psql "$PG_ADMIN_DSN" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS ${source_database} WITH (FORCE)"
psql "$PG_ADMIN_DSN" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS ${restore_database} WITH (FORCE)"
psql "$PG_ADMIN_DSN" -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${source_database}"

export ENV=production
export DB_DRIVER=postgres
export DB_DSN=$source_dsn
export UPLOADS_DIR=$source_uploads
export BASE_URL=http://127.0.0.1:18080
export PORT=18080
export ALLOW_SIGNUPS=false
export OWL_INVITES_SECRET_KEY=$secret
unset OWL_INVITES_SECRET_KEY_FILE || true

go run ./internal/backup/testfixture --state "$state"
schema_version=$($CLI_BINARY migrate version)
fingerprint=$($CLI_BINARY secret fingerprint)
build_identity=$($CLI_BINARY version --json)
pg_dump --format=custom --no-owner --no-privileges --file "$dump" "$source_dsn"
pg_restore --list "$dump" > "$work/database.list"
test -s "$work/database.list"
tar -C "$source_uploads" -cf "$uploads_archive" .
dump_sha=$(sha256sum "$dump" | awk '{print $1}')
uploads_sha=$(sha256sum "$uploads_archive" | awk '{print $1}')
jq -n \
  --arg createdAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg schemaVersion "$schema_version" \
  --arg databaseSHA256 "$dump_sha" \
  --arg uploadsSHA256 "$uploads_sha" \
  --arg secretFingerprint "$fingerprint" \
  --arg pgDumpVersion "$(pg_dump --version)" \
  --argjson buildIdentity "$build_identity" \
  '{formatVersion:1,createdAt:$createdAt,databaseType:"postgres",schemaVersion:($schemaVersion|tonumber),databaseSHA256:$databaseSHA256,uploadsSHA256:$uploadsSHA256,secretFingerprint:$secretFingerprint,pgDumpVersion:$pgDumpVersion,buildIdentity:$buildIdentity,uploadsConsistency:"filesystem snapshot after database; not transactionally atomic"}' \
  > "$metadata"
chmod 600 "$dump" "$uploads_archive" "$metadata" "$state"
if grep -Fq "$secret" "$metadata"; then
  echo "PostgreSQL backup metadata leaked the secret" >&2
  exit 1
fi
echo "$dump_sha  $dump" | sha256sum --check --status
echo "$uploads_sha  $uploads_archive" | sha256sum --check --status

psql "$PG_ADMIN_DSN" -v ON_ERROR_STOP=1 -c "DROP DATABASE ${source_database} WITH (FORCE)"
mv "$source_uploads" "$work/destroyed-source-uploads"
psql "$PG_ADMIN_DSN" -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${restore_database}"
pg_restore --exit-on-error --no-owner --no-privileges --dbname "$restore_dsn" "$dump"
mkdir -p "$restored_uploads"
tar --extract --no-same-owner -f "$uploads_archive" -C "$restored_uploads"

export DB_DSN=$restore_dsn
export UPLOADS_DIR=$restored_uploads
if [[ "$($CLI_BINARY secret fingerprint)" != "$(jq -r .secretFingerprint "$metadata")" ]]; then
  echo "restored database has the wrong secret fingerprint" >&2
  exit 1
fi

$SERVER_BINARY >"$server_log" 2>&1 &
server_pid=$!
curl --fail --retry 30 --retry-delay 1 --retry-all-errors http://127.0.0.1:18080/health/ready >/dev/null

jq -n --arg capability "$(jq -r .capability "$state")" '{capability:$capability}' > "$work/exchange-request.json"
curl --fail --silent --show-error \
  -H 'Content-Type: application/json' --data-binary @"$work/exchange-request.json" \
  http://127.0.0.1:18080/api/v1/invitations/exchange > "$work/exchange-response.json"
jq -e \
  --arg invitation "$(jq -r .invitationId "$state")" \
  --arg guest "$(jq -r .guestId "$state")" \
  --arg question "$(jq -r .questionId "$state")" \
  --arg answer "$(jq -r .answer "$state")" \
  '.data.invitation.id == $invitation and
   any(.data.guests[]; .id == $guest and .attendance == "attending") and
   any(.data.invitationAnswers[]; .questionId == $question and .answer == $answer)' \
  "$work/exchange-response.json" >/dev/null
curl --fail --silent --show-error \
  "http://127.0.0.1:18080/api/v1/uploads/$(jq -r .upload "$state")" \
  | grep -Fx 'upload survived restore' >/dev/null
stop_server

export OWL_INVITES_SECRET_KEY=$wrong_secret
if [[ "$($CLI_BINARY secret fingerprint)" == "$(jq -r .secretFingerprint "$metadata")" ]]; then
  echo "wrong restore secret was not detected by fingerprint" >&2
  exit 1
fi
$SERVER_BINARY >"$server_log" 2>&1 &
server_pid=$!
curl --fail --retry 30 --retry-delay 1 --retry-all-errors http://127.0.0.1:18080/health/ready >/dev/null
old_status=$(curl --silent --output "$work/wrong-old.json" --write-out '%{http_code}' \
  -H 'Content-Type: application/json' --data-binary @"$work/exchange-request.json" \
  http://127.0.0.1:18080/api/v1/invitations/exchange)
jq -n '{capability:"oi1.missing.1.invalid"}' > "$work/missing-request.json"
missing_status=$(curl --silent --output "$work/wrong-missing.json" --write-out '%{http_code}' \
  -H 'Content-Type: application/json' --data-binary @"$work/missing-request.json" \
  http://127.0.0.1:18080/api/v1/invitations/exchange)
test "$old_status" = 401
test "$missing_status" = 401
cmp "$work/wrong-old.json" "$work/wrong-missing.json"
stop_server

echo "PostgreSQL backup/restore acceptance passed"
