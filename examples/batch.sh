#!/bin/sh
# Example: convert every image in ./photos to compressed WebP, keeping the tree,
# and never re-doing files that already exist (safe to re-run).
set -eu

export API2CONVERT_API_KEY="${API2CONVERT_API_KEY:?set your API key first}"

api2convert batch ./photos \
  --to webp \
  --option quality=80 \
  --out-dir ./web \
  --recursive \
  --on-conflict skip \
  --json
