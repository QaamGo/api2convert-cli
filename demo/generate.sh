#!/usr/bin/env bash
# Regenerate demo/demo.gif from demo/demo.tape.
#
# Runs the Charm VHS recorder inside Docker and drives the REAL api2convert
# binary against the local mock API (demo/mock_api.py) — no API key, no quota.
#
# Usage:
#   API2CONVERT_BIN=/path/to/linux-amd64/api2convert demo/generate.sh
#
# Build a suitable binary (linux/amd64) with:
#   docker run --rm -v "$PWD":/src -w /src golang:1.25 go build -o /src/api2convert .
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

BIN="${API2CONVERT_BIN:-}"
if [[ -z "$BIN" || ! -x "$BIN" ]]; then
  echo "error: set API2CONVERT_BIN to an executable linux/amd64 api2convert binary" >&2
  exit 1
fi

VHS_IMAGE="${VHS_IMAGE:-ghcr.io/charmbracelet/vhs:latest}"

docker run --rm \
  -w /demo-src \
  -v "$here":/demo-src \
  -v "$BIN":/usr/local/bin/api2convert:ro \
  "$VHS_IMAGE" \
  /demo-src/demo.tape

echo "Wrote $here/demo.gif"
