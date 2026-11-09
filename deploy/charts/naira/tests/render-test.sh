#!/usr/bin/env bash
# Tests for the naira Helm chart. Each check runs `helm template` with some
# values and asserts on the rendered manifests with yq, so a change to
# values.yaml or a template that breaks the chart's contract fails here
# instead of on someone's cluster. In order, the sections cover:
#   - catalog Deployment, Service and image naming
#   - plugin sidecars, plugins.yaml and per-plugin RBAC
#   - plugin config files, Secrets, ui and portal
#   - guards that must refuse to render (bad values, missing Secrets)
#   - values-dev.yaml and the kind/CI setup
#   - pod hardening, scheduling fields and the catalog NetworkPolicy
#   - handbook workloads, the handbooks ConfigMap, handbook guards and handbook NetworkPolicies
#   - the handbook gateway: Envoy config, Deployment, rollout checksum and guards
# Requires helm and mikefarah yq v4 (both pinned in mise.toml).
set -euo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)"
# Release PRs bump Chart.yaml, so image tags are read from it, not hardcoded.
APP_VERSION=$(yq '.appVersion' "$CHART/Chart.yaml")
fail=0

# BASE lists flags to add a minimal set of values required to render the chart.
BASE=(-f "$CHART/ci/default-values.yaml")
# DB names the operator-supplied database Secrets, for render_raw calls that
# test something else.
DB=(--set database.admin.secret.existingSecret=postgres-admin
  --set database.databases.catalog.secret.existingSecret=catalog-db)
# render_raw renders the chart without applying BASE flags; it is intended
# for tests that set them themselves or verify behavior without them.
render_raw() {
  helm template t "$CHART" --namespace naira "$@"
}
render() {
  render_raw "${BASE[@]}" "$@"
}

# check NAME EXPECTED ACTUAL
check() {
  if [ "$2" = "$3" ]; then
    echo "ok   $1"
  else
    echo "FAIL $1: expected '$2', got '$3'"
    fail=1
  fi
}

# must_fail NAME ERROR_SUBSTRING [helm args]: rendering has to fail with ERROR_SUBSTRING.
must_fail() {
  local name=$1 want=$2 out
  shift 2
  if out=$(${RENDER:-render} "$@" 2>&1); then
    echo "FAIL $name: rendered without error"
    fail=1
  elif [[ $out == *"$want"* ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: wrong error: $out"
    fail=1
  fi
}

# yq selectors for the objects the checks look at.
CATALOG='select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.spec'
pod() { printf 'select(.kind == "Deployment" and .metadata.name == "%s") | .spec.template.spec' "$1"; }

# ── Catalog ──────────────────────────────────────────────────────────────────
check "chart name" \
  "naira" \
  "$(helm show chart "$CHART" | yq '.name')"
check "catalog image from registry and AppVersion" \
  "ghcr.io/naira-project/naira-catalog:${APP_VERSION}" \
  "$(render | yq "$CATALOG | .containers[0].image")"
check "local image with one tag" \
  "naira-catalog:abc" \
  "$(render --set image.registry= --set image.tag=abc \
    | yq "$CATALOG | .containers[0].image")"
check "catalog keycloak URL from dependencies" \
  "http://keycloak.naira-deps.svc.cluster.local:8080" \
  "$(render \
    | yq "$CATALOG | .containers[0].env[] | select(.name == \"KEYCLOAK_BASE_URL\") | .value")"
check "catalog Service fixed name and port" \
  "catalog 8090" \
  "$(render \
    | yq 'select(.kind == "Service" and .metadata.name == "catalog")
          | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "catalog uses the catalog ServiceAccount" \
  "catalog" \
  "$(render | yq "$CATALOG | .serviceAccountName")"
check "objects land in the release namespace" \
  "naira" \
  "$(render | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .metadata.namespace')"

# ── Plugins ──────────────────────────────────────────────────────────────────
PLUGINS_YAML='select(.kind == "ConfigMap" and .metadata.name == "catalog-plugin-config") | .data["plugins.yaml"]'
LITELLM_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-litellm\")"
LITELLM_ENV="$LITELLM_SIDECAR | .env[]"
CHECKSUM='select(.kind == "Deployment" and .metadata.name == "catalog")
          | .spec.template.metadata.annotations["checksum/plugin-config"]'

check "default sidecars (tech-radar off), sorted" \
  "plugin-depl-calls-svc,plugin-fluxcd,plugin-litellm,plugin-mcp-servers,plugin-mlflow,plugin-openmetadata" \
  "$(render | yq ea "[$CATALOG | .initContainers[].name] | join(\",\")")"
check "sidecars are native sidecars" \
  "Always" \
  "$(render | yq ea "[$CATALOG | .initContainers[].restartPolicy] | unique | join(\",\")")"
check "plugin image" \
  "ghcr.io/naira-project/naira-plugin-litellm:${APP_VERSION}" \
  "$(render | yq "$LITELLM_SIDECAR | .image")"
check "plugin PORT" \
  "50051" \
  "$(render | yq "$LITELLM_ENV | select(.name == \"PORT\") | .value")"
check "plugin PATH_PREFIX" \
  "litellm" \
  "$(render | yq "$LITELLM_ENV | select(.name == \"PATH_PREFIX\") | .value")"
check "plugin env default" \
  "http://litellm.naira-deps.svc.cluster.local:4000" \
  "$(render | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "plugin env override" \
  "http://litellm.run1.svc.cluster.local:4000" \
  "$(render --set catalog.plugins.litellm.env.LITELLM_BASE_URL=http://litellm.run1.svc.cluster.local:4000 \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "plugin secret env defaults to catalog-secrets" \
  "catalog-secrets/LITELLM_API_KEY" \
  "$(render \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\")
          | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "plugin resources default" \
  "128Mi" \
  "$(render | yq "$LITELLM_SIDECAR | .resources.limits.memory")"
check "plugin resources override merges" \
  "256Mi 200m 50m" \
  "$(render --set catalog.plugins.litellm.resources.limits.memory=256Mi \
    | yq "$LITELLM_SIDECAR | .resources
          | .limits.memory + \" \" + .limits.cpu + \" \" + .requests.cpu")"
check "plugins.yaml entry" \
  "localhost:50051 0 0 * * *" \
  "$(render | yq "$PLUGINS_YAML" | yq '.plugins.litellm | .address + " " + .schedule')"
check "plugins.yaml omits schedule when unset" \
  "null" \
  "$(render | yq "$PLUGINS_YAML" | yq '.plugins.openmetadata.schedule')"
check "disabled plugin: no sidecar" \
  "" \
  "$(render --set catalog.plugins.litellm.enabled=false | yq "$LITELLM_SIDECAR | .name")"
check "disabled plugin: not in plugins.yaml" \
  "false" \
  "$(render --set catalog.plugins.litellm.enabled=false \
    | yq "$PLUGINS_YAML" | yq '.plugins | has("litellm")')"
check "plugins.yaml mounted" \
  "/etc/catalog/plugins.yaml" \
  "$(render \
    | yq "$CATALOG | .containers[0].volumeMounts[] | select(.name == \"plugin-config\") | .mountPath")"

WITH_LITELLM=$(render | yq "$CHECKSUM")
WITHOUT_LITELLM=$(render --set catalog.plugins.litellm.enabled=false | yq "$CHECKSUM")
if [ "$WITH_LITELLM" != "$WITHOUT_LITELLM" ]; then checksum_changed=different; else checksum_changed=same; fi
check "checksum changes with plugins" "different" "$checksum_changed"

# ── RBAC ─────────────────────────────────────────────────────────────────────
check "ClusterRoles are release-prefixed" \
  "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render | yq ea '[select(.kind == "ClusterRole") | .metadata.name] | sort | join(",")')"
check "bindings match roles" \
  "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render | yq ea '[select(.kind == "ClusterRoleBinding") | .roleRef.name] | sort | join(",")')"
check "binding subject is the catalog SA in the release namespace" \
  "catalog/naira" \
  "$(render \
    | yq 'select(.kind == "ClusterRoleBinding" and .metadata.name == "t-plugin-fluxcd")
          | .subjects[0] | .name + "/" + .namespace')"
check "plugin rules copied" \
  "namespaces,services" \
  "$(render \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-calls-svc")
          | .rules[0].resources | join(",")')"
check "disabled plugin: no RBAC" \
  "" \
  "$(render --set catalog.plugins.fluxcd.enabled=false \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-fluxcd") | .metadata.name')"
check "two releases do not collide" \
  "u-catalog" \
  "$(helm template u "$CHART" --namespace other "${BASE[@]}" \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "u-catalog") | .metadata.name')"

# ── Plugin config files ──────────────────────────────────────────────────────
TECH_RADAR=(
  --set catalog.plugins.tech-radar.enabled=true
  --set-string 'catalog.plugins.tech-radar.config.data.radar\.yaml=schema_version: 1'
)
TECH_RADAR_EXISTING=(
  --set catalog.plugins.tech-radar.enabled=true
  --set catalog.plugins.tech-radar.config.existingConfigMap=my-radar
)
TECH_RADAR_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-tech-radar\")"
TECH_RADAR_VOLUME="$CATALOG | .volumes[] | select(.name == \"plugin-tech-radar-config\")"
TECH_RADAR_CONFIGMAP='select(.kind == "ConfigMap" and .metadata.name == "plugin-tech-radar-config")'

check "tech-radar ConfigMap from data" \
  "schema_version: 1" \
  "$(render "${TECH_RADAR[@]}" | yq "$TECH_RADAR_CONFIGMAP | .data[\"radar.yaml\"]")"
check "tech-radar mounted as a directory" \
  "/etc/naira/techradar none" \
  "$(render "${TECH_RADAR[@]}" \
    | yq "$TECH_RADAR_SIDECAR | .volumeMounts[0] | .mountPath + \" \" + (.subPath // \"none\")")"
check "tech-radar volume source" \
  "plugin-tech-radar-config" \
  "$(render "${TECH_RADAR[@]}" | yq "$TECH_RADAR_VOLUME | .configMap.name")"
check "existingConfigMap: no generated ConfigMap" \
  "" \
  "$(render "${TECH_RADAR_EXISTING[@]}" | yq "$TECH_RADAR_CONFIGMAP | .metadata.name")"
check "existingConfigMap: volume points at it" \
  "my-radar" \
  "$(render "${TECH_RADAR_EXISTING[@]}" | yq "$TECH_RADAR_VOLUME | .configMap.name")"
check "default plus tech-radar and depl-uses-litellm: 8 sidecars" \
  "8" \
  "$(render "${TECH_RADAR[@]}" --set catalog.plugins.depl-uses-litellm.enabled=true \
    | yq "$CATALOG | .initContainers | length")"
