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
  "plugin-depl-calls-svc,plugin-fluxcd,plugin-litellm,plugin-mcp-servers,plugin-mlflow,plugin-openmetadata" \
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
check "plugin resources override merges" "256Mi 200m 50m" \
  "$(render --set catalog.plugins.litellm.resources.limits.memory=256Mi \
     | yq "$CAT | .initContainers[] | select(.name == \"plugin-litellm\") | .resources | .limits.memory + \" \" + .limits.cpu + \" \" + .requests.cpu")"
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

# ── Task 3: RBAC ─────────────────────────────────────────────────────────────
check "ClusterRoles are release-prefixed" "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render | yq ea '[select(.kind == "ClusterRole") | .metadata.name] | sort | join(",")')"
check "bindings match roles" "t-catalog,t-plugin-depl-calls-svc,t-plugin-fluxcd" \
  "$(render | yq ea '[select(.kind == "ClusterRoleBinding") | .roleRef.name] | sort | join(",")')"
check "binding subject is the catalog SA in the release namespace" "catalog/naira" \
  "$(render | yq 'select(.kind == "ClusterRoleBinding" and .metadata.name == "t-plugin-fluxcd") | .subjects[0] | .name + "/" + .namespace')"
check "plugin rules copied" "namespaces,services" \
  "$(render | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-calls-svc") | .rules[0].resources | join(",")')"
check "disabled plugin: no RBAC" "" \
  "$(render --set catalog.plugins.fluxcd.enabled=false | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-fluxcd") | .metadata.name')"
check "two releases do not collide" "u-catalog" \
  "$(helm template u "$CHART" --namespace other "${BASE[@]}" | yq 'select(.kind == "ClusterRole" and .metadata.name == "u-catalog") | .metadata.name')"

# ── Task 4: plugin config ────────────────────────────────────────────────────
TR=(--set catalog.plugins.tech-radar.enabled=true --set-string 'catalog.plugins.tech-radar.config.data.radar\.yaml=schema_version: 1')
check "tech-radar ConfigMap from data" "schema_version: 1" \
  "$(render "${TR[@]}" | yq 'select(.kind == "ConfigMap" and .metadata.name == "plugin-tech-radar-config") | .data["radar.yaml"]')"
check "tech-radar mounted as a directory" "/etc/naira/techradar none" \
  "$(render "${TR[@]}" | yq "$CAT | .initContainers[] | select(.name == \"plugin-tech-radar\") | .volumeMounts[0] | .mountPath + \" \" + (.subPath // \"none\")")"
check "tech-radar volume source" "plugin-tech-radar-config" \
  "$(render "${TR[@]}" | yq "$CAT | .volumes[] | select(.name == \"plugin-tech-radar-config\") | .configMap.name")"
check "existingConfigMap: no generated ConfigMap" "" \
  "$(render --set catalog.plugins.tech-radar.enabled=true --set catalog.plugins.tech-radar.config.existingConfigMap=my-radar \
     | yq 'select(.kind == "ConfigMap" and .metadata.name == "plugin-tech-radar-config") | .metadata.name')"
check "existingConfigMap: volume points at it" "my-radar" \
  "$(render --set catalog.plugins.tech-radar.enabled=true --set catalog.plugins.tech-radar.config.existingConfigMap=my-radar \
     | yq "$CAT | .volumes[] | select(.name == \"plugin-tech-radar-config\") | .configMap.name")"
check "default plus tech-radar and depl-uses-litellm: 8 sidecars" "8" \
  "$(render "${TR[@]}" --set catalog.plugins.depl-uses-litellm.enabled=true | yq "$CAT | .initContainers | length")"
