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

# ── Task 2: plugins ──────────────────────────────────────────────────────────
PLUGINS_YAML='select(.kind == "ConfigMap" and .metadata.name == "catalog-plugin-config") | .data["plugins.yaml"]'
LITELLM_ENV="$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .env[]"

check "default sidecars (tech-radar off), sorted" \
  "plugin-depl-calls-svc,plugin-depl-uses-litellm,plugin-fluxcd,plugin-litellm,plugin-mcp-servers,plugin-mlflow,plugin-openmetadata" \
  "$(render | yq ea "[$CAT | .initContainers[].name] | join(\",\")")"
check "sidecars are native sidecars" "Always" \
  "$(render | yq ea "[$CAT | .initContainers[].restartPolicy] | unique | join(\",\")")"
check "plugin image" "ghcr.io/naira-project/naira-plugin-litellm:0.1.0" \
  "$(render | yq "$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .image")"
check "plugin PORT" "50051" "$(render | yq "$LITELLM_ENV | select(.name == \"PORT\") | .value")"
check "plugin PATH_PREFIX" "litellm" "$(render | yq "$LITELLM_ENV | select(.name == \"PATH_PREFIX\") | .value")"
check "plugin env tpl-rendered from dependencies" "http://litellm.naira-deps.svc.cluster.local:4000" \
  "$(render | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "one dependency URL override reaches the plugin" "http://litellm.run1.svc.cluster.local:4000" \
  "$(render --set dependencies.litellm.baseUrl=http://litellm.run1.svc.cluster.local:4000 \
     | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "plugin secret env defaults to catalog-secrets" "catalog-secrets/LITELLM_API_KEY" \
  "$(render | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\") | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "plugin resources default" "128Mi" \
  "$(render | yq "$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .resources.limits.memory")"
check "plugin resources override merges" "256Mi 50m" \
  "$(render --set catalog.plugins.litellm.resources.limits.memory=256Mi \
     | yq "$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .resources | .limits.memory + \" \" + .requests.cpu")"
check "plugins.yaml entry" "localhost:50051 0 0 * * *" \
  "$(render | yq "$PLUGINS_YAML" | yq '.plugins.litellm | .address + " " + .schedule')"
check "plugins.yaml omits schedule when unset" "null" \
  "$(render | yq "$PLUGINS_YAML" | yq '.plugins.openmetadata.schedule')"
check "disabled plugin: no sidecar" "" \
  "$(render --set catalog.plugins.litellm.enabled=false | yq "$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .name")"
check "disabled plugin: not in plugins.yaml" "false" \
  "$(render --set catalog.plugins.litellm.enabled=false | yq "$PLUGINS_YAML" | yq '.plugins | has("litellm")')"
check "plugins.yaml mounted" "/etc/catalog/plugins.yaml" \
  "$(render | yq "$CAT | .containers[0].volumeMounts[] | select(.name == \"plugin-config\") | .mountPath")"
check "checksum changes with plugins" "different" \
  "$( [ "$(render | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.metadata.annotations["checksum/plugin-config"]')" != \
        "$(render --set catalog.plugins.litellm.enabled=false | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.metadata.annotations["checksum/plugin-config"]')" ] \
      && echo different || echo same)"

exit $fail
