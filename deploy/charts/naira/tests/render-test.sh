#!/usr/bin/env bash
# Tests for the naira Helm chart. Each check runs `helm template` with some
# values and asserts on the rendered manifests with yq, so a change to
# values.yaml or a template that breaks the chart's contract fails here
# instead of on someone's cluster. In order, the sections cover:
#   - default policy: what a plain install runs (the only place that pins it)
#   - catalog Deployment, Service and image naming
#   - plugin sidecars, plugins.yaml and per-plugin RBAC
#   - plugin config files, Secrets, ui and portal
#   - guards that must refuse to render (bad values, missing Secrets)
#   - values-dev.yaml and the kind/CI setup
#   - pod hardening, scheduling fields and the catalog NetworkPolicy
#   - PostgreSQL and db-init
#
# Which plugins are enabled by default is a policy that changes. Only the
# "Default policy" section may depend on it. Every other check picks its
# plugins with `flags` (all plugins off, the named ones on), so it keeps
# passing when a plugin is switched on or off in values.yaml.
#
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

# only PLUGIN...: prints, one argument per line, helm flags that disable every
# plugin defined in values.yaml and then enable the named ones (a later --set
# wins). Use it through `flags`, so a check does not depend on which plugins
# are enabled by default. A plugin that needs more to render (tech-radar needs
# its radar file) still needs those flags on top.
ALL_PLUGINS=$(yq '.catalog.plugins | keys | .[]' "$CHART/values.yaml")
only() {
  local p
  for p in $ALL_PLUGINS; do printf '%s\n' --set "catalog.plugins.$p.enabled=false"; done
  for p in "$@"; do printf '%s\n' --set "catalog.plugins.$p.enabled=true"; done
}
# flags ARRAY PLUGIN...: fills ARRAY with the output of `only`. Pass it to
# helm as "${ARRAY[@]}".
flags() {
  local name=$1 line
  shift
  eval "$name=()"
  # shellcheck disable=SC2034 # line is read by the eval below
  while IFS= read -r line; do
    eval "$name+=(\"\$line\")"
  done < <(only "$@")
}
flags NO_PLUGINS
flags LITELLM_ONLY litellm

# yq selectors for the objects the checks look at.
CATALOG='select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.spec'
pod() { printf 'select(.kind == "Deployment" and .metadata.name == "%s") | .spec.template.spec' "$1"; }

# ── Default policy ───────────────────────────────────────────────────────────
# The one place that pins what a plain install runs. Changing which plugins
# are enabled by default in values.yaml means changing DEFAULT_SIDECARS here
# and nothing else.
DEFAULT_SIDECARS="plugin-depl-calls-svc,plugin-fluxcd,plugin-mcp-servers,plugin-mlflow"

check "default sidecars, sorted" \
  "$DEFAULT_SIDECARS" \
  "$(render | yq ea "[$CATALOG | .initContainers[].name] | join(\",\")")"
check "default: no plugin reads a Secret" \
  "0" \
  "$(render | yq ea '[select(.kind == "Deployment" and .metadata.name == "catalog")
                      | .spec.template.spec.initContainers[].env[]?
                      | select(has("valueFrom"))] | length')"
check "default: no Secret read grant (depl-uses-litellm is opt-in)" \
  "0" \
  "$(render | yq ea '[select(.kind == "ClusterRole") | .rules[] | select(.resources[] == "secrets")] | length')"
check "default: no NetworkPolicy" \
  "0" \
  "$(render | yq ea '[select(.kind == "NetworkPolicy")] | length')"
check "default: no Secrets" \
  "0" \
  "$(render | yq ea '[select(.kind == "Secret")] | length')"

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
check "numeric image tag renders as a string" \
  "ghcr.io/naira-project/naira-catalog:20260924" \
  "$(render --set image.tag=20260924 | yq "$CATALOG | .containers[0].image")"
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

flags LITELLM_MLFLOW litellm mlflow
flags OPENMETADATA_ONLY openmetadata