check "tech-radar registered in plugins.yaml" \
  "localhost:50057" \
  "$(render "${TECH_RADAR[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.tech-radar.address')"

# ── Secrets ──────────────────────────────────────────────────────────────────
secrets=$(render | yq 'select(.kind == "Secret") | .metadata.name' | grep -c . || true) # grep exits 1 on no match
check "no Secrets by default" \
  "0" \
  "${secrets:-0}"
check "catalog Secret when create=true" \
  "k1" \
  "$(render_raw "${DB[@]}" --set portal.enabled=false \
      --set catalog.secret.create=true --set catalog.secret.data.LITELLM_API_KEY=k1 \
    | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "existingSecret reaches plugin env" \
  "ext" \
  "$(render --set catalog.secret.existingSecret=ext \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\") | .valueFrom.secretKeyRef.name")"
check "portal Secret when create=true" \
  "s3" \
  "$(render_raw "${DB[@]}" --set catalog.enabled=false \
      --set portal.oidc.secret.create=true --set portal.oidc.secret.value=s3 \
    | yq 'select(.kind == "Secret" and .metadata.name == "portal-oidc") | .stringData["client-secret"]')"

# ── ui and portal ────────────────────────────────────────────────────────────
UI='select(.kind == "Deployment" and .metadata.name == "ui") | .spec.template.spec.containers[0]'
PORTAL='select(.kind == "Deployment" and .metadata.name == "portal") | .spec.template.spec.containers[0]'

check "ui catalog upstream follows the release namespace" \
  "http://catalog.naira.svc.cluster.local:8090" \
  "$(render | yq "$UI | .env[] | select(.name == \"CATALOG_UPSTREAM\") | .value")"
check "ui catalog upstream override" \
  "http://x:1" \
  "$(render --set ui.catalogUpstream=http://x:1 \
    | yq "$UI | .env[] | select(.name == \"CATALOG_UPSTREAM\") | .value")"
check "ui prometheus upstream default" \
  "http://prometheus-operated.naira-deps.svc.cluster.local:9090" \
  "$(render | yq "$UI | .env[] | select(.name == \"PROMETHEUS_UPSTREAM\") | .value")"
check "ui prometheus upstream override" \
  "http://prom:9090" \
  "$(render --set ui.prometheus.upstream=http://prom:9090 \
    | yq "$UI | .env[] | select(.name == \"PROMETHEUS_UPSTREAM\") | .value")"
check "ui prometheus upstream empty: no env var" \
  "" \
  "$(render --set ui.prometheus.upstream= \
    | yq "$UI | .env[] | select(.name == \"PROMETHEUS_UPSTREAM\") | .value")"
check "ui prometheus allowed paths and methods default to the image's" \
  "" \
  "$(render | yq "$UI | .env[] | select(.name == \"PROMETHEUS_ALLOWED_PATHS\" or .name == \"PROMETHEUS_ALLOWED_METHODS\") | .name")"
check "ui prometheus allowed paths joined with |" \
  "query|series" \
  "$(render --set 'ui.prometheus.allowedPaths[0]=query' --set 'ui.prometheus.allowedPaths[1]=series' \
    | yq "$UI | .env[] | select(.name == \"PROMETHEUS_ALLOWED_PATHS\") | .value")"
check "ui prometheus allowed methods joined with a space" \
  "GET HEAD" \
  "$(render --set 'ui.prometheus.allowedMethods[0]=GET' --set 'ui.prometheus.allowedMethods[1]=HEAD' \
    | yq "$UI | .env[] | select(.name == \"PROMETHEUS_ALLOWED_METHODS\") | .value")"
UI_DEPLOY='select(.kind == "Deployment" and .metadata.name == "ui")'
check "ui config: no ConfigMap by default" \
  "" \
  "$(render | yq 'select(.kind == "ConfigMap" and .metadata.name == "ui-config") | .metadata.name')"
check "ui config: no mount by default" \
  "null" \
  "$(render | yq "$UI | .volumeMounts")"
check "ui config: rendered into config.json" \
  "http://grafana:3000" \
  "$(render --set ui.config.logs.explorerUrl=http://grafana:3000 \
    | yq 'select(.kind == "ConfigMap" and .metadata.name == "ui-config") | .data["config.json"] | from_json | .logs.explorerUrl')"
check "ui config: mounted over the image's config.json" \
  "/usr/share/nginx/html/config.json config.json" \
  "$(render --set ui.config.logs.explorerUrl=http://grafana:3000 \
    | yq "$UI | .volumeMounts[0] | .mountPath + \" \" + .subPath")"
check "ui config: pod restarts when it changes" \
  "true" \
  "$(render --set ui.config.logs.explorerUrl=http://grafana:3000 \
    | yq "$UI_DEPLOY | .spec.template.metadata.annotations | has(\"checksum/config\")")"
check "ui image" \
  "ghcr.io/naira-project/naira-ui:${APP_VERSION}" \
  "$(render | yq "$UI | .image")"
check "ui Service" \
  "ui 80" \
  "$(render \
    | yq 'select(.kind == "Service" and .metadata.name == "ui")
          | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "portal token URL from keycloak baseUrl and realm" \
  "http://keycloak.naira-deps.svc.cluster.local:8080/realms/naira/protocol/openid-connect/token" \
  "$(render | yq "$PORTAL | .env[] | select(.name == \"TOKEN_URL_KEYCLOAK\") | .value")"
check "portal client secret from portal-oidc" \
  "portal-oidc/client-secret" \
  "$(render \
    | yq "$PORTAL | .env[] | select(.name == \"OIDC_CLIENT_SECRET_KEYCLOAK\")
          | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "portal Service" \
  "portal 3000" \
  "$(render \
    | yq 'select(.kind == "Service" and .metadata.name == "portal")
          | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "ui disabled" \
  "" \
  "$(render --set ui.enabled=false \
    | yq 'select(.kind == "Deployment" and .metadata.name == "ui") | .metadata.name')"

# ── Guards ───────────────────────────────────────────────────────────────────
must_fail "duplicate plugin port" \
  "both use port 50051" \
  --set catalog.plugins.mlflow.port=50051
must_fail "plugin port equal to the catalog port" \
  "catalog and mlflow both use port 8090" \
  --set catalog.plugins.mlflow.port=8090
check "numeric image tag renders as a string" \
  "ghcr.io/naira-project/naira-catalog:20260924" \
  "$(render --set image.tag=20260924 | yq "$CATALOG | .containers[0].image")"
must_fail "catalog secret: existing and create" \
  "catalog.secret: set existingSecret or create, not both" \
  --set catalog.secret.existingSecret=x --set catalog.secret.create=true
must_fail "portal secret: existing and create" \
  "portal.oidc.secret: set existingSecret or create, not both" \
  --set portal.oidc.secret.existingSecret=x --set portal.oidc.secret.create=true
must_fail "tech-radar on without a radar file" \
  "catalog.plugins.tech-radar.config: set existingConfigMap or data" \
  --set catalog.plugins.tech-radar.enabled=true
RENDER=render_raw must_fail "no catalog Secret source" \
  "catalog.plugins.litellm reads LITELLM_API_KEY from the catalog Secret: set catalog.secret.existingSecret or catalog.secret.create" \
  --set portal.enabled=false
RENDER=render_raw must_fail "no portal Secret source" \
  "portal reads its OIDC client secret: set portal.oidc.secret.existingSecret or portal.oidc.secret.create" \
  --set catalog.secret.existingSecret=x
check "no Secret needed when nothing reads one" \
  "ok" \
  "$(render_raw "${DB[@]}" --set portal.enabled=false \
      --set catalog.plugins.litellm.enabled=false \
      --set catalog.plugins.openmetadata.enabled=false >/dev/null && echo ok)"
check "a plugin naming its own Secret needs no catalog Secret" \
  "ok" \
  "$(render_raw "${DB[@]}" --set portal.enabled=false \
      --set catalog.plugins.openmetadata.enabled=false \
      --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.name=own >/dev/null && echo ok)"
check "disabled plugin may share a port" \
  "ok" \
  "$(render --set catalog.plugins.mlflow.port=50051 \
      --set catalog.plugins.mlflow.enabled=false >/dev/null && echo ok)"

# ── Dev values (kind) ────────────────────────────────────────────────────────
DEV=(-f "$CHART/values-dev.yaml")

check "dev: local image names" \
  "naira-catalog:${APP_VERSION}" \
  "$(render_raw "${DEV[@]}" | yq "$CATALOG | .containers[0].image")"
check "dev: sidecars (openmetadata off, tech-radar on)" \
  "plugin-depl-calls-svc,plugin-depl-uses-litellm,plugin-fluxcd,plugin-litellm,plugin-mcp-servers,plugin-mlflow,plugin-tech-radar" \
  "$(render_raw "${DEV[@]}" | yq ea "[$CATALOG | .initContainers[].name] | join(\",\")")"
check "dev: catalog Secret matches test-dependencies master key" \
  "sk-local-litellm" \
  "$(render_raw "${DEV[@]}" \
    | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "dev: portal Secret" \
  "naira-local-dev-secret" \
  "$(render_raw "${DEV[@]}" \
    | yq 'select(.kind == "Secret" and .metadata.name == "portal-oidc") | .stringData["client-secret"]')"
check "dev: radar file is main's" \
  "Naira Tech Radar" \
  "$(render_raw "${DEV[@]}" | yq "$TECH_RADAR_CONFIGMAP | .data[\"radar.yaml\"]" | yq '.radar.title')"
check "helm lint default values" \
  "ok" \
  "$(helm lint "$CHART" "${BASE[@]}" >/dev/null && echo ok)"
check "helm lint dev values" \
  "ok" \
  "$(helm lint "$CHART" "${DEV[@]}" >/dev/null && echo ok)"
check "helm lint all-fields values" \
  "ok" \
  "$(helm lint "$CHART" -f "$CHART/ci/all-fields-values.yaml" >/dev/null && echo ok)"

# ── Edge cases ───────────────────────────────────────────────────────────────
check "numeric release name renders a string label" \
  "string" \
  "$(helm template 123 "$CHART" --namespace naira "${BASE[@]}" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "catalog")
          | .metadata.labels["app.kubernetes.io/instance"] | tag' \
    | sed 's/!!//;s/str/string/')"
check "rbac without rules renders no ClusterRole" \
  "" \
  "$(render --set catalog.plugins.mlflow.rbac.foo=bar \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-mlflow") | .metadata.name')"
must_fail "plugin without an image" \
  "image.repository is required" \
  --set catalog.plugins.newone.enabled=true --set catalog.plugins.newone.port=50099
must_fail "secretKeyRef without key" \
  "secretKeyRef.key is required" \
  --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.key=null
RENDER=render_raw must_fail "create with empty data" \
  "catalog.secret.data is empty" \
  --set portal.enabled=false --set catalog.secret.create=true
RENDER=render_raw must_fail "portal create with empty value" \
  "portal.oidc.secret.value is empty" \
  --set catalog.enabled=false --set portal.oidc.secret.create=true

GITHUB=(--set catalog.plugins.github.enabled=true)
GITHUB_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-github\")"

check "github off by default" \
  "" \
  "$(render | yq "$GITHUB_SIDECAR | .name")"
check "github token from the catalog Secret" \
  "catalog-secrets/GITHUB_TOKEN" \
  "$(render "${GITHUB[@]}" \
    | yq "$GITHUB_SIDECAR | .env[] | select(.name == \"GITHUB_TOKEN\")
          | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "github registered in plugins.yaml" \
  "localhost:50059" \
  "$(render "${GITHUB[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.github.address')"
check "github RBAC when enabled" \
  "t-plugin-github" \
  "$(render "${GITHUB[@]}" \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-github") | .metadata.name')"
RENDER=render_raw must_fail "github on without a catalog Secret source" \
  "catalog.plugins.github reads GITHUB_TOKEN from the catalog Secret" \
  --set portal.enabled=false --set catalog.plugins.github.enabled=true \
  --set catalog.plugins.litellm.enabled=false --set catalog.plugins.openmetadata.enabled=false

BEDROCK=(--set catalog.plugins.bedrock.enabled=true)
BEDROCK_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-bedrock\")"

check "bedrock off by default" \
  "" \
  "$(render | yq "$BEDROCK_SIDECAR | .name")"
check "bedrock AWS credentials from the catalog Secret" \
  "catalog-secrets/BEDROCK_AWS_ACCESS_KEY_ID catalog-secrets/BEDROCK_AWS_SECRET_ACCESS_KEY" \
  "$(render "${BEDROCK[@]}" \
    | yq "$BEDROCK_SIDECAR | .env[] | select(.name == \"AWS_ACCESS_KEY_ID\" or .name == \"AWS_SECRET_ACCESS_KEY\")
          | .valueFrom.secretKeyRef | .name + \"/\" + .key" | paste -sd' ' -)"
check "bedrock registered in plugins.yaml" \
  "localhost:50060" \
  "$(render "${BEDROCK[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.bedrock.address')"
check "bedrock needs no RBAC" \
  "" \
  "$(render "${BEDROCK[@]}" \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-bedrock") | .metadata.name')"
RENDER=render_raw must_fail "bedrock on without a catalog Secret source" \
  "catalog.plugins.bedrock reads AWS_ACCESS_KEY_ID from the catalog Secret" \
  --set portal.enabled=false --set catalog.plugins.bedrock.enabled=true \
  --set catalog.plugins.litellm.enabled=false --set catalog.plugins.openmetadata.enabled=false

check "depl-uses-litellm off by default: no Secret read grant" \
  "0" \
  "$(render | yq ea '[select(.kind == "ClusterRole") | .rules[] | select(.resources[] == "secrets")] | length')"
check "depl-uses-litellm enabled: ClusterRole with secrets" \
  "t-plugin-depl-uses-litellm" \
  "$(render --set catalog.plugins.depl-uses-litellm.enabled=true \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-uses-litellm") | .metadata.name')"

# 57 characters: "plugin-" plus the name goes over the 63 of a container name.
LONG=$(printf 'a%.0s' {1..57})
must_fail "plugin name over 56 characters" \
  "name too long" \
  --set "catalog.plugins.${LONG}.enabled=true" \
  --set "catalog.plugins.${LONG}.port=50099" \
  --set "catalog.plugins.${LONG}.image.repository=x"

# ── Pod hardening and scheduling ─────────────────────────────────────────────
check "catalog runs as the distroless uid" \
  "65532 true" \
  "$(render \
    | yq "$(pod catalog) | .containers[0].securityContext
          | (.runAsUser | tostring) + \" \" + (.runAsNonRoot | tostring)")"
check "sidecar security context defaults" \
  "false ALL RuntimeDefault" \
  "$(render \
    | yq "$LITELLM_SIDECAR | .securityContext
          | (.allowPrivilegeEscalation | tostring) + \" \" + .capabilities.drop[0] + \" \" + .seccompProfile.type")"
check "plugin securityContext merges over defaults" \
  "true true" \
  "$(render --set catalog.plugins.litellm.securityContext.readOnlyRootFilesystem=true \
    | yq "$LITELLM_SIDECAR | .securityContext
          | (.readOnlyRootFilesystem | tostring) + \" \" + (.runAsNonRoot | tostring)")"
