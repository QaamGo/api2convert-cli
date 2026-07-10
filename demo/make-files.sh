#!/usr/bin/env bash
# Create the demo inputs the tape converts. The mock API does not inspect the
# bytes, so these are tiny stand-ins with realistic names/extensions.
set -euo pipefail

dir="${1:-$PWD}"
cd "$dir"

printf 'api2convert demo document\n' > report.docx

mkdir -p images
printf 'demo image: sunset\n'  > images/sunset.jpg
printf 'demo image: skyline\n' > images/skyline.png
printf 'demo image: harbor\n'  > images/harbor.jpg