check "sidecars are native sidecars" \
  "Always" \
  "$(render "${LITELLM_ONLY[@]}" | yq ea "[$CATALOG | .initContainers[].restartPolicy] | unique | join(\",\")")"
check "sidecars are sorted by plugin name" \
  "plugin-litellm,plugin-mlflow" \
  "$(render "${LITELLM_MLFLOW[@]}" | yq ea "[$CATALOG | .initContainers[].name] | join(\",\")")"
check "plugin image" \
  "ghcr.io/naira-project/naira-plugin-litellm:${APP_VERSION}" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$LITELLM_SIDECAR | .image")"
check "plugin PORT" \
  "50051" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$LITELLM_ENV | select(.name == \"PORT\") | .value")"
check "plugin PATH_PREFIX" \
  "litellm" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$LITELLM_ENV | select(.name == \"PATH_PREFIX\") | .value")"
check "plugin env default" \
  "http://litellm.naira-deps.svc.cluster.local:4000" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "plugin env override" \
  "http://litellm.run1.svc.cluster.local:4000" \
  "$(render "${LITELLM_ONLY[@]}" \
      --set catalog.plugins.litellm.env.LITELLM_BASE_URL=http://litellm.run1.svc.cluster.local:4000 \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_BASE_URL\") | .value")"
check "plugin secret env defaults to catalog-secrets" \
  "catalog-secrets/LITELLM_API_KEY" \
  "$(render "${LITELLM_ONLY[@]}" \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\")
          | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "plugin resources default" \
  "128Mi" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$LITELLM_SIDECAR | .resources.limits.memory")"
check "plugin resources override merges" \
  "256Mi 200m 50m" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.plugins.litellm.resources.limits.memory=256Mi \
    | yq "$LITELLM_SIDECAR | .resources
          | .limits.memory + \" \" + .limits.cpu + \" \" + .requests.cpu")"
check "plugins.yaml entry" \
  "localhost:50051 0 0 * * *" \
  "$(render "${LITELLM_ONLY[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.litellm | .address + " " + .schedule')"
check "plugins.yaml omits schedule when unset" \
  "null" \
  "$(render "${OPENMETADATA_ONLY[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.openmetadata.schedule')"
check "disabled plugin: no sidecar" \
  "" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.plugins.litellm.enabled=false | yq "$LITELLM_SIDECAR | .name")"
check "disabled plugin: not in plugins.yaml" \
  "false" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.plugins.litellm.enabled=false \
    | yq "$PLUGINS_YAML" | yq '.plugins | has("litellm")')"
check "plugins.yaml mounted" \
  "/etc/catalog/plugins.yaml" \
  "$(render \
    | yq "$CATALOG | .containers[0].volumeMounts[] | select(.name == \"plugin-config\") | .mountPath")"

WITH_LITELLM=$(render "${LITELLM_ONLY[@]}" | yq "$CHECKSUM")
WITHOUT_LITELLM=$(render "${NO_PLUGINS[@]}" | yq "$CHECKSUM")
if [ "$WITH_LITELLM" != "$WITHOUT_LITELLM" ]; then checksum_changed=different; else checksum_changed=same; fi
check "checksum changes with plugins" "different" "$checksum_changed"

# ── RBAC ─────────────────────────────────────────────────────────────────────
flags RBAC_ONLY depl-calls-svc fluxcd
flags MLFLOW_ONLY mlflow

check "ClusterRoles are release-prefixed" \
  "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render "${RBAC_ONLY[@]}" | yq ea '[select(.kind == "ClusterRole") | .metadata.name] | sort | join(",")')"
check "bindings match roles" \
  "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render "${RBAC_ONLY[@]}" | yq ea '[select(.kind == "ClusterRoleBinding") | .roleRef.name] | sort | join(",")')"
check "binding subject is the catalog SA in the release namespace" \
  "catalog/naira" \
  "$(render "${RBAC_ONLY[@]}" \
    | yq 'select(.kind == "ClusterRoleBinding" and .metadata.name == "t-plugin-fluxcd")
          | .subjects[0] | .name + "/" + .namespace')"
check "plugin rules copied" \
  "namespaces,services" \
  "$(render "${RBAC_ONLY[@]}" \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-calls-svc")
          | .rules[0].resources | join(",")')"
check "disabled plugin: no RBAC" \
  "" \
  "$(render "${RBAC_ONLY[@]}" --set catalog.plugins.fluxcd.enabled=false \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-fluxcd") | .metadata.name')"
