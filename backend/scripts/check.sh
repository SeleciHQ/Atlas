#!/usr/bin/env bash
# Backend verification script. Requires Go 1.27.4+ on PATH.
# Run from anywhere; paths are derived from this script's location.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! command -v go >/dev/null 2>&1; then
  echo "Go 1.27.4 or newer is required on PATH (see backend/README.md)." >&2
  exit 1
fi
if ! command -v gofmt >/dev/null 2>&1; then
  echo "gofmt is required on PATH (ships with Go)." >&2
  exit 1
fi

unformatted="$(gofmt -l cmd internal)"
if [[ -n "$unformatted" ]]; then
  echo "Unformatted Go files:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./cmd/... ./internal/...
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