check "catalog startup probe" \
  "/healthz" \
  "$(render | yq "$(pod catalog) | .containers[0].startupProbe.httpGet.path")"
check "ui startup probe" \
  "/" \
  "$(render | yq "$(pod ui) | .containers[0].startupProbe.httpGet.path")"
check "portal startup probe" \
  "/" \
  "$(render | yq "$(pod portal) | .containers[0].startupProbe.httpGet.path")"
check "ui does not drop capabilities" \
  "null" \
  "$(render | yq "$(pod ui) | .containers[0].securityContext.capabilities")"
check "portal seccomp profile" \
  "RuntimeDefault" \
  "$(render | yq "$(pod portal) | .containers[0].securityContext.seccompProfile.type")"
check "no scheduling fields by default" \
  "false" \
  "$(render \
    | yq "$(pod catalog)
          | (has(\"imagePullSecrets\") or has(\"nodeSelector\") or has(\"tolerations\")
             or has(\"affinity\") or has(\"priorityClassName\") or has(\"securityContext\"))
          | tostring")"

for workload in catalog ui portal; do
  check "$workload imagePullSecrets" \
    "regcred" \
    "$(render --set 'imagePullSecrets[0].name=regcred' \
      | yq "$(pod "$workload") | .imagePullSecrets[0].name")"
  check "$workload podSecurityContext" \
    "1000" \
    "$(render --set "$workload.podSecurityContext.fsGroup=1000" \
      | yq "$(pod "$workload") | .securityContext.fsGroup")"
  check "$workload priorityClassName" \
    "high" \
    "$(render --set "$workload.priorityClassName=high" \
      | yq "$(pod "$workload") | .priorityClassName")"
  check "$workload nodeSelector" \
    "linux" \
    "$(render --set "$workload.nodeSelector.os=linux" \
      | yq "$(pod "$workload") | .nodeSelector.os")"
  check "$workload tolerations" \
    "dedicated" \
    "$(render --set "$workload.tolerations[0].key=dedicated" \
      | yq "$(pod "$workload") | .tolerations[0].key")"
  check "$workload affinity" \
    "amd64" \
    "$(render --set "$workload.affinity.nodeAffinity.arch=amd64" \
      | yq "$(pod "$workload") | .affinity.nodeAffinity.arch")"
  check "$workload pod annotations" \
    "v" \
    "$(render --set-string "$workload.podAnnotations.k=v" \
      | yq "select(.kind == \"Deployment\" and .metadata.name == \"$workload\")
            | .spec.template.metadata.annotations.k")"
done

check "catalog keeps its checksum annotation with podAnnotations" \
  "true" \
  "$(render --set-string catalog.podAnnotations.k=v \
    | yq 'select(.kind == "Deployment" and .metadata.name == "catalog")
          | .spec.template.metadata.annotations | has("checksum/plugin-config")')"

# ── NetworkPolicy ────────────────────────────────────────────────────────────
NETWORK_POLICY='select(.kind == "NetworkPolicy") | .spec'

check "no NetworkPolicy by default" \
  "0" \
  "$(render | yq ea '[select(.kind == "NetworkPolicy")] | length')"
check "NetworkPolicy admits the ui pods on the catalog port" \
  "ui 8090" \
  "$(render --set catalog.networkPolicy.enabled=true \
    | yq "$NETWORK_POLICY | .ingress[0]
          | .from[0].podSelector.matchLabels[\"app.kubernetes.io/name\"] + \" \" + (.ports[0].port | tostring)")"
check "NetworkPolicy selects the catalog pods" \
  "catalog" \
  "$(render --set catalog.networkPolicy.enabled=true \
    | yq "$NETWORK_POLICY | .podSelector.matchLabels[\"app.kubernetes.io/name\"]")"
check "NetworkPolicy with the ui disabled admits nobody" \
  "false Ingress" \
  "$(render --set ui.enabled=false --set catalog.networkPolicy.enabled=true \
    | yq "$NETWORK_POLICY | (has(\"ingress\") | tostring) + \" \" + .policyTypes[0]")"
check "NetworkPolicy extraIngress is appended" \
  "2 ingress-nginx" \
  "$(render --set catalog.networkPolicy.enabled=true \
      --set 'catalog.networkPolicy.extraIngress[0].from[0].namespaceSelector.matchLabels.n=ingress-nginx' \
    | yq "$NETWORK_POLICY | (.ingress | length | tostring) + \" \"
          + .ingress[1].from[0].namespaceSelector.matchLabels.n")"
# ── PostgreSQL and db-init ───────────────────────────────────────────────────
DB_JOB='select(.kind == "Job") | .spec.template.spec.containers[0]'
db_job_name() { render "$@" | yq 'select(.kind == "Job") | .metadata.name'; }

check "postgres reads the admin Secret, renders none of its own" \
  "postgres-admin|0" \
  "$(o=$(render); echo "$o" | yq 'select(.kind == "StatefulSet" and .metadata.name == "postgres")
       | .spec.template.spec.containers[0].env[] | select(.name == "POSTGRES_PASSWORD") | .valueFrom.secretKeyRef.name' \
     )|$(render | yq ea '[select(.kind == "Secret")] | length')"
check "db-init targets the subchart Service as the admin user" \
  "postgres.naira.svc.cluster.local postgres postgres-admin" \
  "$(render | yq "$DB_JOB | (.env[] | select(.name == \"PGHOST\") | .value) + \" \"
       + (.env[] | select(.name == \"PGUSER\") | .value) + \" \"
       + (.env[] | select(.name == \"PGPASSWORD\") | .valueFrom.secretKeyRef.name)")"
check "db-init reads each database password from its Secret" \
  "catalog-db" \
  "$(render | yq "$DB_JOB | .env[] | select(.name == \"DB_PASSWORD_0\") | .valueFrom.secretKeyRef.name")"
check "dev: db-init ensures catalog, langfuse, litellm and openmetadata" \
  "catalog,langfuse,litellm,openmetadata" \
  "$(render_raw "${DEV[@]}" | yq "$DB_JOB | .args | join(\",\")")"
check "dev: litellm Secret has username and password" \
  "litellm litellm-local-password" \
  "$(render_raw "${DEV[@]}" | yq 'select(.kind == "Secret" and .metadata.name == "litellm-db") | .stringData.username + " " + .stringData.password')"
check "external server: no StatefulSet, db-init uses database.host" \
  "0 pg.example.com" \
  "$(o=$(render --set postgresql.enabled=false --set database.host=pg.example.com)
     echo "$(echo "$o" | yq ea '[select(.kind == "StatefulSet")] | length') $(echo "$o" | yq "$DB_JOB | .env[] | select(.name == \"PGHOST\") | .value")")"
check "no databases and no subchart: no db-init, no Secret needed" \
  "0" \
  "$(render_raw --set postgresql.enabled=false --set database.databases.catalog.enabled=false \
      -f "$CHART/ci/default-values.yaml" --set database.admin.secret.existingSecret= \
    | yq ea '[select(.kind == "Job" or .kind == "ConfigMap" and .metadata.name == "db-init")] | length')"
check "db-init Job name is stable across renders" \
  "$(db_job_name)" \
  "$(db_job_name)"
if [ "$(db_job_name)" = "$(db_job_name --set database.databases.extra.enabled=true \
      --set database.databases.extra.secret.existingSecret=extra-db)" ]; then
  echo "FAIL db-init Job name changes with its databases"; fail=1
else
  echo "ok   db-init Job name changes with its databases"
fi
must_fail "database needs a Secret source" "database.databases.catalog: set secret.existingSecret or secret.create" \
  --set database.databases.catalog.secret.existingSecret=
must_fail "database without a secret block" "database.databases.extra: set secret.existingSecret or secret.create" \
  --set database.databases.extra.enabled=true
must_fail "database Secret create needs a password" "database.databases.catalog.secret.create is true but password is empty" \
  --set database.databases.catalog.secret.existingSecret= --set database.databases.catalog.secret.create=true
must_fail "admin Secret must match the subchart's" "postgresql.auth.existingSecret must equal" \
  --set database.admin.secret.existingSecret=other
must_fail "external server needs a host" "set database.host" \
  --set postgresql.enabled=false
must_fail "subchart and host are exclusive" "database.host: leave empty" \
  --set database.host=pg.example.com
must_fail "database name is checked" "name must match" \
  --set database.databases.Bad_Name.enabled=true --set database.databases.Bad_Name.secret.existingSecret=x
for reserved in postgres template0 template1; do
  must_fail "database name $reserved is reserved" "reserved name" \
    --set "database.databases.$reserved.enabled=true" --set "database.databases.$reserved.secret.existingSecret=x"
done
must_fail "database named like the admin is reserved" "reserved name" --set database.admin.username=dbadmin \
  --set database.databases.dbadmin.enabled=true --set database.databases.dbadmin.secret.existingSecret=x
check "db-init gets sslmode, retries and a deadline" "prefer 60 900" \
  "$(render | yq ea '[select(.kind == "Job")] | .[0]
       | [(.spec.template.spec.containers[0].env[] | select(.name == "PGSSLMODE" or .name == "DB_WAIT_RETRIES") | .value),
          (.spec.activeDeadlineSeconds | tostring)] | join(" ")')"

# ── Handbooks ────────────────────────────────────────────────────────────────────
DIGEST=$(printf 'a%.0s' {1..64})
# handbook ID [EXTRA JSON FIELDS]: one handbooks[] entry that is valid until EXTRA overrides
# a field (the last duplicate key wins).
handbook() {
  local id=$1 extra=${2:+,$2}
  printf '{"id":"%s","contract":"naira.handbook/v1alpha1","image":"ghcr.io/naira-project/%s@sha256:%s"%s}' \
    "$id" "$id" "$DIGEST" "$extra"
}
# render_handbooks ENTRY...: render with exactly these handbooks.
render_handbooks() {
  local entries
  entries=$(IFS=,; echo "$*")
  render --set-json "handbooks=[$entries]"
}
# The gateway is also named handbook-*, so handbook workloads are told apart by their component label.
HANDBOOK_DEPLOYMENTS='select(.kind == "Deployment" and .metadata.labels["app.kubernetes.io/component"] == "handbook") | .metadata.name'
HANDBOOK_SERVICES='select(.kind == "Service" and .metadata.labels["app.kubernetes.io/component"] == "handbook") | .metadata.name'
HANDBOOKS_JSON='select(.kind == "ConfigMap" and .metadata.name == "handbooks") | .data["handbooks.json"] | from_json'
HANDBOOK_POD() { printf 'select(.kind == "Deployment" and .metadata.name == "handbook-%s") | .spec.template.spec' "$1"; }

check "no handbooks by default: no Deployment, no Service" \
  "0 0" \
  "$(o=$(render); echo "$(echo "$o" | yq ea "[$HANDBOOK_DEPLOYMENTS] | length") $(echo "$o" | yq ea "[$HANDBOOK_SERVICES] | length")")"
check "no handbooks by default: handbooks ConfigMap holds an empty list" \
  "[]" \
  "$(render | yq "$HANDBOOKS_JSON" -o=json -I=0)"
check "handbooks ConfigMap is in the release namespace" \
  "naira" \
  "$(render | yq 'select(.kind == "ConfigMap" and .metadata.name == "handbooks") | .metadata.namespace')"

check "handbook: Deployment and Service named handbook-<id>" \
  "handbook-platform handbook-platform" \
  "$(o=$(render_handbooks "$(handbook platform)")
     echo "$(echo "$o" | yq "$HANDBOOK_DEPLOYMENTS") $(echo "$o" | yq "$HANDBOOK_SERVICES")")"
check "handbook: image is used as given, by digest, not built from the chart's image values" \
  "ghcr.io/naira-project/platform@sha256:${DIGEST}" \
  "$(render_handbooks "$(handbook platform)" | yq "$(HANDBOOK_POD platform) | .containers[0].image")"
check "handbook: image ignores image.registry and image.tag" \
  "ghcr.io/naira-project/platform@sha256:${DIGEST}" \
  "$(render --set image.registry=other.example --set image.tag=zz \
      --set-json "handbooks=[$(handbook platform)]" \
    | yq "$(HANDBOOK_POD platform) | .containers[0].image")"
check "handbook: readiness on naira-handbook.json, port 8080" \
  "/naira-handbook.json 8080" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform) | .containers[0]
          | .readinessProbe.httpGet.path + \" \" + (.ports[0].containerPort | tostring)")"
check "handbook: restricted security context" \
  "false true true ALL RuntimeDefault" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform) | .containers[0].securityContext
          | (.allowPrivilegeEscalation | tostring) + \" \" + (.readOnlyRootFilesystem | tostring) + \" \"
            + (.runAsNonRoot | tostring) + \" \" + .capabilities.drop[0] + \" \" + .seccompProfile.type")"
check "handbook: does not pin a uid, the image contract only says non-root" \
  "null" \
  "$(render_handbooks "$(handbook platform)" | yq "$(HANDBOOK_POD platform) | .containers[0].securityContext.runAsUser")"
check "handbook: writable /tmp is a small emptyDir" \
  "/tmp tmp true" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform)
          | .containers[0].volumeMounts[0].mountPath + \" \" + .volumes[0].name + \" \"
            + (.volumes[0].emptyDir | has(\"sizeLimit\") | tostring)")"
check "handbook: no API token and not the catalog ServiceAccount" \
  "false null" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform) | (.automountServiceAccountToken | tostring) + \" \" + (.serviceAccountName // \"null\")")"
check "handbook: small resources" \
  "100m 64Mi" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform) | .containers[0].resources.limits | .cpu + \" \" + .memory")"
check "handbook: Service on 8080 selects only that handbook's pods" \
  "8080 handbook-platform" \
  "$(render_handbooks "$(handbook platform)" "$(handbook other)" \
    | yq 'select(.kind == "Service" and .metadata.name == "handbook-platform")
          | (.spec.ports[0].port | tostring) + " " + .spec.selector["app.kubernetes.io/name"]')"
check "handbook: pods carry the handbook component label" \
  "handbook" \
  "$(render_handbooks "$(handbook platform)" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "handbook-platform")
          | .spec.template.metadata.labels["app.kubernetes.io/component"]')"
check "handbook: component label is not part of the selector" \
  "false" \
  "$(render_handbooks "$(handbook platform)" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "handbook-platform")
          | .spec.selector.matchLabels | has("app.kubernetes.io/component") | tostring')"
check "handbook: replicas default to 1" \
  "1" \
  "$(render_handbooks "$(handbook platform)" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "handbook-platform") | .spec.replicas')"
check "handbook: replicas from handbookDefaults" \
  "2" \
  "$(render --set handbookDefaults.replicas=2 --set-json "handbooks=[$(handbook platform)]" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "handbook-platform") | .spec.replicas')"
check "handbook: imagePullSecrets apply" \
  "regcred" \
  "$(render --set 'imagePullSecrets[0].name=regcred' --set-json "handbooks=[$(handbook platform)]" \
    | yq "$(HANDBOOK_POD platform) | .imagePullSecrets[0].name")"
check "handbook: no scheduling fields by default" \
  "false" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$(HANDBOOK_POD platform)
          | (has(\"imagePullSecrets\") or has(\"nodeSelector\") or has(\"tolerations\")
             or has(\"affinity\") or has(\"priorityClassName\") or has(\"securityContext\")) | tostring")"
check "handbook: scheduling fields from handbookDefaults" \
  "linux dedicated high 1000" \
  "$(render --set handbookDefaults.nodeSelector.os=linux --set 'handbookDefaults.tolerations[0].key=dedicated' \
      --set handbookDefaults.priorityClassName=high --set handbookDefaults.podSecurityContext.fsGroup=1000 \
      --set-json "handbooks=[$(handbook platform)]" \
    | yq "$(HANDBOOK_POD platform)
          | .nodeSelector.os + \" \" + .tolerations[0].key + \" \" + .priorityClassName + \" \"
            + (.securityContext.fsGroup | tostring)")"

# ConfigMap: enabled handbooks only, sorted primary first, then order, then id.
check "handbooks.json entry: id, label, url, primary, order" \
  '{"id":"platform","label":"Platform Handbook","order":3,"primary":true,"url":"http://platform.handbook.localhost:3002/"}' \
  "$(render_handbooks "$(handbook platform '"label":"Platform Handbook","primary":true,"order":3')" \
    | yq "$HANDBOOKS_JSON | .[0]" -o=json -I=0)"
check "handbooks.json: label defaults to the id, order to 0, primary to false" \
  "platform 0 false" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$HANDBOOKS_JSON | .[0] | .label + \" \" + (.order | tostring) + \" \" + (.primary | tostring)")"
check "handbooks.json: owner and source appear when set" \
  "team-platform https://example.com/hb" \
  "$(render_handbooks "$(handbook platform '"owner":"team-platform","source":"https://example.com/hb"')" \
    | yq "$HANDBOOKS_JSON | .[0] | .owner + \" \" + .source")"
check "handbooks.json: no owner and source when unset" \
  "false false" \
  "$(render_handbooks "$(handbook platform)" \
    | yq "$HANDBOOKS_JSON | .[0] | (has(\"owner\") | tostring) + \" \" + (has(\"source\") | tostring)")"
check "handbooks.json: primary first, then order, then id" \
  "primary-one,a-early,b-early,c-late" \
  "$(render_handbooks "$(handbook c-late '"order":5')" "$(handbook b-early '"order":1')" \
      "$(handbook primary-one '"primary":true,"order":9')" "$(handbook a-early '"order":1')" \
    | yq "$HANDBOOKS_JSON | map(.id) | join(\",\")")"
check "handbooks.json: negative and missing order sort before positive" \
  "neg,none,pos" \
  "$(render_handbooks "$(handbook pos '"order":2')" "$(handbook none)" "$(handbook neg '"order":-3')" \
    | yq "$HANDBOOKS_JSON | map(.id) | join(\",\")")"
check "handbooks.json: negative orders sort as numbers" \
  "ten-down,three-down" \
  "$(render_handbooks "$(handbook three-down '"order":-3')" "$(handbook ten-down '"order":-10')" \
    | yq "$HANDBOOKS_JSON | map(.id) | join(\",\")")"
check "handbooks.json: ids sort as text, order as a number" \
  "two,ten" \
  "$(render_handbooks "$(handbook ten '"order":10')" "$(handbook two '"order":2')" \
    | yq "$HANDBOOKS_JSON | map(.id) | join(\",\")")"

# Address: <scheme>://<id>.<domain>[:port]/, the port left out when it is the scheme's default.
handbook_url() { render --set-json "handbooks=[$(handbook platform)]" "$@" | yq "$HANDBOOKS_JSON | .[0].url"; }
check "handbook URL: kind defaults" \
  "http://platform.handbook.localhost:3002/" \
  "$(handbook_url)"
check "handbook URL: https on 443 has no port" \
  "https://platform.handbook.example.com/" \
  "$(handbook_url --set handbookGateway.scheme=https --set handbookGateway.port=443 --set handbookGateway.domain=handbook.example.com)"
check "handbook URL: http on 80 has no port" \
  "http://platform.handbook.example.com/" \
  "$(handbook_url --set handbookGateway.port=80 --set handbookGateway.domain=handbook.example.com)"
check "handbook URL: https on another port keeps it" \
  "https://platform.handbook.example.com:8443/" \
  "$(handbook_url --set handbookGateway.scheme=https --set handbookGateway.port=8443 --set handbookGateway.domain=handbook.example.com)"
check "handbook URL: 443 on http is not a default port" \
  "http://platform.handbook.example.com:443/" \
  "$(handbook_url --set handbookGateway.port=443 --set handbookGateway.domain=handbook.example.com)"

# Lifecycle: enabled false and removal take the handbook's objects and entry away, nothing else.
check "disabled handbook: no Deployment, no Service, no entry" \
  "handbook-other|handbook-other|other" \
  "$(o=$(render_handbooks "$(handbook platform '"enabled":false')" "$(handbook other)")
     echo "$(echo "$o" | yq "$HANDBOOK_DEPLOYMENTS")|$(echo "$o" | yq "$HANDBOOK_SERVICES")|$(echo "$o" | yq "$HANDBOOKS_JSON | map(.id) | join(\",\")")")"
check "handbook without enabled is enabled" \
  "handbook-platform" \
  "$(render_handbooks "$(handbook platform)" | yq "$HANDBOOK_DEPLOYMENTS")"
check "all handbooks disabled: back to an empty list" \
  "0 []" \
  "$(o=$(render_handbooks "$(handbook platform '"enabled":false')")
     echo "$(echo "$o" | yq ea "[$HANDBOOK_DEPLOYMENTS] | length") $(echo "$o" | yq "$HANDBOOKS_JSON" -o=json -I=0)")"
check "removed handbook: its objects are gone, the other stays" \
  "handbook-other" \
  "$(render_handbooks "$(handbook other)" | yq "$HANDBOOK_DEPLOYMENTS")"

# Everything outside the handbook objects renders byte for byte the same with or without handbooks.
# The portal Deployment carries a checksum of handbooks.json, which is the one thing handbooks may change in it.
without_handbook_objects() {
  yq 'select((.metadata.name == "handbooks" or (.metadata.name | test("^handbook-"))) | not)
      | (select(.kind == "Deployment" and .metadata.name == "portal") | .spec.template.metadata.annotations) |= del(.["checksum/handbooks"])'
}
if [ "$(render | without_handbook_objects | sha256sum)" = \
     "$(render_handbooks "$(handbook platform)" "$(handbook other '"enabled":false')" | without_handbook_objects | sha256sum)" ]; then
  echo "ok   catalog, ui, portal and database objects are untouched by handbooks"
else
  echo "FAIL catalog, ui, portal and database objects are untouched by handbooks"; fail=1
fi
# Handbook guards. A bad entry fails the whole render, and the message names the handbook.
must_fail_handbooks() {
  local name=$1 want=$2 entries
  shift 2
  entries=$(IFS=,; echo "$*")
  must_fail "$name" "$want" --set-json "handbooks=[$entries]"
}
for bad in Bad -a a- a_b a.b "$(printf 'a%.0s' {1..43})"; do
  must_fail_handbooks "handbook id ${bad:0:12} is not a DNS label" "handbooks[0]: invalid id \"$bad\"" \
    "$(handbook x "\"id\":\"$bad\"")"
