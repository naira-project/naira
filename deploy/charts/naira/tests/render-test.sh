#!/usr/bin/env bash
# Render assertions for the naira chart. Needs helm and mikefarah yq v4.
set -euo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)"
fail=0

# BASE names the two Secrets an operator must supply; without it the chart
# refuses to render (Task 7). render_raw is for tests that set the Secret
# source themselves (create=true, values-dev.yaml) or test the refusal.
BASE=(-f "$CHART/ci/default-values.yaml")
render_raw() { helm template t "$CHART" --namespace naira "$@"; }
render() { render_raw "${BASE[@]}" "$@"; }

check() {
  if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: expected '$2', got '$3'"; fail=1; fi
}

must_fail() {
  local name=$1 want=$2 out; shift 2
  if out=$(${RENDER:-render} "$@" 2>&1); then echo "FAIL $name: rendered without error"; fail=1
  elif [[ $out == *"$want"* ]]; then echo "ok   $name"
  else echo "FAIL $name: wrong error: $out"; fail=1; fi
}

CAT='select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.spec'

# ── Task 1: catalog ──────────────────────────────────────────────────────────
check "chart name" "naira" "$(helm show chart "$CHART" | yq '.name')"
check "catalog image from registry and AppVersion" "ghcr.io/naira-project/naira-catalog:0.1.0" \
  "$(render | yq "$CAT | .containers[0].image")"
check "local image with one tag" "naira-catalog:abc" \
  "$(render --set image.registry= --set image.tag=abc | yq "$CAT | .containers[0].image")"
check "catalog keycloak URL from dependencies" "http://keycloak.naira-deps.svc.cluster.local:8080" \
  "$(render | yq "$CAT | .containers[0].env[] | select(.name == \"KEYCLOAK_BASE_URL\") | .value")"
check "catalog Service fixed name and port" "catalog 8090" \
  "$(render | yq 'select(.kind == "Service" and .metadata.name == "catalog") | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "catalog uses the catalog ServiceAccount" "catalog" \
  "$(render | yq "$CAT | .serviceAccountName")"
check "objects land in the release namespace" "naira" \
  "$(render | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .metadata.namespace')"

exit $fail
