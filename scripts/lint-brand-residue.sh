#!/usr/bin/env bash
set -euo pipefail

upstream_name='Open''RSVP'
upstream_slug='open''rsvp'
working_name='In''via'
old_module='github.com/yannkr/'"$upstream_slug"
old_repository='yannkr/'"$upstream_slug"
old_image='ghcr.io/'"$old_repository"

scan_args=(--hidden --glob '!.git/**' --glob '!web/node_modules/**' --glob '!web/.svelte-kit/**' --glob '!web/build/**')

allowed_residue() {
  local file=$1
  local text=$2
  case "$file" in
    LICENSE|NOTICE|docs/provenance.md|docs/upstream/*|docs/gate-1-foundation.md|docs/gate-3-production-readiness.md|docs/gate-4-brand-inventory.md|docs/gate-4-owl-rebrand.md)
      return 0
      ;;
    README.md)
      [[ "$text" == *"docs/upstream/$upstream_slug-history.md"* ]]
      ;;
    internal/backup/sqlite_test.go|internal/config/config.go|internal/config/config_test.go)
      [[ "$text" == *"$upstream_name"* || "$text" == *"$upstream_slug"* ]]
      ;;
    internal/calendar/ics.go|internal/calendar/ics_test.go)
      [[ "$text" == *"$upstream_name"* || "$text" == *"$upstream_slug"* ]]
      ;;
    internal/useradmin/service.go)
      [[ "$text" == *"$upstream_name"* ]]
      ;;
    internal/webhook/dispatcher.go|internal/webhook/dispatcher_test.go)
      [[ "$text" == *"$upstream_name"* ]]
      ;;
    internal/notification/templates/templates_test.go)
      [[ "$text" == *"$upstream_name"* || "$text" == *"$upstream_slug"* ]]
      ;;
    scripts/test-production-container.sh)
      [[ "$text" == *"/usr/local/bin/$upstream_slug"* || "$text" == *"retired $upstream_slug executable"* ]]
      ;;
    docs/operations/backup-restore.md|docs/operations/production-deployment.md)
      [[ "$text" == *"$upstream_name"* || "$text" == *"$upstream_slug"* ]]
      ;;
    *)
      return 1
      ;;
  esac
}

violations=0
while IFS= read -r match; do
  [[ -z "$match" ]] && continue
  match=${match#./}
  match=${match#.\\}
  file=${match%%:*}
  file=${file//\\//}
  rest=${match#*:}
  text=${rest#*:}
  if ! allowed_residue "$file" "$text"; then
    printf 'unapproved brand residue: %s\n' "$match" >&2
    violations=$((violations + 1))
  fi
done < <(rg -n "${scan_args[@]}" -e "$upstream_name|$upstream_slug|$working_name" . || true)

while IFS= read -r match; do
  [[ -z "$match" ]] && continue
  match=${match#./}
  match=${match#.\\}
  file=${match%%:*}
  file=${file//\\//}
  case "$file" in
    NOTICE|docs/provenance.md|docs/upstream/*|docs/gate-4-brand-inventory.md|docs/gate-4-owl-rebrand.md) ;;
    *)
      printf 'unapproved old module path: %s\n' "$match" >&2
      violations=$((violations + 1))
      ;;
  esac
done < <(rg -n -F "${scan_args[@]}" "$old_module" . || true)

while IFS= read -r match; do
  [[ -z "$match" ]] && continue
  match=${match#./}
  match=${match#.\\}
  file=${match%%:*}
  file=${file//\\//}
  case "$file" in
    NOTICE|docs/provenance.md|docs/upstream/*|docs/gate-4-brand-inventory.md|docs/gate-4-owl-rebrand.md) ;;
    *)
      printf 'unapproved old repository identity: %s\n' "$match" >&2
      violations=$((violations + 1))
      ;;
  esac
done < <(rg -n -F "${scan_args[@]}" "$old_repository" . || true)

test "$(sed -n '1p' go.mod)" = 'module github.com/xxi0xx/owl-invites' || {
  echo 'Go module is not the canonical Owl Invites module' >&2
  violations=$((violations + 1))
}
test -d cmd/owl-invites || {
  echo 'canonical cmd/owl-invites target is missing' >&2
  violations=$((violations + 1))
}
test ! -e cmd/$upstream_slug || {
  echo 'retired upstream command target still exists' >&2
  violations=$((violations + 1))
}
grep -Fq 'ENTRYPOINT ["/usr/local/bin/owl-invites"]' Dockerfile || {
  echo 'Docker entrypoint is not the canonical Owl Invites binary' >&2
  violations=$((violations + 1))
}
grep -Fq 'org.opencontainers.image.title="Owl Invites"' Dockerfile || {
  echo 'OCI title is not Owl Invites' >&2
  violations=$((violations + 1))
}
grep -Fq 'ARG SOURCE_URL=https://github.com/xxi0xx/owl-invites' Dockerfile || {
  echo 'OCI source default is not the canonical repository' >&2
  violations=$((violations + 1))
}

count_literal() {
  local token=$1
  (rg -o -F "${scan_args[@]}" "$token" . || true) | wc -l | tr -d ' '
}

printf 'Approved brand residue counts:\n'
printf '  %s: %s\n' "$upstream_name" "$(count_literal "$upstream_name")"
printf '  %s: %s\n' "$upstream_slug" "$(count_literal "$upstream_slug")"
printf '  prior working name: %s\n' "$(count_literal "$working_name")"
printf '  old module path: %s\n' "$(count_literal "$old_module")"
printf '  old repository path: %s\n' "$(count_literal "$old_repository")"
printf '  old GHCR path: %s\n' "$(count_literal "$old_image")"

if (( violations > 0 )); then
  printf '%d unapproved brand residue occurrence(s) found\n' "$violations" >&2
  exit 1
fi

echo 'Brand residue is limited to documented compatibility and provenance exceptions.'