done
must_fail_handbooks "handbook without id" "handbooks[1]: id is required" \
  "$(handbook ok)" '{"contract":"naira.handbook/v1alpha1"}'
must_fail_handbooks "handbook entry that is not a map" "handbooks[0]: must be a map" '"platform"'
must_fail_handbooks "duplicate handbook id" "handbooks[platform]: duplicate id (entries 0 and 2)" \
  "$(handbook platform)" "$(handbook other)" "$(handbook platform)"
must_fail_handbooks "duplicate handbook id even when one is disabled" "handbooks[platform]: duplicate id" \
  "$(handbook platform)" "$(handbook platform '"enabled":false')"
must_fail_handbooks "handbook id gateway collides with the gateway objects" "handbooks[gateway]: id is reserved" \
  "$(handbook gateway)"
must_fail_handbooks "two primary handbooks" "handbooks: more than one primary handbook (a, b)" \
  "$(handbook a '"primary":true')" "$(handbook b '"primary":true')"
check "a disabled primary does not count" \
  "ok" \
  "$(render_handbooks "$(handbook a '"primary":true,"enabled":false')" "$(handbook b '"primary":true')" >/dev/null && echo ok)"
must_fail_handbooks "handbook without an image" "handbooks[platform]: image is required" \
  "$(handbook platform '"image":""')"
must_fail_handbooks "handbook image by tag only" "handbooks[platform]: image must be pinned by digest" \
  "$(handbook platform '"image":"ghcr.io/naira-project/platform:1.0"')"
must_fail_handbooks "handbook image with a short digest" "handbooks[platform]: image must be pinned by digest" \
  "$(handbook platform "\"image\":\"ghcr.io/naira-project/platform@sha256:${DIGEST:1}\"")"
must_fail_handbooks "handbook image with an uppercase digest" "handbooks[platform]: image must be pinned by digest" \
  "$(handbook platform "\"image\":\"ghcr.io/naira-project/platform@sha256:${DIGEST//a/A}\"")"
must_fail_handbooks "handbook image with another digest algorithm" "handbooks[platform]: image must be pinned by digest" \
  "$(handbook platform "\"image\":\"ghcr.io/naira-project/platform@sha512:${DIGEST}\"")"
check "a tag next to the digest is accepted" \
  "ok" \
  "$(render_handbooks "$(handbook platform "\"image\":\"ghcr.io/naira-project/platform:1.0@sha256:${DIGEST}\"")" >/dev/null && echo ok)"
must_fail_handbooks "handbook image from another registry" \
  "handbooks[platform]: image \"evil.example/platform@sha256:${DIGEST}\" is not from an allowed registry" \
  "$(handbook platform "\"image\":\"evil.example/platform@sha256:${DIGEST}\"")"
must_fail_handbooks "registry match stops at the path boundary" "handbooks[platform]: image" \
  "$(handbook platform "\"image\":\"ghcr.io/naira-project-evil/platform@sha256:${DIGEST}\"")"
must_fail_handbooks "registry match is not a substring match" "handbooks[platform]: image" \
  "$(handbook platform "\"image\":\"evil.example/ghcr.io/naira-project/platform@sha256:${DIGEST}\"")"
check "an allowed registry written with a trailing slash matches" \
  "ok" \
  "$(render --set 'handbookGateway.allowedRegistries[0]=ghcr.io/naira-project/' \
      --set-json "handbooks=[$(handbook platform)]" >/dev/null && echo ok)"
