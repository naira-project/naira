#!/usr/bin/env bash
# Waits up to ~60s until VCLUSTER_PLATFORM_URL answers any HTTP request.
# Intended for CI after joining the tailnet, which propagates new peers
# eventually. Masks the URL in GitHub Actions logs.
#
# Usage: VCLUSTER_PLATFORM_URL=<url> wait-for-platform.sh
set -euo pipefail

if [ -z "${VCLUSTER_PLATFORM_URL:-}" ]; then
  echo "::error::VCLUSTER_PLATFORM_URL is not set" >&2
  exit 1
fi
echo "::add-mask::${VCLUSTER_PLATFORM_URL}"

for _ in $(seq 1 30); do
  if curl -s -o /dev/null --max-time 5 "${VCLUSTER_PLATFORM_URL}"; then
    exit 0
  fi
  sleep 2
done
echo "::error::vCluster Platform not reachable over the tailnet" >&2
exit 1