check "tech-radar registered in plugins.yaml" "localhost:50057" \
  "$(render "${TR[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.tech-radar.address')"

# ── Task 5: secrets ──────────────────────────────────────────────────────────
n=$(render | yq 'select(.kind == "Secret") | .metadata.name' | grep -c . || true)   # grep exits 1 on no match
check "no Secrets by default" "0" "${n:-0}"
check "catalog Secret when create=true" "k1" \
  "$(render_raw --set portal.enabled=false --set catalog.secret.create=true --set catalog.secret.data.LITELLM_API_KEY=k1 \
     | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "existingSecret reaches plugin env" "ext" \
  "$(render --set catalog.secret.existingSecret=ext | yq "$LITELLM_ENV | select(.name == \"LITELLM_API_KEY\") | .valueFrom.secretKeyRef.name")"
check "portal Secret when create=true" "s3" \
  "$(render_raw --set catalog.enabled=false --set portal.oidc.secret.create=true --set portal.oidc.secret.value=s3 \
     | yq 'select(.kind == "Secret" and .metadata.name == "portal-oidc") | .stringData["client-secret"]')"

# ── Task 6: ui and portal ────────────────────────────────────────────────────
UI='select(.kind == "Deployment" and .metadata.name == "ui") | .spec.template.spec.containers[0]'
PORTAL='select(.kind == "Deployment" and .metadata.name == "portal") | .spec.template.spec.containers[0]'
check "ui catalog upstream follows the release namespace" "http://catalog.naira.svc.cluster.local:8090" \
  "$(render | yq "$UI | .env[] | select(.name == \"CATALOG_UPSTREAM\") | .value")"
check "ui catalog upstream override" "http://x:1" \
  "$(render --set ui.catalogUpstream=http://x:1 | yq "$UI | .env[] | select(.name == \"CATALOG_UPSTREAM\") | .value")"
check "ui image" "ghcr.io/naira-project/naira-ui:0.1.0" "$(render | yq "$UI | .image")"
check "ui Service" "ui 80" \
  "$(render | yq 'select(.kind == "Service" and .metadata.name == "ui") | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "portal token URL from keycloak baseUrl and realm" \
  "http://keycloak.naira-deps.svc.cluster.local:8080/realms/naira/protocol/openid-connect/token" \
  "$(render | yq "$PORTAL | .env[] | select(.name == \"TOKEN_URL_KEYCLOAK\") | .value")"
check "portal client secret from portal-oidc" "portal-oidc/client-secret" \
  "$(render | yq "$PORTAL | .env[] | select(.name == \"OIDC_CLIENT_SECRET_KEYCLOAK\") | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "portal Service" "portal 3000" \
  "$(render | yq 'select(.kind == "Service" and .metadata.name == "portal") | .metadata.name + " " + (.spec.ports[0].port | tostring)')"
check "ui disabled" "" \
  "$(render --set ui.enabled=false | yq 'select(.kind == "Deployment" and .metadata.name == "ui") | .metadata.name')"

# ── Task 7: guards ───────────────────────────────────────────────────────────
must_fail "duplicate plugin port" "both use port 50051" --set catalog.plugins.mlflow.port=50051
must_fail "plugin port equal to the catalog port" "catalog and mlflow both use port 8090" --set catalog.plugins.mlflow.port=8090
check "numeric image tag renders as a string" "ghcr.io/naira-project/naira-catalog:20260924" \
  "$(render --set image.tag=20260924 | yq "$CAT | .containers[0].image")"
must_fail "catalog secret: existing and create" "catalog.secret: set existingSecret or create, not both" \
  --set catalog.secret.existingSecret=x --set catalog.secret.create=true
must_fail "portal secret: existing and create" "portal.oidc.secret: set existingSecret or create, not both" \
  --set portal.oidc.secret.existingSecret=x --set portal.oidc.secret.create=true
must_fail "tech-radar on without a radar file" "catalog.plugins.tech-radar.config: set existingConfigMap or data" \
  --set catalog.plugins.tech-radar.enabled=true
RENDER=render_raw must_fail "no catalog Secret source" \
  "catalog.plugins.litellm reads LITELLM_API_KEY from the catalog Secret: set catalog.secret.existingSecret or catalog.secret.create" \
  --set portal.enabled=false
RENDER=render_raw must_fail "no portal Secret source" "portal reads its OIDC client secret: set portal.oidc.secret.existingSecret or portal.oidc.secret.create" \
  --set catalog.secret.existingSecret=x
check "no Secret needed when nothing reads one" "ok" \
  "$(render_raw --set portal.enabled=false --set catalog.plugins.litellm.enabled=false --set catalog.plugins.openmetadata.enabled=false >/dev/null && echo ok)"
check "a plugin naming its own Secret needs no catalog Secret" "ok" \
  "$(render_raw --set portal.enabled=false --set catalog.plugins.openmetadata.enabled=false --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.name=own >/dev/null && echo ok)"
check "disabled plugin may share a port" "ok" \
  "$(render --set catalog.plugins.mlflow.port=50051 --set catalog.plugins.mlflow.enabled=false >/dev/null && echo ok)"

# ── Task 8: dev values ───────────────────────────────────────────────────────
DEV=(-f "$CHART/values-dev.yaml")
check "dev: local image names" "naira-catalog:0.1.0" "$(render_raw "${DEV[@]}" | yq "$CAT | .containers[0].image")"
check "dev: sidecars (openmetadata off, tech-radar on)" \
  "plugin-depl-calls-svc,plugin-depl-uses-litellm,plugin-fluxcd,plugin-litellm,plugin-mcp-servers,plugin-mlflow,plugin-tech-radar" \
  "$(render_raw "${DEV[@]}" | yq ea "[$CAT | .initContainers[].name] | join(\",\")")"
check "dev: catalog Secret matches test-dependencies master key" "sk-local-litellm" \
  "$(render_raw "${DEV[@]}" | yq 'select(.kind == "Secret" and .metadata.name == "catalog-secrets") | .stringData.LITELLM_API_KEY')"
check "dev: portal Secret" "naira-local-dev-secret" \
  "$(render_raw "${DEV[@]}" | yq 'select(.kind == "Secret" and .metadata.name == "portal-oidc") | .stringData["client-secret"]')"
check "dev: radar file is main's" "Naira Tech Radar" \
  "$(render_raw "${DEV[@]}" | yq 'select(.kind == "ConfigMap" and .metadata.name == "plugin-tech-radar-config") | .data["radar.yaml"]' | yq '.radar.title')"
check "helm lint ci values" "ok" "$(helm lint "$CHART" "${BASE[@]}" >/dev/null && echo ok)"
check "helm lint dev" "ok" "$(helm lint "$CHART" "${DEV[@]}" >/dev/null && echo ok)"

# ── Task 9: fix-pass regressions (F2, F3, F4) ────────────────────────────────
check "numeric release name renders a string label" "string" \
  "$(helm template 123 "$CHART" --namespace naira "${BASE[@]}" | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .metadata.labels["app.kubernetes.io/instance"] | tag' | sed 's/!!//;s/str/string/')"
check "rbac without rules renders no ClusterRole" "" \
  "$(render --set catalog.plugins.mlflow.rbac.foo=bar | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-mlflow") | .metadata.name')"
must_fail "plugin without an image" "image.repository is required" \
  --set catalog.plugins.newone.enabled=true --set catalog.plugins.newone.port=50099
must_fail "secretKeyRef without key" "secretKeyRef.key is required" \
  --set catalog.plugins.litellm.env.LITELLM_API_KEY.secretKeyRef.key=null
RENDER=render_raw must_fail "create with empty data" "catalog.secret.data is empty" \
  --set portal.enabled=false --set catalog.secret.create=true

# ── Review fixes: portal Secret guard, github plugin ─────────────────────────
RENDER=render_raw must_fail "portal create with empty value" "portal.oidc.secret.value is empty" \
  --set catalog.enabled=false --set portal.oidc.secret.create=true
check "github off by default" "" \
  "$(render | yq "$CAT | .initContainers[] | select(.name == \"plugin-github\") | .name")"
GH=(--set catalog.plugins.github.enabled=true)
check "github token from the catalog Secret" "catalog-secrets/GITHUB_TOKEN" \
  "$(render "${GH[@]}" | yq "$CAT | .initContainers[] | select(.name == \"plugin-github\") | .env[] | select(.name == \"GITHUB_TOKEN\") | .valueFrom.secretKeyRef | .name + \"/\" + .key")"
check "github registered in plugins.yaml" "localhost:50059" \
  "$(render "${GH[@]}" | yq "$PLUGINS_YAML" | yq '.plugins.github.address')"
check "github RBAC when enabled" "t-plugin-github" \
  "$(render "${GH[@]}" | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-github") | .metadata.name')"
RENDER=render_raw must_fail "github on without a catalog Secret source" "catalog.plugins.github reads GITHUB_TOKEN from the catalog Secret" \
  --set portal.enabled=false --set catalog.plugins.github.enabled=true --set catalog.plugins.litellm.enabled=false --set catalog.plugins.openmetadata.enabled=false

check "depl-uses-litellm off by default: no Secret read grant" "0" \
  "$(render | yq ea '[select(.kind == "ClusterRole") | .rules[] | select(.resources[] == "secrets")] | length')"
check "depl-uses-litellm enabled: ClusterRole with secrets" "t-plugin-depl-uses-litellm" \
  "$(render --set catalog.plugins.depl-uses-litellm.enabled=true | yq 'select(.kind == "ClusterRole" and .metadata.name == "t-plugin-depl-uses-litellm") | .metadata.name')"
LONG=$(printf 'a%.0s' $(seq 1 57))
must_fail "plugin name over 56 characters" "name too long" \
  --set "catalog.plugins.${LONG}.enabled=true" --set "catalog.plugins.${LONG}.port=50099" --set "catalog.plugins.${LONG}.image.repository=x"

# ── Pod hardening and scheduling ─────────────────────────────────────────────
POD='select(.kind == "Deployment" and .metadata.name == "%s") | .spec.template.spec'
pod() { printf "$POD" "$1"; }
SIDE='.initContainers[] | select(.name == "plugin-litellm")'
check "catalog runs as the distroless uid" "65532 true" \
  "$(render | yq "$(pod catalog) | .containers[0].securityContext | (.runAsUser | tostring) + \" \" + (.runAsNonRoot | tostring)")"