check "a second allowed registry matches" \
  "ok" \
  "$(render --set 'handbookGateway.allowedRegistries[0]=ghcr.io/naira-project' \
      --set 'handbookGateway.allowedRegistries[1]=reg.example.com/docs' \
      --set-json "handbooks=[$(handbook platform "\"image\":\"reg.example.com/docs/platform@sha256:${DIGEST}\"")]" \
    >/dev/null && echo ok)"
must_fail "no allowed registry allows no handbook" \
  "handbooks[platform]: handbookGateway.allowedRegistries is empty" \
  --set 'handbookGateway.allowedRegistries=null' --set-json "handbooks=[$(handbook platform)]"
must_fail_handbooks "unsupported handbook contract" \
  'handbooks[platform]: unsupported contract "naira.handbook/v2"; supported: naira.handbook/v1alpha1' \
  "$(handbook platform '"contract":"naira.handbook/v2"')"
must_fail_handbooks "handbook contract missing" 'handbooks[platform]: unsupported contract ""' \
  "$(handbook platform '"contract":""')"
must_fail_handbooks "handbook order is not a number" "handbooks[platform]: order must be a whole number" \
  "$(handbook platform '"order":"two"')"
must_fail_handbooks "handbook order is not whole" "handbooks[platform]: order must be a whole number" \
  "$(handbook platform '"order":1.5')"
must_fail_handbooks "handbook order is out of range" "handbooks[platform]: order must be a whole number" \
  "$(handbook platform '"order":100001')"
check "handbook order at the limits is accepted" \
  "ok" \
  "$(render_handbooks "$(handbook a '"order":-100000')" "$(handbook b '"order":100000')" >/dev/null && echo ok)"
check "a disabled handbook is only checked for its id" \
  "ok" \
  "$(render_handbooks "$(handbook platform '"enabled":false,"image":"nope","contract":"x","order":"y","primary":true')" \
      "$(handbook other '"primary":true')" >/dev/null && echo ok)"
must_fail "handbook address domain with a scheme" "handbookGateway.domain: \"https://handbook.example.com\"" \
  --set handbookGateway.domain=https://handbook.example.com --set-json "handbooks=[$(handbook platform)]"
must_fail "handbook address domain with a port" "handbookGateway.domain" \
  --set handbookGateway.domain=handbook.example.com:3002 --set-json "handbooks=[$(handbook platform)]"
must_fail "handbook address domain in uppercase" "handbookGateway.domain" \
  --set handbookGateway.domain=Handbook.Example.com --set-json "handbooks=[$(handbook platform)]"
must_fail "handbook address domain empty" "handbookGateway.domain" \
  --set handbookGateway.domain= --set-json "handbooks=[$(handbook platform)]"
must_fail "handbook address scheme" "handbookGateway.scheme: must be http or https, got \"ftp\"" \
  --set handbookGateway.scheme=ftp --set-json "handbooks=[$(handbook platform)]"
must_fail "handbook address scheme missing" "handbookGateway.scheme: must be http or https, got \"\"" \
  --set handbookGateway.scheme=null --set-json "handbooks=[$(handbook platform)]"
for port in 0 65536 abc 1.5; do
  must_fail "handbook address port $port" "handbookGateway.port: must be a whole number from 1 to 65535" \
    --set-string handbookGateway.port="$port" --set-json "handbooks=[$(handbook platform)]"
done
check "handbook address port as a number is accepted" \
  "ok" \
  "$(render --set handbookGateway.port=8443 --set-json "handbooks=[$(handbook platform)]" >/dev/null && echo ok)"
check "gateway address is not checked while no handbook is enabled" \
  "ok" \
  "$(render --set handbookGateway.domain=https://x --set handbookGateway.scheme=ftp --set handbookGateway.port=0 \
      --set 'handbookGateway.allowedRegistries=null' >/dev/null && echo ok)"

# Handbook NetworkPolicies: ingress to the handbook pods from the gateway pods only, and an egress
# allow-list for the gateway pods. The gateway Deployment is tested in its own section below.
render_handbook_np() {
  render --set handbookGateway.networkPolicy.enabled=true --set-json "handbooks=[$(handbook platform)]" "$@"
}
NP_NAMES='[select(.kind == "NetworkPolicy") | .metadata.name] | sort | join(",")'
np() { printf 'select(.kind == "NetworkPolicy" and .metadata.name == "%s") | .spec' "$1"; }

check "handbook NetworkPolicies are off by default" \
  "" \
  "$(render_handbooks "$(handbook platform)" | yq ea "$NP_NAMES")"
check "handbook NetworkPolicies: none without an enabled handbook" \
  "" \
  "$(render --set handbookGateway.networkPolicy.enabled=true | yq ea "$NP_NAMES")"
check "handbook NetworkPolicies: none when the only handbook is disabled" \
  "" \
  "$(render --set handbookGateway.networkPolicy.enabled=true \
      --set-json "handbooks=[$(handbook platform '"enabled":false')]" | yq ea "$NP_NAMES")"
check "handbook NetworkPolicies: ingress to the handbooks and egress from the gateway" \
  "handbook-gateway-egress,handbook-ingress" \
  "$(render_handbook_np | yq ea "$NP_NAMES")"
check "handbook NetworkPolicies sit next to the catalog's, which stays opt-in" \
  "catalog,handbook-gateway-egress,handbook-ingress" \
  "$(render_handbook_np --set catalog.networkPolicy.enabled=true | yq ea "$NP_NAMES")"
check "handbook NetworkPolicies are in the release namespace" \
  "naira" \
  "$(render_handbook_np | yq 'select(.kind == "NetworkPolicy" and .metadata.name == "handbook-ingress") | .metadata.namespace')"

check "handbook ingress: selects the handbook pods of this release, ingress only" \
  "Ingress handbook t" \
  "$(render_handbook_np | yq "$(np handbook-ingress)
      | (.policyTypes | join(\",\")) + \" \" + .podSelector.matchLabels[\"app.kubernetes.io/component\"]
        + \" \" + .podSelector.matchLabels[\"app.kubernetes.io/instance\"]")"
check "handbook ingress: one rule, from the gateway pods of this release, TCP 8080" \
  "1 handbook-gateway t TCP 8080" \
  "$(render_handbook_np | yq "$(np handbook-ingress) | .ingress
      | (length | tostring) + \" \" + .[0].from[0].podSelector.matchLabels[\"app.kubernetes.io/name\"]
        + \" \" + .[0].from[0].podSelector.matchLabels[\"app.kubernetes.io/instance\"]
        + \" \" + .[0].ports[0].protocol + \" \" + (.[0].ports[0].port | tostring)")"
check "handbook ingress: the rule names one source, so nothing else is admitted" \
  "1" \
  "$(render_handbook_np | yq "$(np handbook-ingress) | .ingress[0].from | length")"