check "two releases do not collide" \
  "u-catalog" \
  "$(helm template u "$CHART" --namespace other "${BASE[@]}" \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "u-catalog") | .metadata.name')"
check "rbac without rules renders no ClusterRole" \
  "" \
  "$(render "${MLFLOW_ONLY[@]}" --set catalog.plugins.mlflow.rbac.foo=bar \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-mlflow") | .metadata.name')"
check "depl-uses-litellm enabled: ClusterRole with secrets" \
  "t-plugin-depl-uses-litellm" \
  "$(render --set catalog.plugins.depl-uses-litellm.enabled=true \
    | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-uses-litellm") | .metadata.name')"

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
# TECH_RADAR comes after the flags, so it re-enables tech-radar.
flags MLFLOW_DEPL mlflow depl-uses-litellm
check "mlflow, depl-uses-litellm and tech-radar: 3 sidecars" \
  "3" \
  "$(render "${MLFLOW_DEPL[@]}" "${TECH_RADAR[@]}" | yq "$CATALOG | .initContainers | length")"
check "tech-radar registered in plugins.yaml" \
  "localhost:50057" \
  "$(render "${TECH_RADAR[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.tech-radar.address')"

# ── Secrets ──────────────────────────────────────────────────────────────────
check "catalog Secret when create=true" \
  "k1" \
  "$(render_raw "${DB[@]}" --set portal.enabled=false \
      --set catalog.secret.create=true --set catalog.secret.data.LITELLM_API_KEY=k1 \
    | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "existingSecret reaches plugin env" \
  "ext" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.secret.existingSecret=ext \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\") | .valueFrom.secretKeyRef.name")"
check "portal Secret when create=true" \
  "s3" \
  "$(render_raw "${DB[@]}" --set catalog.enabled=false \
      --set portal.oidc.secret.create=true --set portal.oidc.secret.value=s3 \
    | yq 'select(.kind == "Secret" and .metadata.name == "portal-oidc") | .stringData["client-secret"]')"

# ── ui and portal ────────────────────────────────────────────────────────────
UI='select(.kind == "Deployment" and .metadata.name == "ui") | .spec.template.spec.containers[0]'
UI_DEPLOY='select(.kind == "Deployment" and .metadata.name == "ui")'
PORTAL='select(.kind == "Deployment" and .metadata.name == "portal") | .spec.template.spec.containers[0]'
ui_env() { yq "$UI | .env[] | select(.name == \"$1\") | .value"; }

check "ui catalog upstream follows the release namespace" \
  "http://catalog.naira.svc.cluster.local:8090" \
  "$(render | ui_env CATALOG_UPSTREAM)"
check "ui catalog upstream override" \
  "http://x:1" \
  "$(render --set ui.catalogUpstream=http://x:1 | ui_env CATALOG_UPSTREAM)"
check "ui prometheus upstream default" \
  "http://prometheus-operated.naira-deps.svc.cluster.local:9090" \
  "$(render | ui_env PROMETHEUS_UPSTREAM)"
check "ui prometheus upstream override" \
  "http://prom:9090" \
  "$(render --set ui.prometheus.upstream=http://prom:9090 | ui_env PROMETHEUS_UPSTREAM)"
check "ui prometheus upstream empty: no env var" \
  "" \
  "$(render --set ui.prometheus.upstream= | ui_env PROMETHEUS_UPSTREAM)"
check "ui prometheus allowed paths and methods default to the image's" \
  "" \
  "$(render | yq "$UI | .env[] | select(.name == \"PROMETHEUS_ALLOWED_PATHS\" or .name == \"PROMETHEUS_ALLOWED_METHODS\") | .name")"
check "ui prometheus allowed paths joined with |" \
  "query|series" \
  "$(render --set 'ui.prometheus.allowedPaths[0]=query' --set 'ui.prometheus.allowedPaths[1]=series' \
    | ui_env PROMETHEUS_ALLOWED_PATHS)"
check "ui prometheus allowed methods joined with a space" \
  "GET HEAD" \
  "$(render --set 'ui.prometheus.allowedMethods[0]=GET' --set 'ui.prometheus.allowedMethods[1]=HEAD' \
    | ui_env PROMETHEUS_ALLOWED_METHODS)"
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
check "ui disabled" \
  "" \
  "$(render --set ui.enabled=false | yq "$UI_DEPLOY | .metadata.name")"
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

# ── Guards ───────────────────────────────────────────────────────────────────
must_fail "duplicate plugin port" \
  "both use port 50051" \
  "${LITELLM_MLFLOW[@]}" --set catalog.plugins.mlflow.port=50051
must_fail "plugin port equal to the catalog port" \
  "catalog and mlflow both use port 8090" \
  "${MLFLOW_ONLY[@]}" --set catalog.plugins.mlflow.port=8090
check "disabled plugin may share a port" \
  "ok" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.plugins.mlflow.port=50051 >/dev/null && echo ok)"
