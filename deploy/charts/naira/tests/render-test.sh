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
# Requires helm and mikefarah yq v4 (both pinned in mise.toml).
set -euo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)"
# Release PRs bump Chart.yaml, so image tags are read from it, not hardcoded.
APP_VERSION=$(yq '.appVersion' "$CHART/Chart.yaml")
fail=0

# BASE lists flags to add a minimal set of values required to render the chart.
BASE=(-f "$CHART/ci/default-values.yaml")
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
  "$(render_raw --set portal.enabled=false \
      --set catalog.secret.create=true --set catalog.secret.data.LITELLM_API_KEY=k1 \
    | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "existingSecret reaches plugin env" \
  "ext" \
  "$(render --set catalog.secret.existingSecret=ext \
    | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\") | .valueFrom.secretKeyRef.name")"
check "portal Secret when create=true" \
  "s3" \
  "$(render_raw --set catalog.enabled=false \
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
  "$(render_raw --set portal.enabled=false \
      --set catalog.plugins.litellm.enabled=false \
      --set catalog.plugins.openmetadata.enabled=false >/dev/null && echo ok)"
check "a plugin naming its own Secret needs no catalog Secret" \
  "ok" \
  "$(render_raw --set portal.enabled=false \
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
check "all-fields values render" \
  "ok" \
  "$(render_raw -f "$CHART/ci/all-fields-values.yaml" >/dev/null && echo ok)"

exit $fail