check "gateway egress: selects the gateway pods of this release, egress only" \
  "Egress handbook-gateway t" \
  "$(render_handbook_np | yq "$(np handbook-gateway-egress)
      | (.policyTypes | join(\",\")) + \" \" + .podSelector.matchLabels[\"app.kubernetes.io/name\"]
        + \" \" + .podSelector.matchLabels[\"app.kubernetes.io/instance\"]")"
check "gateway egress: three rules" \
  "3" \
  "$(render_handbook_np | yq "$(np handbook-gateway-egress) | .egress | length")"
check "gateway egress: DNS on port 53, UDP and TCP, to any address" \
  "false UDP/53,TCP/53" \
  "$(render_handbook_np | yq "$(np handbook-gateway-egress) | .egress[0]
      | (has(\"to\") | tostring) + \" \" + ([.ports[] | .protocol + \"/\" + (.port | tostring)] | join(\",\"))")"
check "gateway egress: the handbook pods of this release on 8080" \
  "handbook t TCP 8080" \
  "$(render_handbook_np | yq "$(np handbook-gateway-egress) | .egress[1]
      | .to[0].podSelector.matchLabels[\"app.kubernetes.io/component\"]
        + \" \" + .to[0].podSelector.matchLabels[\"app.kubernetes.io/instance\"]
        + \" \" + .ports[0].protocol + \" \" + (.ports[0].port | tostring)")"
check "gateway egress: the Keycloak port from keycloak.baseUrl, to any address" \
  "false TCP 8080" \
  "$(render_handbook_np | yq "$(np handbook-gateway-egress) | .egress[2]
      | (has(\"to\") | tostring) + \" \" + .ports[0].protocol + \" \" + (.ports[0].port | tostring)")"
keycloak_port() {
  render_handbook_np --set keycloak.baseUrl="$1" | yq "$(np handbook-gateway-egress) | .egress[2].ports[0].port"
}
check "gateway egress: https without a port means 443" "443" "$(keycloak_port https://keycloak.example.com)"
check "gateway egress: http without a port means 80" "80" "$(keycloak_port http://keycloak.example.com)"
check "gateway egress: an explicit port wins, a path is ignored" \
  "8443" "$(keycloak_port https://keycloak.example.com:8443/auth)"
check "gateway egress: an IPv6 host keeps its port" "8080" "$(keycloak_port 'http://[fd00::1]:8080')"
check "gateway egress: a malformed handbook entry fails on its own guard, not on the policy" \
  "ok" \
  "$(out=$(render --set handbookGateway.networkPolicy.enabled=true --set-json 'handbooks=["platform"]' 2>&1) \
     || { [[ $out == *"handbooks[0]: must be a map"* ]] && echo ok; })"

# ── Handbook gateway ─────────────────────────────────────────────────────────────
# envoy.yaml is a string in a ConfigMap; yq reads it back as YAML. That Envoy accepts it is
# checked by running Envoy (`envoy --mode validate`), not here.
GW_ENVOY='select(.kind == "ConfigMap" and .metadata.name == "handbook-gateway") | .data["envoy.yaml"] | from_yaml'
# gwq EXPR: query the Envoy config of the rendered chart on stdin. Two passes, because yq
# builds an empty array for every other document of the stream when EXPR collects.
gwq() { yq "$GW_ENVOY" | yq "$1"; }
GW_OBJECTS='[select(.metadata.name == "handbook-gateway") | .kind] | sort | join(",")'
GW_DEPLOYMENT='select(.kind == "Deployment" and .metadata.name == "handbook-gateway")'
GW_POD="$GW_DEPLOYMENT | .spec.template.spec"
HCM='.static_resources.listeners[0].filter_chains[0].filters[0].typed_config'
VHOSTS="${HCM}.route_config.virtual_hosts"
JWT_PER_ROUTE='.typed_per_filter_config["envoy.filters.http.jwt_authn"]'
gw_render() { render --set-json "handbooks=[$(handbook platform),$(handbook other)]" "$@"; }
CFG=$(gw_render | yq "$GW_ENVOY")
# cfg EXPR: query the Envoy config rendered for the handbooks platform and other.
cfg() { echo "$CFG" | yq "$1"; }
# vhost NAME: yq path to one virtual host.
vhost() { printf '%s[] | select(.name == "%s")' "$VHOSTS" "$1"; }
# jwt_summary VHOST: each route as path:requirement, "disabled" where no JWT is checked.
jwt_summary() {
  cfg "$(vhost "$1") | [.routes[] | (.match.path // .match.prefix) + \":\"
    + (${JWT_PER_ROUTE} | (.requirement_name // (select(.disabled) | \"disabled\") // \"none\"))] | join(\" \")"
}

check "gateway: nothing is rendered without handbooks" "" "$(render | yq ea "$GW_OBJECTS")"
check "gateway: nothing is rendered while every handbook is disabled" "" \
  "$(render_handbooks "$(handbook platform '"enabled":false')" | yq ea "$GW_OBJECTS")"
check "gateway: Deployment, Service and ConfigMap handbook-gateway" "ConfigMap,Deployment,Service" \
  "$(gw_render | yq ea "$GW_OBJECTS")"
check "gateway: objects are in the release namespace" "naira naira naira" \
  "$(gw_render | yq ea '[select(.metadata.name == "handbook-gateway") | .metadata.namespace] | join(" ")')"

# Virtual hosts and clusters: one per enabled handbook, plus the default host and the JWKS cluster.
check "gateway: one virtual host per enabled handbook, sorted by id, then the default" \
  "handbook-alpha,handbook-zeta,default" \
  "$(render_handbooks "$(handbook zeta)" "$(handbook alpha)" "$(handbook off '"enabled":false')" \
    | gwq "[${VHOSTS}[].name] | join(\",\")")"
for n in 1 2 3; do
  entries=()
  for i in $(seq 1 "$n"); do entries+=("$(handbook "w$i")"); done
  check "gateway: $n enabled handbooks give $((n + 1)) virtual hosts and $((n + 1)) clusters" \
    "$((n + 1)) $((n + 1))" \
    "$(render_handbooks "${entries[@]}" \
      | gwq "([${VHOSTS}[]] | length | tostring) + \" \" + ([.static_resources.clusters[]] | length | tostring)")"
done
check "gateway: clusters are the JWKS cluster and one per handbook" \
  "handbook-other,handbook-platform,keycloak-jwks" \
  "$(cfg '[.static_resources.clusters[].name] | sort | join(",")')"
check "gateway: a disabled handbook has no virtual host and no cluster" \
  "handbook-other|handbook-other,keycloak-jwks" \
  "$(c=$(render_handbooks "$(handbook platform '"enabled":false')" "$(handbook other)" | yq "$GW_ENVOY")
     echo "$(echo "$c" | yq "[${VHOSTS}[].name | select(. != \"default\")] | join(\",\")")|$(echo "$c" | yq '[.static_resources.clusters[].name] | sort | join(",")')")"
check "gateway: a virtual host answers to <id>.<domain> with and without the port" \
  "platform.handbook.localhost,platform.handbook.localhost:3002" \
  "$(cfg "$(vhost handbook-platform) | .domains | join(\",\")")"
check "gateway: the domains follow handbookGateway.domain and port" \
  "platform.handbook.example.com,platform.handbook.example.com:8443" \
  "$(gw_render --set handbookGateway.domain=handbook.example.com --set handbookGateway.port=8443 --set handbookGateway.scheme=https \
    | gwq "$(vhost handbook-platform) | .domains | join(\",\")")"
check "gateway: unknown hosts get a 404 without a JWT check" \
  "* 404 true" \
  "$(cfg "$(vhost default) | (.domains | join(\",\")) + \" \" + (.routes[0].direct_response.status | tostring)
    + \" \" + (.routes[0]${JWT_PER_ROUTE}.disabled | tostring)")"
check "gateway: the default host is the last one, so no handbook host is shadowed" \
  "default" \
  "$(cfg "[${VHOSTS}[].name] | .[-1]")"
for id in platform other; do
  check "gateway: handbook $id routes only to its own cluster" \
    "handbook-$id" \
    "$(cfg "$(vhost "handbook-$id") | [.routes[].route.cluster | select(. != null)] | unique | join(\",\")")"
  check "gateway: handbook $id cluster is its Service on 8080, health-checked on naira-handbook.json" \
    "STRICT_DNS handbook-$id.naira.svc.cluster.local 8080 /naira-handbook.json" \
    "$(cfg ".static_resources.clusters[] | select(.name == \"handbook-$id\")
      | .type + \" \" + .load_assignment.endpoints[0].lb_endpoints[0].endpoint.address.socket_address.address
        + \" \" + (.load_assignment.endpoints[0].lb_endpoints[0].endpoint.address.socket_address.port_value | tostring)
        + \" \" + .health_checks[0].http_health_check.path")"
  # A cluster that has carried no traffic is checked every no_traffic_interval, 60 s by default.
  check "gateway: handbook $id cluster is checked every 5 s, also before its first request" \
    "5s 5s 2s 2 1" \
    "$(cfg ".static_resources.clusters[] | select(.name == \"handbook-$id\") | .health_checks[0]
      | .interval + \" \" + .no_traffic_interval + \" \" + .timeout + \" \" + (.unhealthy_threshold | tostring)
        + \" \" + (.healthy_threshold | tostring)")"
  # With every host unhealthy Envoy otherwise routes to them anyway ("panic mode").
  check "gateway: handbook $id cluster never panics into unhealthy hosts" \
    "0" \
    "$(cfg ".static_resources.clusters[] | select(.name == \"handbook-$id\") | .common_lb_config.healthy_panic_threshold.value | tostring")"
done

# Authentication: a JWT from Keycloak, read from a header on /_naira/session and from a cookie elsewhere.
check "gateway: JWT issuer is keycloak.issuer" \
  "http://localhost:8080/realms/naira http://localhost:8080/realms/naira" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers | .keycloak_header.issuer + \" \" + .keycloak_cookie.issuer")"
check "gateway: the keys come from keycloak.baseUrl, cached" \
  "http://keycloak.naira-deps.svc.cluster.local:8080/realms/naira/protocol/openid-connect/certs keycloak-jwks 600s" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers.keycloak_cookie.remote_jwks
    | .http_uri.uri + \" \" + .http_uri.cluster + \" \" + .cache_duration")"
check "gateway: both providers read the same keys" "true" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers
    | (.keycloak_header.remote_jwks.http_uri.uri == .keycloak_cookie.remote_jwks.http_uri.uri) | tostring")"
check "gateway: the session provider reads only x-naira-token" "x-naira-token false false" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers.keycloak_header
    | (.from_headers | map(.name) | join(\",\")) + \" \" + (has(\"from_cookies\") | tostring) + \" \" + (has(\"from_params\") | tostring)")"
check "gateway: the cookie provider reads only the naira_handbook cookie, not Authorization or a query parameter" \
  "naira_handbook false false" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers.keycloak_cookie
    | (.from_cookies | join(\",\")) + \" \" + (has(\"from_headers\") | tostring) + \" \" + (has(\"from_params\") | tostring)")"
# Without forward the header is gone before the Set-Cookie value is built from it.
check "gateway: the session token header is kept for the Set-Cookie value" "true" \
  "$(cfg "${HCM}.http_filters[0].typed_config.providers.keycloak_header.forward | tostring")"
check "gateway: requirements map to the two providers" "keycloak_header keycloak_cookie" \
  "$(cfg "${HCM}.http_filters[0].typed_config.requirement_map | .header.provider_name + \" \" + .cookie.provider_name")"
check "gateway: JWT filter runs before the router" "envoy.filters.http.jwt_authn envoy.filters.http.router" \
  "$(cfg "${HCM}.http_filters | map(.name) | join(\" \")")"
check "gateway: JWT issuer and keys follow keycloak values" \
  "https://auth.example.com/realms/prod https://kc.example.com/auth/realms/prod/protocol/openid-connect/certs" \
  "$(gw_render --set keycloak.issuer=https://auth.example.com/realms/prod --set keycloak.realm=prod \
      --set keycloak.baseUrl=https://kc.example.com/auth/ \
    | gwq "${HCM}.http_filters[0].typed_config.providers.keycloak_cookie | .issuer + \" \" + .remote_jwks.http_uri.uri")"

# Routes: exactly three paths skip the JWT check, and everything else needs the cookie.
for id in platform other; do
  check "gateway: handbook $id routes and their JWT requirement" \
    "/_naira/session:header /_naira/session:disabled /_naira/bootstrap.html:disabled /_naira/assets/:disabled /:cookie" \
    "$(jwt_summary "handbook-$id")"
done
check "gateway: /_naira/session sets the cookie only on POST" ":method POST 204" \
  "$(cfg "$(vhost handbook-platform) | .routes[0] | .match.headers[0].name + \" \" + .match.headers[0].string_match.exact
    + \" \" + (.direct_response.status | tostring)")"
check "gateway: other methods on /_naira/session get 405 and an Allow header" "405 allow:POST" \
  "$(cfg "$(vhost handbook-platform) | .routes[1] | (.direct_response.status | tostring) + \" \" + .response_headers_to_add[0].header.key
    + \":\" + .response_headers_to_add[0].header.value")"
check "gateway: the cookie is HttpOnly, SameSite=Lax, host-only, and not Secure over http" \
  "naira_handbook=%REQ(x-naira-token)%; HttpOnly; SameSite=Lax; Path=/" \
  "$(cfg "$(vhost handbook-platform) | .routes[0].response_headers_to_add[0].header | select(.key == \"set-cookie\") | .value")"
check "gateway: the cookie is Secure over https" \
  "naira_handbook=%REQ(x-naira-token)%; HttpOnly; Secure; SameSite=Lax; Path=/" \
  "$(gw_render --set handbookGateway.scheme=https --set handbookGateway.port=443 \
    | gwq "$(vhost handbook-platform) | .routes[0].response_headers_to_add[0].header.value")"
# A Domain attribute would send handbook A's cookie to handbook B.
check "gateway: the cookie has no Domain and no expiry (a session cookie)" "false" \
  "$(cfg "$(vhost handbook-platform) | .routes[0].response_headers_to_add[0].header.value | test(\"Domain|Max-Age|Expires\") | tostring")"
check "gateway: three routes per handbook reach the handbook and each drops the cookie header" "3 1" \
  "$(cfg "$(vhost handbook-platform) | ([.routes[] | select(.route)] | length | tostring) + \" \"
    + ([.routes[] | select(.route) | ((.request_headers_to_remove // []) | map(select(. == \"cookie\")) | length)] | unique | join(\",\"))")"
check "gateway: the session route reaches no upstream" "0" \
  "$(cfg "$(vhost handbook-platform) | [.routes[0:2][] | select(.route)] | length | tostring")"

# Response headers.
check "gateway: handbooks can only be framed by the portal" \
  "content-security-policy frame-ancestors http://localhost:3000" \
  "$(cfg "$(vhost handbook-platform) | .response_headers_to_add[0].header | .key + \" \" + .value")"
check "gateway: the frame ancestor follows handbookGateway.portalOrigin" \
  "frame-ancestors https://naira.example.com" \
  "$(gw_render --set handbookGateway.portalOrigin=https://naira.example.com \
    | gwq "$(vhost handbook-other) | .response_headers_to_add[0].header.value")"
check "gateway: the CSP overwrites any header from the handbook" "OVERWRITE_IF_EXISTS_OR_ADD" \
  "$(cfg "$(vhost handbook-platform) | .response_headers_to_add[0].append_action")"
check "gateway: the Server header is dropped, not replaced" "PASS_THROUGH server true" \
  "$(cfg "${HCM}.server_header_transformation + \" \" + (${HCM}.route_config.response_headers_to_remove | join(\",\"))
    + \" \" + (${HCM}.http_filters[1].typed_config.suppress_envoy_headers | tostring)")"
# Authorization rules run on the normalized path, so /_naira/assets/../x cannot ride on the open prefix.
check "gateway: paths are normalized before routing" "true true UNESCAPE_AND_REDIRECT" \
  "$(cfg "(${HCM}.normalize_path | tostring) + \" \" + (${HCM}.merge_slashes | tostring) + \" \" + ${HCM}.path_with_escaped_slashes_action")"

# Local replies: the files are used as they are, and Envoy reads every % as the start of an operator.
UNAUTHORIZED="$CHART/files/handbook-gateway/unauthorized.html"
UNAVAILABLE="$CHART/files/handbook-gateway/unavailable.html"
LOCAL_REPLY="${HCM}.local_reply_config.mappers"
check "gateway: a missing or invalid token gets the sign-in redirect page" \
  "401 $(cat "$UNAUTHORIZED")" \
  "$(cfg "(${LOCAL_REPLY}[0].filter.status_code_filter.comparison.value.default_value | tostring) + \" \"
    + ${LOCAL_REPLY}[0].body_format_override.text_format_source.inline_string")"
check "gateway: a handbook that cannot be reached gets the degraded page with status 503" \
  "UH,UF,UC 503 $(cat "$UNAVAILABLE")" \
  "$(cfg "(${LOCAL_REPLY}[1].filter.response_flag_filter.flags | join(\",\")) + \" \" + (${LOCAL_REPLY}[1].status_code | tostring) + \" \"
    + ${LOCAL_REPLY}[1].body_format_override.text_format_source.inline_string")"
check "gateway: local reply pages are HTML" "text/html; charset=utf-8,text/html; charset=utf-8" \
  "$(cfg "[${LOCAL_REPLY}[].body_format_override.content_type] | join(\",\")")"
for f in unauthorized unavailable; do
  check "gateway: $f.html is under 4 KB" "ok" \
    "$([ "$(wc -c <"$CHART/files/handbook-gateway/$f.html")" -lt 4096 ] && echo ok)"
done
check "gateway: unauthorized.html has no % for Envoy to read" "0" \
  "$(grep -c '%' "$UNAUTHORIZED" || true)"
check "gateway: unavailable.html uses only the two route metadata operators" "0" \
  "$(sed -E 's/%METADATA\(ROUTE:naira\.handbook:(label|contact)\)%//g' "$UNAVAILABLE" | grep -c '%' || true)"
check "gateway: unavailable.html shows the handbook name and its contact" "2" \
  "$(grep -c -E '%METADATA\(ROUTE:naira\.handbook:(label|contact)\)%' "$UNAVAILABLE" || true)"

# The degraded page reads the handbook's name and contact from route metadata, escaped for HTML.
ROUTE_META='filter_metadata["naira.handbook"]'
check "gateway: the three handbook routes carry the same label and contact" "1" \
  "$(cfg "$(vhost handbook-platform) | [.routes[] | select(.route) | .metadata.${ROUTE_META} | .label + \"|\" + .contact] | unique | length | tostring")"
check "gateway: the label defaults to the id" "platform" \
  "$(cfg "$(vhost handbook-platform) | .routes[-1].metadata.${ROUTE_META}.label")"
check "gateway: the label is the handbook's label, escaped for HTML" \
  '&lt;b&gt;Hand &amp; book&lt;/b&gt; &#34;one&#34;' \
  "$(render_handbooks "$(handbook platform '"label":"<b>Hand & book</b> \"one\""')" \
    | gwq "$(vhost handbook-platform) | .routes[-1].metadata.${ROUTE_META}.label")"
contact() {
  render_handbooks "$(handbook platform "$1")" | gwq "$(vhost handbook-platform) | .routes[-1].metadata.${ROUTE_META}.contact"
}
check "gateway: contact names the owner and links the source" \
  'Owner: team-platform. <a href="https://github.com/acme/hb?a=1&amp;b=2">Source repository</a>.' \
  "$(contact '"owner":"team-platform","source":"https://github.com/acme/hb?a=1&b=2"')"
check "gateway: contact with an owner only" "Owner: team-platform." \
  "$(contact '"owner":"team-platform"')"
check "gateway: contact with a source only" '<a href="https://example.com/hb">Source repository</a>.' \
  "$(contact '"source":"https://example.com/hb"')"
check "gateway: contact falls back to a sentence when nothing is set" "Contact the owner of this handbook." \
  "$(contact '"x":1')"
check "gateway: the owner is escaped" "Owner: &lt;i&gt;x&lt;/i&gt;." \
  "$(contact '"owner":"<i>x</i>"')"
check "gateway: a source that is not http(s) is not linked" "Contact the owner of this handbook." \
  "$(contact '"source":"javascript:alert(1)"')"
check "gateway: a quote in the source cannot close the href attribute" \
  '<a href="https://x.example/&#34;onmouseover=&#34;alert(1)">Source repository</a>.' \
  "$(contact '"source":"https://x.example/\"onmouseover=\"alert(1)"')"
check "gateway: a source with a space is not a URL and is not linked" "Contact the owner of this handbook." \
  "$(contact '"source":"https://x.example/ onmouseover=alert(1)"')"
check "gateway: a label with % renders into valid config" "100% sure" \
  "$(render_handbooks "$(handbook platform '"label":"100% sure"')" \
    | gwq "$(vhost handbook-platform) | .routes[-1].metadata.${ROUTE_META}.label")"

# Admin and listener.
check "gateway: the admin interface listens on loopback only" "127.0.0.1" \
  "$(cfg '.admin.address.socket_address.address')"
check "gateway: one listener, on 8080" "1 8080" \
  "$(cfg '([.static_resources.listeners[]] | length | tostring) + " " + (.static_resources.listeners[0].address.socket_address.port_value | tostring)')"

# Keycloak endpoint: the JWKS cluster follows keycloak.baseUrl, with TLS for https.
JWKS_CLUSTER='.static_resources.clusters[] | select(.name == "keycloak-jwks")'
jwks_cluster() { gw_render --set keycloak.baseUrl="$1" | gwq "$JWKS_CLUSTER | $2"; }
SOCKET='.load_assignment.endpoints[0].lb_endpoints[0].endpoint.address.socket_address'
check "gateway: JWKS cluster, http: address and port from keycloak.baseUrl, no TLS" \
  "keycloak.naira-deps.svc.cluster.local 8080 false" \
  "$(cfg "$JWKS_CLUSTER | ${SOCKET}.address + \" \" + (${SOCKET}.port_value | tostring) + \" \" + (has(\"transport_socket\") | tostring)")"
check "gateway: JWKS cluster, https: port 443 by default, TLS with the host as SNI and the system CAs" \
  "keycloak.example.com 443 keycloak.example.com /etc/ssl/certs/ca-certificates.crt" \
  "$(jwks_cluster https://keycloak.example.com "${SOCKET}.address + \" \" + (${SOCKET}.port_value | tostring)
    + \" \" + .transport_socket.typed_config.sni
    + \" \" + .transport_socket.typed_config.common_tls_context.validation_context.trusted_ca.filename")"
check "gateway: JWKS cluster checks the certificate against the host name" "DNS keycloak.example.com" \
  "$(jwks_cluster https://keycloak.example.com \
    ".transport_socket.typed_config.common_tls_context.validation_context.match_typed_subject_alt_names[0]
      | .san_type + \" \" + .matcher.exact")"
check "gateway: JWKS cluster, an explicit port wins and a path is ignored" "8443" \
  "$(jwks_cluster https://keycloak.example.com:8443/auth "${SOCKET}.port_value")"
check "gateway: JWKS cluster, http without a port means 80" "80" \
  "$(jwks_cluster http://keycloak.example.com "${SOCKET}.port_value")"
check "gateway: JWKS cluster, an IPv6 host loses its brackets" "fd00::1 8080" \
  "$(jwks_cluster 'http://[fd00::1]:8080' "${SOCKET}.address + \" \" + (${SOCKET}.port_value | tostring)")"

# Deployment and Service.
check "gateway: image is pinned by digest and ignores image.registry and image.tag" \
  "envoyproxy/envoy@sha256:09ca6f2140a61ad8df11779a4a30d52b48433d444466328494f9188c9c35eded" \
  "$(gw_render --set image.registry=other.example --set image.tag=zz | yq "$GW_POD | .containers[0].image")"
check "gateway: image repository and digest come from handbookGateway.image" \
  "reg.example.com/envoy@sha256:$DIGEST" \
  "$(gw_render --set handbookGateway.image.repository=reg.example.com/envoy --set handbookGateway.image.digest="sha256:$DIGEST" \
    | yq "$GW_POD | .containers[0].image")"
check "gateway: restricted security context" "false true true ALL RuntimeDefault 65532" \
  "$(gw_render | yq "$GW_POD | .containers[0].securityContext
    | (.allowPrivilegeEscalation | tostring) + \" \" + (.readOnlyRootFilesystem | tostring) + \" \" + (.runAsNonRoot | tostring)
      + \" \" + .capabilities.drop[0] + \" \" + .seccompProfile.type + \" \" + (.runAsUser | tostring)")"