must_fail "catalog secret: existing and create" \
  "catalog.secret: set existingSecret or create, not both" \
  --set catalog.secret.existingSecret=x --set catalog.secret.create=true
must_fail "portal secret: existing and create" \
  "portal.oidc.secret: set existingSecret or create, not both" \
  --set portal.oidc.secret.existingSecret=x --set portal.oidc.secret.create=true
must_fail "tech-radar on without a radar file" \
  "catalog.plugins.tech-radar.config: set existingConfigMap or data" \
  --set catalog.plugins.tech-radar.enabled=true
must_fail "plugin without an image" \
  "image.repository is required" \
  --set catalog.plugins.newone.enabled=true --set catalog.plugins.newone.port=50099
must_fail "secretKeyRef without key" \
  "secretKeyRef.key is required" \
  "${LITELLM_ONLY[@]}" --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.key=null
# 57 characters: "plugin-" plus the name goes over the 63 of a container name.
LONG=$(printf 'a%.0s' {1..57})
must_fail "plugin name over 56 characters" \
  "name too long" \
  --set "catalog.plugins.${LONG}.enabled=true" \
  --set "catalog.plugins.${LONG}.port=50099" \
  --set "catalog.plugins.${LONG}.image.repository=x"

# Missing Secret sources. These use render_raw: BASE would supply them.
RENDER=render_raw must_fail "no catalog Secret source" \
  "catalog.plugins.litellm reads LITELLM_API_KEY from the catalog Secret: set catalog.secret.existingSecret or catalog.secret.create" \
  "${LITELLM_ONLY[@]}" --set portal.enabled=false
RENDER=render_raw must_fail "OpenMetadata requires a catalog Secret source" \
  "catalog.plugins.openmetadata reads OPENMETADATA_ADMIN_PASSWORD from the catalog Secret: set catalog.secret.existingSecret or catalog.secret.create" \
  "${OPENMETADATA_ONLY[@]}" --set portal.enabled=false
RENDER=render_raw must_fail "no portal Secret source" \
  "portal reads its OIDC client secret: set portal.oidc.secret.existingSecret or portal.oidc.secret.create" \
  --set catalog.secret.existingSecret=x
RENDER=render_raw must_fail "create with empty data" \
  "catalog.secret.data is empty" \
  --set portal.enabled=false --set catalog.secret.create=true
