#!/usr/bin/env bash
# Hidden pre-roll for demo.tape: start the mock API, wait until it answers, and
# create the demo input files. Kept out of the tape so the recording only shows
# the api2convert commands themselves.
set -e

nohup python3 /demo-src/mock_api.py >/tmp/mock.log 2>&1 &

n=0
until python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:8080/health")' 2>/dev/null; do
  n=$((n + 1))
  [ "$n" -ge 100 ] && { echo "mock API did not start" >&2; exit 1; }
  sleep 0.1
done

bash /demo-src/make-files.sh "${1:-/demo}"