check "gateway: no API token and not the catalog ServiceAccount" "false null" \
  "$(gw_render | yq "$GW_POD | (.automountServiceAccountToken | tostring) + \" \" + (.serviceAccountName // \"null\")")"
check "gateway: runs envoy.yaml from the handbook-gateway ConfigMap, mounted read-only" \
  "/etc/envoy true handbook-gateway -c /etc/envoy/envoy.yaml --concurrency 2" \
  "$(gw_render | yq "$GW_POD | .containers[0].volumeMounts[0].mountPath + \" \" + (.containers[0].volumeMounts[0].readOnly | tostring)
    + \" \" + .volumes[0].configMap.name + \" \" + (.containers[0].args | join(\" \"))")"
check "gateway: the listener is the only port, the admin port is not exposed" "http 8080 1" \
  "$(gw_render | yq "$GW_POD | .containers[0].ports[0].name + \" \" + (.containers[0].ports[0].containerPort | tostring)
    + \" \" + (.containers[0].ports | length | tostring)")"
# The admin interface is on loopback, which kubelet probes (pod IP) cannot reach.
check "gateway: probes check the listener" "http http" \
  "$(gw_render | yq "$GW_POD | .containers[0] | .readinessProbe.tcpSocket.port + \" \" + .livenessProbe.tcpSocket.port")"
check "gateway: Service on 8080 selects the gateway pods" "8080 handbook-gateway t" \
  "$(gw_render | yq 'select(.kind == "Service" and .metadata.name == "handbook-gateway")
    | (.spec.ports[0].port | tostring) + " " + .spec.selector["app.kubernetes.io/name"] + " " + .spec.selector["app.kubernetes.io/instance"]')"
check "gateway: the Deployment's pods carry the labels the Service and the Deployment select" \
  "handbook-gateway t handbook-gateway t" \
  "$(gw_render | yq "$GW_DEPLOYMENT | .spec.template.metadata.labels[\"app.kubernetes.io/name\"] + \" \"
    + .spec.template.metadata.labels[\"app.kubernetes.io/instance\"] + \" \" + .spec.selector.matchLabels[\"app.kubernetes.io/name\"]
    + \" \" + .spec.selector.matchLabels[\"app.kubernetes.io/instance\"]")"
# handbook-ingress selects component=handbook pods; the gateway has to stay out of it.
check "gateway: pods do not carry the handbook component label" "false" \
  "$(gw_render | yq "$GW_DEPLOYMENT | .spec.template.metadata.labels | has(\"app.kubernetes.io/component\") | tostring")"
check "gateway: the NetworkPolicies select the pods the Deployment creates" "handbook-gateway handbook-gateway" \
  "$(o=$(gw_render --set handbookGateway.networkPolicy.enabled=true)
     echo "$(echo "$o" | yq 'select(.metadata.name == "handbook-gateway-egress") | .spec.podSelector.matchLabels["app.kubernetes.io/name"]') $(echo "$o" | yq "$GW_DEPLOYMENT | .spec.template.metadata.labels[\"app.kubernetes.io/name\"]")")"
check "gateway: one replica by default" "1" "$(gw_render | yq "$GW_DEPLOYMENT | .spec.replicas")"
check "gateway: replicas from handbookGateway.replicas" "2" \
  "$(gw_render --set handbookGateway.replicas=2 | yq "$GW_DEPLOYMENT | .spec.replicas")"
check "gateway: small resources" "500m 128Mi" \
  "$(gw_render | yq "$GW_POD | .containers[0].resources.limits | .cpu + \" \" + .memory")"
check "gateway: imagePullSecrets apply" "regcred" \
  "$(gw_render --set 'imagePullSecrets[0].name=regcred' | yq "$GW_POD | .imagePullSecrets[0].name")"
check "gateway: no scheduling fields by default" "false" \
  "$(gw_render | yq "$GW_POD | (has(\"imagePullSecrets\") or has(\"nodeSelector\") or has(\"tolerations\")
    or has(\"affinity\") or has(\"priorityClassName\") or has(\"securityContext\")) | tostring")"
check "gateway: scheduling fields from handbookGateway" "linux dedicated high 1000" \
  "$(gw_render --set handbookGateway.nodeSelector.os=linux --set 'handbookGateway.tolerations[0].key=dedicated' \
      --set handbookGateway.priorityClassName=high --set handbookGateway.podSecurityContext.fsGroup=1000 \
    | yq "$GW_POD | .nodeSelector.os + \" \" + .tolerations[0].key + \" \" + .priorityClassName
      + \" \" + (.securityContext.fsGroup | tostring)")"
check "gateway: podAnnotations are kept next to checksum/config" "v1 ok" \
  "$(gw_render --set-string handbookGateway.podAnnotations.team=v1 \
    | yq "$GW_DEPLOYMENT | .spec.template.metadata.annotations
      | .team + \" \" + (select(has(\"checksum/config\")) | \"ok\")")"

# Rollout: the pod restarts when, and only when, the Envoy config changes.
gw_sum() { yq "$GW_DEPLOYMENT | .spec.template.metadata.annotations[\"checksum/config\"]"; }
SUM=$(gw_render | gw_sum)
check "gateway: checksum/config is a sha256" "ok" "$([[ $SUM =~ ^[0-9a-f]{64}$ ]] && echo ok)"
check "gateway: same values, same checksum" "$SUM" "$(gw_render | gw_sum)"
check "gateway: handbooks listed in another order keep the checksum" "$SUM" \
  "$(render --set-json "handbooks=[$(handbook other),$(handbook platform)]" | gw_sum)"
check "gateway: a handbook's order and primary do not restart the gateway" "$SUM" \
  "$(render --set-json "handbooks=[$(handbook platform '"order":4,"primary":true'),$(handbook other '"order":-1')]" | gw_sum)"
check "gateway: a new handbook image digest does not restart the gateway" "$SUM" \
  "$(render --set-json "handbooks=[$(handbook platform "\"image\":\"ghcr.io/naira-project/platform@sha256:${DIGEST//a/b}\""),$(handbook other)]" | gw_sum)"
check "gateway: unrelated values do not restart the gateway" "$SUM" \
  "$(gw_render --set catalog.replicas=3 --set handbookDefaults.replicas=2 | gw_sum)"
for change in 'handbookGateway.domain=handbook.example.com' 'handbookGateway.port=8443' 'handbookGateway.scheme=https' \
              'handbookGateway.portalOrigin=https://naira.example.com' 'keycloak.issuer=https://x.example/realms/naira' \
              'keycloak.realm=other' 'keycloak.baseUrl=http://other.example:8080'; do
  check "gateway: ${change%%=*} restarts the gateway" "changed" \
    "$([ "$(gw_render --set "$change" | gw_sum)" != "$SUM" ] && echo changed)"
done
check "gateway: a new handbook restarts the gateway" "changed" \
  "$([ "$(render --set-json "handbooks=[$(handbook platform),$(handbook other),$(handbook third)]" | gw_sum)" != "$SUM" ] && echo changed)"
check "gateway: a new handbook label restarts the gateway" "changed" \
  "$([ "$(render --set-json "handbooks=[$(handbook platform '"label":"New"'),$(handbook other)]" | gw_sum)" != "$SUM" ] && echo changed)"
check "gateway: the ConfigMap itself is not annotated" "null" \
  "$(gw_render | yq 'select(.kind == "ConfigMap" and .metadata.name == "handbook-gateway") | .metadata.annotations // "null"')"

