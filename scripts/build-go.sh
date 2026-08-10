#!/usr/bin/env sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: scripts/build-go.sh OUTPUT PACKAGE" >&2
  exit 2
fi

output=$1
package=$2
version=${VERSION:-dev}
commit=${COMMIT:-}
build_state=${BUILD_STATE:-}

if [ -z "$commit" ]; then
  commit=$(git rev-parse HEAD 2>/dev/null || printf 'unknown')
fi
if [ -z "$build_state" ]; then
  if [ -n "$(git status --porcelain 2>/dev/null || true)" ]; then
    build_state=dirty
  elif git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    build_state=clean
  else
    build_state=unknown
  fi
fi

module=github.com/yannkr/openrsvp/internal/buildinfo
ldflags="-s -w -X ${module}.Version=${version} -X ${module}.Commit=${commit} -X ${module}.BuildState=${build_state}"

mkdir -p "$(dirname "$output")"
CGO_ENABLED=${CGO_ENABLED:-1} GOOS=${GOOS:-} GOARCH=${GOARCH:-} \
  go build -trimpath -ldflags "$ldflags" -o "$output" "$package"