check "sidecar security context defaults" "false ALL RuntimeDefault" \
  "$(render | yq "$(pod catalog) | $SIDE | .securityContext | (.allowPrivilegeEscalation | tostring) + \" \" + .capabilities.drop[0] + \" \" + .seccompProfile.type")"
check "plugin securityContext merges over defaults" "true true" \
  "$(render --set catalog.plugins.litellm.securityContext.readOnlyRootFilesystem=true \
     | yq "$(pod catalog) | $SIDE | .securityContext | (.readOnlyRootFilesystem | tostring) + \" \" + (.runAsNonRoot | tostring)")"
check "sidecar startup probe is a TCP check on the plugin port" "50051 60" \
  "$(render | yq "$(pod catalog) | $SIDE | .startupProbe | (.tcpSocket.port | tostring) + \" \" + (.failureThreshold | tostring)")"
check "plugin startupProbe merges over defaults" "1 5" \
  "$(render --set catalog.plugins.litellm.startupProbe.failureThreshold=5 \
     | yq "$(pod catalog) | $SIDE | .startupProbe | (.periodSeconds | tostring) + \" \" + (.failureThreshold | tostring)")"
check "catalog startup probe" "/healthz" "$(render | yq "$(pod catalog) | .containers[0].startupProbe.httpGet.path")"
check "ui startup probe" "/" "$(render | yq "$(pod ui) | .containers[0].startupProbe.httpGet.path")"
check "portal startup probe" "/" "$(render | yq "$(pod portal) | .containers[0].startupProbe.httpGet.path")"
check "ui does not drop capabilities" "null" "$(render | yq "$(pod ui) | .containers[0].securityContext.capabilities")"
check "portal seccomp profile" "RuntimeDefault" "$(render | yq "$(pod portal) | .containers[0].securityContext.seccompProfile.type")"
check "no scheduling fields by default" "false" \
  "$(render | yq "$(pod catalog) | (has(\"imagePullSecrets\") or has(\"nodeSelector\") or has(\"tolerations\") or has(\"affinity\") or has(\"topologySpreadConstraints\") or has(\"priorityClassName\") or has(\"securityContext\")) | tostring")"
for w in catalog ui portal; do
  check "$w imagePullSecrets" "regcred" \
    "$(render --set 'imagePullSecrets[0].name=regcred' | yq "$(pod $w) | .imagePullSecrets[0].name")"
  check "$w podSecurityContext" "1000" \
    "$(render --set $w.podSecurityContext.fsGroup=1000 | yq "$(pod $w) | .securityContext.fsGroup")"
  check "$w priorityClassName" "high" "$(render --set $w.priorityClassName=high | yq "$(pod $w) | .priorityClassName")"
  check "$w nodeSelector" "linux" "$(render --set $w.nodeSelector.os=linux | yq "$(pod $w) | .nodeSelector.os")"
  check "$w tolerations" "dedicated" \
    "$(render --set 'catalog.tolerations[0].key=dedicated' --set 'ui.tolerations[0].key=dedicated' --set 'portal.tolerations[0].key=dedicated' | yq "$(pod $w) | .tolerations[0].key")"
  check "$w pod annotations" "v" \
    "$(render --set-string $w.podAnnotations.k=v | yq "select(.kind == \"Deployment\" and .metadata.name == \"$w\") | .spec.template.metadata.annotations.k")"
  check "$w soft anti-affinity" "app.kubernetes.io/name=$w 100" \
    "$(render --set $w.podAntiAffinity=soft \
       | yq "$(pod $w) | .affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0] | \"app.kubernetes.io/name=\" + .podAffinityTerm.labelSelector.matchLabels[\"app.kubernetes.io/name\"] + \" \" + (.weight | tostring)")"
  check "$w hard anti-affinity" "kubernetes.io/hostname" \
    "$(render --set $w.podAntiAffinity=hard \
       | yq "$(pod $w) | .affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[0].topologyKey")"
  check "$w topology spread gets its own selector" "$w" \
    "$(render --set $w.topologySpreadConstraints[0].maxSkew=1 --set $w.topologySpreadConstraints[0].topologyKey=zone --set $w.topologySpreadConstraints[0].whenUnsatisfiable=DoNotSchedule \
       | yq "$(pod $w) | .topologySpreadConstraints[0].labelSelector.matchLabels[\"app.kubernetes.io/name\"]")"