# Guards. They apply once a handbook is enabled, and every message names the value to fix.
must_fail "gateway image digest missing" "handbookGateway.image.digest: must be sha256:<64 lowercase hex digits>" \
  --set handbookGateway.image.digest= --set-json "handbooks=[$(handbook platform)]"
must_fail "gateway image digest is a tag" "handbookGateway.image.digest: must be sha256:" \
  --set handbookGateway.image.digest=distroless-v1.40.2 --set-json "handbooks=[$(handbook platform)]"
must_fail "gateway image digest in uppercase" "handbookGateway.image.digest: must be sha256:" \
  --set handbookGateway.image.digest="sha256:${DIGEST//a/A}" --set-json "handbooks=[$(handbook platform)]"
must_fail "gateway image repository missing" "handbookGateway.image.repository is required" \
  --set handbookGateway.image.repository= --set-json "handbooks=[$(handbook platform)]"
for origin in "" naira.example.com ftp://naira.example.com http://naira.example.com/portal \
              "http://naira.example.com;script-src *" "http://naira.example.com *" "http://naira .example.com" \
              "http://naira.example.com?x" "http://user@naira.example.com" "http://naira.example.com:99999x"; do
  must_fail "gateway portalOrigin \"$origin\"" 'handbookGateway.portalOrigin: must be an origin, scheme://host[:port]' \
    --set-string handbookGateway.portalOrigin="$origin" --set-json "handbooks=[$(handbook platform)]"
done
check "gateway: portalOrigin with an https host and port is accepted" "ok" \
  "$(gw_render --set handbookGateway.portalOrigin=https://naira.example.com:8443 >/dev/null && echo ok)"
for base in "" keycloak:8080 ftp://keycloak.example.com http:// "http://keycloak example.com"; do
  must_fail "gateway keycloak.baseUrl \"$base\"" "keycloak.baseUrl: must be an http or https URL with a host" \
    --set-string keycloak.baseUrl="$base" --set-json "handbooks=[$(handbook platform)]"
done
must_fail "gateway keycloak.issuer missing" "keycloak.issuer is required" \
  --set keycloak.issuer= --set-json "handbooks=[$(handbook platform)]"
must_fail "gateway keycloak.realm missing" "keycloak.realm is required" \
  --set keycloak.realm= --set-json "handbooks=[$(handbook platform)]"
check "gateway: its values are not checked while no handbook is enabled" "ok" \
  "$(render --set handbookGateway.image.digest= --set handbookGateway.portalOrigin=nope --set keycloak.baseUrl= \
      --set keycloak.issuer= >/dev/null && echo ok)"

# ── Handbooks in the portal ──────────────────────────────────────────────────────
# The portal reads handbooks.json from the mounted ConfigMap handbooks and restarts when its content changes.
PORTAL='select(.kind == "Deployment" and .metadata.name == "portal") | .spec.template'
HANDBOOKS_JSON_TEXT='select(.kind == "ConfigMap" and .metadata.name == "handbooks") | .data["handbooks.json"]'
portal_sum() { yq "$PORTAL | .metadata.annotations[\"checksum/handbooks\"]"; }

check "portal: mounts the handbooks ConfigMap read-only" "handbooks handbooks /etc/naira/handbooks true" \
  "$(render | yq "$PORTAL | .spec.volumes[0].name + \" \" + .spec.volumes[0].configMap.name + \" \"
      + .spec.containers[0].volumeMounts[0].mountPath + \" \" + (.spec.containers[0].volumeMounts[0].readOnly | tostring)")"
check "portal: the mount is named like the volume" "handbooks" \
  "$(render | yq "$PORTAL | .spec.containers[0].volumeMounts[0].name")"
check "portal: NAIRA_HANDBOOKS_FILE is handbooks.json in that mount" "/etc/naira/handbooks/handbooks.json" \
  "$(render | yq "$PORTAL | .spec.containers[0].env[] | select(.name == \"NAIRA_HANDBOOKS_FILE\") | .value")"
check "portal: the file name is the key of the handbooks ConfigMap" "handbooks.json" \
  "$(o=$(render)
     f=$(echo "$o" | yq "$PORTAL | .spec.containers[0].env[] | select(.name == \"NAIRA_HANDBOOKS_FILE\") | .value | sub(\".*/\"; \"\")")
     echo "$o" | yq "select(.kind == \"ConfigMap\" and .metadata.name == \"handbooks\") | .data | keys | .[] | select(. == \"$f\")")"
check "portal: the other env vars and the other volumes are unchanged" "PORT NAIRA_UI_BASE_URL IDP_NAMES BASE_DOMAINS_KEYCLOAK AUTH_SERVER_URL_KEYCLOAK TOKEN_URL_KEYCLOAK OIDC_CLIENT_ID_KEYCLOAK OIDC_CLIENT_SECRET_KEYCLOAK NAIRA_HANDBOOKS_FILE 1" \
  "$(render | yq "$PORTAL | ([.spec.containers[0].env[].name] | join(\" \")) + \" \" + (.spec.volumes | length | tostring)")"
check "portal: also mounts the (empty) list when no handbook is enabled" "[]" \
  "$(render | yq "$HANDBOOKS_JSON_TEXT | from_json" -o=json -I=0)"
check "portal: a podAnnotation stays next to checksum/handbooks" "x ok" \
  "$(render --set-string portal.podAnnotations.team=x \
    | yq "$PORTAL | .metadata.annotations | .team + \" \" + (select(has(\"checksum/handbooks\")) | \"ok\")")"

PSUM=$(render | portal_sum)
check "portal: checksum/handbooks is a sha256" "ok" "$([[ $PSUM =~ ^[0-9a-f]{64}$ ]] && echo ok)"
check "portal: checksum/handbooks is the sha256 of handbooks.json" \
  "$(o=$(render_handbooks "$(handbook platform '"label":"Handbook"')"); printf '%s' "$(echo "$o" | yq "$HANDBOOKS_JSON_TEXT")" | sha256sum | cut -d' ' -f1)" \
  "$(render_handbooks "$(handbook platform '"label":"Handbook"')" | portal_sum)"
check "portal: same values, same checksum" "$PSUM" "$(render | portal_sum)"
PSUM2=$(render_handbooks "$(handbook platform)" "$(handbook other)" | portal_sum)
check "portal: handbooks listed in another order keep the checksum (the file is sorted)" "$PSUM2" \
  "$(render_handbooks "$(handbook other)" "$(handbook platform)" | portal_sum)"
for change in '"label":"New"' '"order":4' '"primary":true' '"owner":"team"' '"source":"https://example.com/hb"'; do
  field=${change%%:*}
  check "portal: a handbook's ${field//\"/} restarts the portal" "changed" \
    "$([ "$(render_handbooks "$(handbook platform "$change")" "$(handbook other)" | portal_sum)" != "$PSUM2" ] && echo changed)"
done
check "portal: a new handbook restarts the portal" "changed" \
  "$([ "$PSUM2" != "$PSUM" ] && echo changed)"
check "portal: removing a handbook restarts the portal" "changed" \
  "$([ "$(render_handbooks "$(handbook platform)" | portal_sum)" != "$PSUM2" ] && echo changed)"
check "portal: another handbook domain restarts the portal (the urls change)" "changed" \
  "$([ "$(render --set handbookGateway.domain=handbook.example.com --set-json "handbooks=[$(handbook platform),$(handbook other)]" | portal_sum)" != "$PSUM2" ] && echo changed)"
check "portal: a new handbook image digest does not restart the portal" "$PSUM2" \
  "$(render --set-json "handbooks=[$(handbook platform "\"image\":\"ghcr.io/naira-project/platform@sha256:${DIGEST//a/b}\""),$(handbook other)]" | portal_sum)"
check "portal: a disabled handbook does not restart the portal" "$PSUM2" \
  "$(render_handbooks "$(handbook platform)" "$(handbook other)" "$(handbook off '"enabled":false')" | portal_sum)"
check "portal: unrelated values do not restart the portal" "$PSUM2" \
  "$(render --set catalog.replicas=3 --set handbookGateway.replicas=2 --set portal.replicas=2 \
      --set-json "handbooks=[$(handbook platform),$(handbook other)]" | portal_sum)"

# ── Handbooks in the dev values ──────────────────────────────────────────────────
# values-dev.yaml lists two handbooks for Tilt, which builds handbook/<id> and swaps the image.
TILTFILE="$CHART/../../../Tiltfile"
DEV_HANDBOOK_DEPLOYMENT='select(.kind == "Deployment" and .metadata.labels["app.kubernetes.io/component"] == "handbook")'
check "dev handbooks: the how-to first as primary, then the example" "howto:true,example:false" \
  "$(render_raw "${DEV[@]}" | yq "$HANDBOOKS_JSON | map(.id + \":\" + (.primary | tostring)) | join(\",\")")"
check "dev handbooks: addresses are on port 3003" \
  "http://howto.naira.localhost:3003/,http://example.naira.localhost:3003/" \
  "$(render_raw "${DEV[@]}" | yq "$HANDBOOKS_JSON | map(.url) | join(\",\")")"
# Chrome drops the gateway's SameSite=Lax cookie when the portal is on another site than the handbooks.
check "dev handbooks: the portal host is the handbook domain, so both are on one site" \
  "$(yq '.handbookGateway.domain' "$CHART/values-dev.yaml")" \
  "$(yq '.handbookGateway.portalOrigin' "$CHART/values-dev.yaml" | sed -E 's#^[a-z]+://([^:/]+).*#\1#')"
# Grafana is forwarded to 3002 and the UI's log links point there.
check "dev handbooks: the gateway port is not the one of the Grafana log links" "differ" \
  "$([ "$(yq '.handbookGateway.port' "$CHART/values-dev.yaml")" != "$(yq '.ui.config.logs.explorerUrl' "$CHART/values-dev.yaml" | sed 's/.*://')" ] && echo differ)"
check "dev handbooks: the gateway port in handbooks.json is not the Grafana port" "true" \
  "$(render_raw "${DEV[@]}" | yq "$HANDBOOKS_JSON | map(.url | test(\":3002/\") | not) | all | tostring")"
# Tilt matches a docker_build to a Deployment by the image name, and builds it from a directory.
DEV_HANDBOOK_IMAGES=$(render_raw "${DEV[@]}" \
  | yq "$DEV_HANDBOOK_DEPLOYMENT | .metadata.name + \" \" + (.spec.template.spec.containers[0].image | sub(\"@.*\"; \"\"))" \
  | sed -e '/^---$/d' -e 's/^handbook-//')
check "dev handbooks: each image is named after its handbook" "howto local/naira-handbook-howto example local/naira-handbook-example" \
  "$(echo "$DEV_HANDBOOK_IMAGES" | sort -r | tr '\n' ' ' | sed 's/ $//')"
while read -r id image; do
  check "dev handbooks: the Tiltfile builds $image from handbook/$id" "1" \
    "$(grep -c "'$image': 'handbook/$id'" "$TILTFILE")"
  check "dev handbooks: handbook/$id exists and is a handbook project" "ok" \
    "$([ -f "$CHART/../../../handbook/$id/Dockerfile" ] && [ -f "$CHART/../../../handbook/$id/package.json" ] && echo ok)"
done <<<"$DEV_HANDBOOK_IMAGES"
check "dev handbooks: the gateway's Envoy is pinned by digest, which the Tiltfile pulls instead of building" "true" \
  "$(render_raw "${DEV[@]}" | yq 'select(.kind == "Deployment" and .metadata.name == "handbook-gateway") | .spec.template.spec.containers[0].image | test("@sha256:[0-9a-f]{64}$") | tostring')"
check "dev handbooks: render stays valid with the placeholder digest" "ok" \
  "$(render_raw "${DEV[@]}" >/dev/null && echo ok)"

ALL_FIELDS_GW=$(render_raw -f "$CHART/ci/all-fields-values.yaml")
check "gateway: all-fields values set the image and replicas" \
  "reg.example.com/mirror/envoy@sha256:$(printf 'd%.0s' {1..64}) 2" \
  "$(echo "$ALL_FIELDS_GW" | yq "$GW_DEPLOYMENT | .spec.template.spec.containers[0].image + \" \" + (.spec.replicas | tostring)")"
check "gateway: all-fields values set the scheduling fields" "system-cluster-critical linux dedicated" \
  "$(echo "$ALL_FIELDS_GW" | yq "$GW_POD | .priorityClassName + \" \" + .nodeSelector[\"kubernetes.io/os\"] + \" \" + .tolerations[0].key")"
check "gateway: all-fields values set the frame ancestor and make the cookie Secure" \
  "frame-ancestors https://naira.example.com true" \
  "$(echo "$ALL_FIELDS_GW" | gwq "$(vhost handbook-platform-handbook) | .response_headers_to_add[0].header.value + \" \"
    + (.routes[0].response_headers_to_add[0].header.value | test(\"; Secure;\") | tostring)")"
check "all-fields values render" \
  "ok" \
  "$(render_raw -f "$CHART/ci/all-fields-values.yaml" >/dev/null && echo ok)"

exit $fail