RENDER=render_raw must_fail "portal create with empty value" \
  "portal.oidc.secret.value is empty" \
  --set catalog.enabled=false --set portal.oidc.secret.create=true
check "no Secret needed when nothing reads one" \
  "ok" \
  "$(render_raw "${DB[@]}" "${MLFLOW_ONLY[@]}" --set portal.enabled=false >/dev/null && echo ok)"
check "a plugin naming its own Secret needs no catalog Secret" \
  "ok" \
  "$(render_raw "${DB[@]}" "${LITELLM_ONLY[@]}" --set portal.enabled=false \
      --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.name=own >/dev/null && echo ok)"

# ── GitHub and Bedrock plugins (both read the catalog Secret) ────────────────
flags GITHUB github
GITHUB_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-github\")"

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
  "${GITHUB[@]}" --set portal.enabled=false

flags BEDROCK bedrock
BEDROCK_SIDECAR="$CATALOG | .initContainers[] | select(.name == \"plugin-bedrock\")"

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
  "${BEDROCK[@]}" --set portal.enabled=false

# ── Dev values (kind) ────────────────────────────────────────────────────────
DEV=(-f "$CHART/values-dev.yaml")

check "dev: local image names" \
  "naira-catalog:${APP_VERSION}" \
  "$(render_raw "${DEV[@]}" | yq "$CATALOG | .containers[0].image")"
check "dev: sidecars (LiteLLM and tech-radar on, OpenMetadata off)" \
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
check "all-fields values render" \
  "ok" \
  "$(render_raw -f "$CHART/ci/all-fields-values.yaml" >/dev/null && echo ok)"

# ── Edge cases ───────────────────────────────────────────────────────────────
check "numeric release name renders a string label" \
  "string" \
  "$(helm template 123 "$CHART" --namespace naira "${BASE[@]}" \
    | yq 'select(.kind == "Deployment" and .metadata.name == "catalog")
          | .metadata.labels["app.kubernetes.io/instance"] | tag' \
    | sed 's/!!//;s/str/string/')"

# ── Pod hardening and scheduling ─────────────────────────────────────────────
check "catalog runs as the distroless uid" \
  "65532 true" \
  "$(render \
    | yq "$(pod catalog) | .containers[0].securityContext
          | (.runAsUser | tostring) + \" \" + (.runAsNonRoot | tostring)")"
check "sidecar security context defaults" \
  "false ALL RuntimeDefault" \
  "$(render "${LITELLM_ONLY[@]}" \
    | yq "$LITELLM_SIDECAR | .securityContext
          | (.allowPrivilegeEscalation | tostring) + \" \" + .capabilities.drop[0] + \" \" + .seccompProfile.type")"
check "plugin securityContext merges over defaults" \
  "true true" \
  "$(render "${LITELLM_ONLY[@]}" --set catalog.plugins.litellm.securityContext.readOnlyRootFilesystem=true \
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

out=$(render)
check "postgres reads the admin Secret, renders none of its own" \
  "postgres-admin|0" \
  "$(yq 'select(.kind == "StatefulSet" and .metadata.name == "postgres")
         | .spec.template.spec.containers[0].env[] | select(.name == "POSTGRES_PASSWORD")
         | .valueFrom.secretKeyRef.name' <<<"$out")|$(yq ea '[select(.kind == "Secret")] | length' <<<"$out")"
