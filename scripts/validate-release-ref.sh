#!/usr/bin/env bash
set -euo pipefail

release_tag=${1:-}
release_commit=${2:-}
main_ref=${3:-refs/remotes/origin/main}

if [[ ! "$release_tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "release tag must use exact stable SemVer syntax vX.Y.Z: $release_tag" >&2
  exit 1
fi

if [[ -z "$release_commit" ]]; then
  echo "release commit is required" >&2
  exit 1
fi

git rev-parse --verify "${release_commit}^{commit}" >/dev/null
git rev-parse --verify "${main_ref}^{commit}" >/dev/null

if ! git merge-base --is-ancestor "$release_commit" "$main_ref"; then
  echo "release commit $release_commit is not contained in $main_ref" >&2
  exit 1
fi

echo "release source accepted: tag=$release_tag commit=$(git rev-parse "$release_commit") main=$main_ref"
