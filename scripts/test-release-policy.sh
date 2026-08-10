#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
validator="$repo_root/scripts/validate-release-ref.sh"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

git -C "$fixture" init --quiet
git -C "$fixture" config user.email gate3@example.invalid
git -C "$fixture" config user.name "Gate 3 policy test"
printf 'main\n' >"$fixture/state"
git -C "$fixture" add state
git -C "$fixture" commit --quiet -m main
git -C "$fixture" update-ref refs/remotes/origin/main HEAD
main_commit=$(git -C "$fixture" rev-parse HEAD)

git -C "$fixture" checkout --quiet -b review
printf 'review\n' >>"$fixture/state"
git -C "$fixture" commit --quiet -am review
review_commit=$(git -C "$fixture" rev-parse HEAD)

git -C "$fixture" checkout --quiet --detach "$main_commit"
(cd "$fixture" && bash "$validator" v1.2.3 "$main_commit" refs/remotes/origin/main >/dev/null)

for invalid in v1 v1.2 v1.2.3-rc.1 v01.2.3 v1.02.3 v1.2.03 latest 1.2.3; do
  if (cd "$fixture" && bash "$validator" "$invalid" "$main_commit" refs/remotes/origin/main >/dev/null 2>&1); then
    echo "invalid release tag was accepted: $invalid" >&2
    exit 1
  fi
done

if (cd "$fixture" && bash "$validator" v1.2.3 "$review_commit" refs/remotes/origin/main >/dev/null 2>&1); then
  echo "non-main release commit was accepted" >&2
  exit 1
fi

echo "release policy tests passed"