done
check "catalog keeps its checksum annotation with podAnnotations" "true" \
  "$(render --set-string catalog.podAnnotations.k=v | yq 'select(.kind == "Deployment" and .metadata.name == "catalog") | .spec.template.metadata.annotations | has("checksum/plugin-config")')"
check "explicit affinity.podAntiAffinity wins over the preset" "null" \
  "$(render --set catalog.podAntiAffinity=hard --set catalog.affinity.podAntiAffinity.x=y \
     | yq "$(pod catalog) | .affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution")"
check "topology spread keeps an explicit selector" "mine" \
  "$(render --set catalog.topologySpreadConstraints[0].maxSkew=1 --set catalog.topologySpreadConstraints[0].topologyKey=zone \
     --set catalog.topologySpreadConstraints[0].whenUnsatisfiable=DoNotSchedule --set catalog.topologySpreadConstraints[0].labelSelector.matchLabels.app=mine \
     | yq "$(pod catalog) | .topologySpreadConstraints[0].labelSelector.matchLabels.app")"
must_fail "podAntiAffinity value" "ui.podAntiAffinity: must be soft, hard or empty" --set ui.podAntiAffinity=maybe

# ── PDB, HPA, NetworkPolicy ──────────────────────────────────────────────────
PDB='select(.kind == "PodDisruptionBudget" and .metadata.name == "%s") | .spec'
check "no PDB, HPA or NetworkPolicy by default" "0" \
  "$(render | yq ea '[select(.kind == "PodDisruptionBudget" or .kind == "HorizontalPodAutoscaler" or .kind == "NetworkPolicy")] | length')"
