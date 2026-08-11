#!/usr/bin/env bash
set -euo pipefail

review_tag=${1:-}
review_commit=${2:-}

if [[ ! "$review_commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "review commit must be a full lowercase 40-character git SHA" >&2
  exit 1
fi

expected_tag="review-sha-$review_commit"
if [[ "$review_tag" != "$expected_tag" ]]; then
  echo "review tag must exactly bind its commit as $expected_tag" >&2
  exit 1
fi

git rev-parse --verify "${review_commit}^{commit}" >/dev/null
if [[ "$(git rev-parse "${review_tag}^{commit}")" != "$review_commit" ]]; then
  echo "review tag $review_tag does not resolve to $review_commit" >&2
  exit 1
fi

echo "review source accepted: tag=$review_tag commit=$review_commit"
