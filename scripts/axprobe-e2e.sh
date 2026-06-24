#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${AXPROBE_IMAGE:-segmentstream-cli-axprobe:local}"
SCENARIO="${1:-full-analytics}"
if [ "$#" -gt 0 ]; then
  shift
fi

if ! command -v axprobe >/dev/null 2>&1; then
  echo "axprobe is required but was not found on PATH." >&2
  echo "Install it with: go install github.com/segmentstream/axprobe@latest" >&2
  exit 127
fi

REPORT_DIR="${AXPROBE_REPORT_DIR:-$ROOT/tests/axprobe-reports}"
mkdir -p "$REPORT_DIR"

commit="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
version="${SEGMENTSTREAM_VERSION:-axprobe-$commit}"
date_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker build \
  --file "$ROOT/.axprobe/Dockerfile" \
  --tag "$IMAGE" \
  --build-arg "SEGMENTSTREAM_VERSION=$version" \
  --build-arg "SEGMENTSTREAM_COMMIT=$commit" \
  --build-arg "SEGMENTSTREAM_DATE=$date_utc" \
  "$ROOT"

report="$REPORT_DIR/${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ).json"

cd "$ROOT"
exec axprobe run --report "$report" "$@" "$SCENARIO"