check "PDB with minAvailable" "1" "$(render --set catalog.pdb.enabled=true --set catalog.pdb.minAvailable=1 | yq "$(printf "$PDB" catalog) | .minAvailable")"
check "PDB minAvailable wins over maxUnavailable" "null" \
  "$(render --set catalog.pdb.enabled=true --set catalog.pdb.minAvailable=1 | yq "$(printf "$PDB" catalog) | .maxUnavailable")"
check "PDB defaults to maxUnavailable 1" "1" "$(render --set ui.pdb.enabled=true | yq "$(printf "$PDB" ui) | .maxUnavailable")"
check "PDB percentage stays a string" "!!str" \
  "$(render --set ui.pdb.enabled=true --set-string ui.pdb.maxUnavailable=50% | yq "$(printf "$PDB" ui) | .maxUnavailable | tag")"
check "PDB selects the workload" "portal" \
  "$(render --set portal.pdb.enabled=true | yq "$(printf "$PDB" portal) | .selector.matchLabels[\"app.kubernetes.io/name\"]")"
check "PDB not rendered for a disabled workload" "0" \
  "$(render --set ui.enabled=false --set ui.pdb.enabled=true | yq ea '[select(.kind == "PodDisruptionBudget")] | length')"
HPA='select(.kind == "HorizontalPodAutoscaler" and .metadata.name == "%s") | .spec'
check "ui HPA targets its Deployment" "ui 1 3" \
  "$(render --set ui.autoscaling.enabled=true | yq "$(printf "$HPA" ui) | .scaleTargetRef.name + \" \" + (.minReplicas | tostring) + \" \" + (.maxReplicas | tostring)")"