check "db-init targets the subchart Service as the admin user" \
  "postgres.naira.svc.cluster.local postgres postgres-admin" \
  "$(yq "$DB_JOB | (.env[] | select(.name == \"PGHOST\") | .value) + \" \"
         + (.env[] | select(.name == \"PGUSER\") | .value) + \" \"
         + (.env[] | select(.name == \"PGPASSWORD\") | .valueFrom.secretKeyRef.name)" <<<"$out")"
check "db-init reads each database password from its Secret" \
  "catalog-db" \
  "$(yq "$DB_JOB | .env[] | select(.name == \"DB_PASSWORD_0\") | .valueFrom.secretKeyRef.name" <<<"$out")"
check "db-init gets sslmode, retries and a deadline" \
  "prefer 60 900" \
  "$(yq ea '[select(.kind == "Job")] | .[0]
         | [(.spec.template.spec.containers[0].env[]
             | select(.name == "PGSSLMODE" or .name == "DB_WAIT_RETRIES") | .value),
            (.spec.activeDeadlineSeconds | tostring)] | join(" ")' <<<"$out")"
check "dev: db-init ensures catalog, langfuse, litellm and openmetadata" \
  "catalog,langfuse,litellm,openmetadata" \
  "$(render_raw "${DEV[@]}" | yq "$DB_JOB | .args | join(\",\")")"
check "dev: litellm Secret has username and password" \
  "litellm litellm-local-password" \
  "$(render_raw "${DEV[@]}" \
    | yq 'select(.kind == "Secret" and .metadata.name == "litellm-db") | .stringData.username + " " + .stringData.password')"

out=$(render --set postgresql.enabled=false --set database.host=pg.example.com)
check "external server: no StatefulSet, db-init uses database.host" \
  "0 pg.example.com" \
  "$(yq ea '[select(.kind == "StatefulSet")] | length' <<<"$out") $(yq "$DB_JOB | .env[] | select(.name == \"PGHOST\") | .value" <<<"$out")"
check "no databases and no subchart: no db-init, no Secret needed" \
  "0" \
  "$(render_raw --set postgresql.enabled=false --set database.databases.catalog.enabled=false \
      -f "$CHART/ci/default-values.yaml" --set database.admin.secret.existingSecret= \
    | yq ea '[select(.kind == "Job" or (.kind == "ConfigMap" and .metadata.name == "db-init"))] | length')"

JOB_NAME=$(db_job_name)
JOB_NAME_AGAIN=$(db_job_name)
JOB_NAME_EXTRA=$(db_job_name --set database.databases.extra.enabled=true \
  --set database.databases.extra.secret.existingSecret=extra-db)
check "db-init Job name is stable across renders" "$JOB_NAME" "$JOB_NAME_AGAIN"
if [ "$JOB_NAME" = "$JOB_NAME_EXTRA" ]; then
  echo "FAIL db-init Job name changes with its databases"
  fail=1
else
  echo "ok   db-init Job name changes with its databases"
fi

must_fail "database needs a Secret source" \
  "database.databases.catalog: set secret.existingSecret or secret.create" \
  --set database.databases.catalog.secret.existingSecret=
must_fail "database without a secret block" \
  "database.databases.extra: set secret.existingSecret or secret.create" \
  --set database.databases.extra.enabled=true
must_fail "database Secret create needs a password" \
  "database.databases.catalog.secret.create is true but password is empty" \
  --set database.databases.catalog.secret.existingSecret= --set database.databases.catalog.secret.create=true
must_fail "admin Secret must match the subchart's" \
  "postgresql.auth.existingSecret must equal" \
  --set database.admin.secret.existingSecret=other
must_fail "external server needs a host" \
  "set database.host" \
  --set postgresql.enabled=false
must_fail "subchart and host are exclusive" \
  "database.host: leave empty" \
  --set database.host=pg.example.com
must_fail "database name is checked" \
  "name must match" \
  --set database.databases.Bad_Name.enabled=true --set database.databases.Bad_Name.secret.existingSecret=x
for reserved in postgres template0 template1; do
  must_fail "database name $reserved is reserved" \
    "reserved name" \
    --set "database.databases.$reserved.enabled=true" --set "database.databases.$reserved.secret.existingSecret=x"
done
must_fail "database named like the admin is reserved" \
  "reserved name" \
  --set database.admin.username=dbadmin \
  --set database.databases.dbadmin.enabled=true --set database.databases.dbadmin.secret.existingSecret=x

exit $fail