check "HPA omits Deployment replicas" "null" \
  "$(render --set ui.autoscaling.enabled=true | yq "$(pod ui | sed 's/ | .spec.template.spec//') | .spec.replicas")"
check "Deployment keeps replicas without HPA" "1" \
  "$(render | yq 'select(.kind == "Deployment" and .metadata.name == "ui") | .spec.replicas')"
check "HPA with memory target only" "memory" \
  "$(render --set portal.autoscaling.enabled=true --set portal.autoscaling.targetCPUUtilizationPercentage=null --set portal.autoscaling.targetMemoryUtilizationPercentage=70 \
     | yq "$(printf "$HPA" portal) | .metrics[0].resource.name")"
check "no catalog HPA" "0" \
  "$(render --set catalog.autoscaling.enabled=true | yq ea '[select(.kind == "HorizontalPodAutoscaler" and .metadata.name == "catalog")] | length')"
must_fail "HPA min above max" "ui.autoscaling: minReplicas is greater than maxReplicas" \
  --set ui.autoscaling.enabled=true --set ui.autoscaling.minReplicas=5
must_fail "HPA without a target" "portal.autoscaling: set targetCPUUtilizationPercentage" \
  --set portal.autoscaling.enabled=true --set portal.autoscaling.targetCPUUtilizationPercentage=null
NP='select(.kind == "NetworkPolicy") | .spec'
check "NetworkPolicy admits the ui pods on the catalog port" "ui 8090" \
  "$(render --set catalog.networkPolicy.enabled=true \
     | yq "$NP | .ingress[0] | .from[0].podSelector.matchLabels[\"app.kubernetes.io/name\"] + \" \" + (.ports[0].port | tostring)")"
check "NetworkPolicy selects the catalog pods" "catalog" \
  "$(render --set catalog.networkPolicy.enabled=true | yq "$NP | .podSelector.matchLabels[\"app.kubernetes.io/name\"]")"
check "NetworkPolicy with the ui disabled admits nobody" "false Ingress" \
  "$(render --set ui.enabled=false --set catalog.networkPolicy.enabled=true | yq "$NP | (has(\"ingress\") | tostring) + \" \" + .policyTypes[0]")"
check "NetworkPolicy extraIngress is appended" "2 ingress-nginx" \
  "$(render --set catalog.networkPolicy.enabled=true --set 'catalog.networkPolicy.extraIngress[0].from[0].namespaceSelector.matchLabels.n=ingress-nginx' \
     | yq "$NP | (.ingress | length | tostring) + \" \" + .ingress[1].from[0].namespaceSelector.matchLabels.n")"
check "hardened values render" "ok" \
  "$(render_raw -f "$CHART/ci/hardened-values.yaml" >/dev/null && echo ok)"
check "helm lint hardened" "ok" "$(helm lint "$CHART" -f "$CHART/ci/hardened-values.yaml" >/dev/null && echo ok)"

exit $fail
