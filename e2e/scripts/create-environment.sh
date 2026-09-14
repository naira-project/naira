#!/usr/bin/env bash
# Creates an E2E environment for one scenario on an existing kind cluster:
# builds and loads only the images that scenario's rendered naira-core
# values actually reference, installs the naira-core and components Helm
# charts (deploy/charts/{naira-core,components} — the same charts
# deploy/dev's Taskfile installs) into a fresh namespace with e2e-target
# values, waits for it to be genuinely ready, and seeds a starting dataset.
#
# deploy/charts/{naira-core,components}/ are the single source of truth for
# every component any scenario might deploy (see CONTEXT.md's "Component"
# and "Plugin" entries) — this script only supplies:
#   - deploy/environments/e2e/*.values.yaml: e2e-wide baseline (mostly
#     everything off; catalog+keycloak always on, see e2e/README.md)
#   - e2e/<scenario>/*.values.yaml: which components/plugins this scenario
#     turns on, plus any scenario-specific config (replaces the old
#     components.env)
#   - a small generated runtime overlay: the one thing genuinely dynamic
#     per run — collapsing every component into this run's own namespace,
#     so concurrent PR environments on one shared cluster don't collide
#   - e2e/<scenario>/extras.env (optional): scenario-only test fixtures that
#     aren't in either chart (e.g. chatbot1), applied as plain manifests
#
# Usage:
#   create-environment.sh --scenario <NAME> --env-id <ID> --tag <TAG> [--cluster-name <NAME>]
#
# On success, prints ENV_ID=<id> and (if GITHUB_ENV is set) appends ENV_ID and
# NAMESPACE to it for later workflow steps.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${E2E_DIR}/.." && pwd)"
CHARTS_DIR="${REPO_ROOT}/deploy/charts"
ENV_VALUES_DIR="${REPO_ROOT}/deploy/environments/e2e"

CLUSTER_NAME="naira-idp-e2e"
TAG=""
ENV_ID=""
SCENARIO=""

usage() {
  cat >&2 <<'EOF'
Usage: create-environment.sh --scenario <NAME> --env-id <ID> --tag <TAG> [--cluster-name <NAME>]

Flags:
  --scenario NAME    Required. Name of a directory under e2e/ whose
                      values.yaml files select what to deploy (e.g.
                      litellm_chatbot_to_catalog_api).
  --env-id ID         Required. Namespace/environment ID to use. The caller
                      should compute this before invoking the script so a
                      teardown step can find it even if setup fails.
  --tag TAG           Required. Image tag to build/deploy every component
                      image under.
  --cluster-name NAME Kind cluster context to deploy into. Defaults to
                      naira-idp-e2e.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --scenario) SCENARIO="$2"; shift 2 ;;
    --env-id) ENV_ID="$2"; shift 2 ;;
    --tag) TAG="$2"; shift 2 ;;
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [ -z "${SCENARIO}" ]; then echo "error: --scenario is required" >&2; usage; exit 1; fi
if [ -z "${TAG}" ]; then echo "error: --tag is required" >&2; usage; exit 1; fi
if [ -z "${ENV_ID}" ]; then
  echo "error: --env-id is required" >&2
  usage
  exit 1
fi

SCENARIO_DIR="${E2E_DIR}/${SCENARIO}"
if [ ! -d "${SCENARIO_DIR}" ]; then
  echo "error: no such scenario '${SCENARIO}' (${SCENARIO_DIR} not found)" >&2
  exit 1
fi

NAMESPACE="${ENV_ID}"
export ENV_ID NAMESPACE TAG

echo "==> Environment: ${ENV_ID} (namespace: ${NAMESPACE}, image tag: ${TAG}, scenario: ${SCENARIO})"

kubectl config use-context "kind-${CLUSTER_NAME}"

echo "==> Applying namespace and quota"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
envsubst '${NAMESPACE}' < "${E2E_DIR}/base/quota.yaml" | kubectl apply -f -

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

echo "==> Resolving components chart dependencies"
helm dependency build "${CHARTS_DIR}/components" >/dev/null

# ---------------------------------------------------------------------------
# Layered values: chart defaults -> e2e baseline -> scenario overrides ->
# generated runtime overlay (this run's own namespace, everywhere).
# ---------------------------------------------------------------------------
NAIRA_CORE_VALUES=(-f "${ENV_VALUES_DIR}/naira-core.values.yaml")
COMPONENTS_VALUES=(-f "${ENV_VALUES_DIR}/components.values.yaml")

if [ -f "${SCENARIO_DIR}/components.values.yaml" ]; then
  COMPONENTS_VALUES+=(-f "${SCENARIO_DIR}/components.values.yaml")
fi

# The scenario's own naira-core.values.yaml (if any) is itself an envsubst
# template — it references ${NAMESPACE} directly for plugin env vars that
# reach other in-cluster services (see e2e/README.md for why the Deployment
# needs this) — so it's rendered to a temp file rather than passed as-is.
if [ -f "${SCENARIO_DIR}/naira-core.values.yaml" ]; then
  envsubst '${NAMESPACE}' < "${SCENARIO_DIR}/naira-core.values.yaml" > "${WORKDIR}/scenario-naira-core.values.yaml"
  NAIRA_CORE_VALUES+=(-f "${WORKDIR}/scenario-naira-core.values.yaml")
fi

# Generic runtime overlay — the same for every scenario, so it isn't a repo
# file: collapses every component into this run's own namespace instead of
# dev's fixed per-component namespaces, so concurrent PR environments on one
# shared cluster don't collide (see e2e/README.md).
envsubst '${NAMESPACE}' > "${WORKDIR}/runtime-naira-core.values.yaml" <<'EOF'
namespace: ${NAMESPACE}
mcpMock:
  namespace: ${NAMESPACE}
catalog:
  keycloak:
    baseUrl: http://keycloak.${NAMESPACE}.svc.cluster.local:8080
    issuer: http://keycloak.${NAMESPACE}.svc.cluster.local:8080/realms/naira
EOF
NAIRA_CORE_VALUES+=(-f "${WORKDIR}/runtime-naira-core.values.yaml" --set "tag=${TAG}" --set "rbacSuffix=${ENV_ID}")

envsubst '${NAMESPACE}' > "${WORKDIR}/runtime-components.values.yaml" <<'EOF'
keycloak:
  namespace: ${NAMESPACE}
  hostname: http://keycloak.${NAMESPACE}.svc.cluster.local:8080
postgres:
  namespace: ${NAMESPACE}
litellm:
  namespace: ${NAMESPACE}
  databaseUrl: postgresql://litellm:litellm-local-password@postgres.${NAMESPACE}.svc.cluster.local:5432/litellm
llamacpp:
  namespace: ${NAMESPACE}
vllm:
  namespace: ${NAMESPACE}
mlflow:
  namespace: ${NAMESPACE}
openmetadata:
  namespace: ${NAMESPACE}
monitoring:
  namespaceOverride: ${NAMESPACE}
EOF
COMPONENTS_VALUES+=(-f "${WORKDIR}/runtime-components.values.yaml")

# ---------------------------------------------------------------------------
# Build and load images. Only catalog's own image and its plugin sidecars,
# plus ui/portal/mcp-mock, are ever locally built — every components chart
# component is a public image. Which of those this scenario actually needs
# is derived from the real rendered manifest (the same values used for
# install below), not a separately-maintained list.
# ---------------------------------------------------------------------------
component_build_dir() {
  case "$1" in
    ui) echo "${REPO_ROOT}/ui" ;;
    portal) echo "${REPO_ROOT}/naira-openmfp-portal" ;;
    mcp-mock) echo "${REPO_ROOT}/deploy/dev/tools/mcp-mock" ;;
    *) echo "${REPO_ROOT}/plugins/cmd/${1//-/_}" ;;
  esac
}

build_and_load() {
  local image="$1" dockerfile_dir="$2" build_root="${3:-$2}"
  echo "==> Building ${image}:${TAG}"
  docker build -t "${image}:${TAG}" -f "${dockerfile_dir}/Dockerfile" "${build_root}"
  kind load docker-image "${image}:${TAG}" --name "${CLUSTER_NAME}"
}

echo "==> Building and loading images"
build_and_load catalog "${REPO_ROOT}/catalog" "${REPO_ROOT}"
build_and_load seed "${SCENARIO_DIR}/seed"

LOCAL_IMAGES="$(helm template naira-core "${CHARTS_DIR}/naira-core" \
  "${NAIRA_CORE_VALUES[@]}" \
  | grep -oE "image: \"[a-z0-9-]+:${TAG}\"" \
  | sed -E "s/image: \"([a-z0-9-]+):${TAG}\"/\1/" \
  | sort -u)"

for image in ${LOCAL_IMAGES}; do
  # catalog is already built above with its own (non-plugin) source path.
  [ "${image}" = "catalog" ] && continue
  build_dir="$(component_build_dir "${image}")"
  case "${image}" in
    ui|portal) build_and_load "${image}" "${build_dir}" ;;
    *) build_and_load "${image}" "${build_dir}" "${REPO_ROOT}" ;;
  esac
done

# ---------------------------------------------------------------------------
# Install. --create-namespace covers components: its openmetadata dependency
# has no namespace override of its own, so it deploys into this release's
# namespace — every other component in that chart sets its own explicit
# namespace via the runtime overlay above and is unaffected.
# ---------------------------------------------------------------------------
echo "==> Installing naira-core"
helm upgrade --install "naira-core-${ENV_ID}" "${CHARTS_DIR}/naira-core" \
  "${NAIRA_CORE_VALUES[@]}" \
  --namespace "${NAMESPACE}" --create-namespace --wait --timeout 5m

echo "==> Installing components"
helm upgrade --install "components-${ENV_ID}" "${CHARTS_DIR}/components" \
  "${COMPONENTS_VALUES[@]}" \
  --namespace "${NAMESPACE}" --create-namespace --wait --timeout 15m

# ---------------------------------------------------------------------------
# Scenario-only extras that aren't in either chart (e.g. chatbot1).
# ---------------------------------------------------------------------------
if [ -f "${SCENARIO_DIR}/extras.env" ]; then
  # shellcheck source=/dev/null
  source "${SCENARIO_DIR}/extras.env"
  for extra in ${EXTRAS:-}; do
    build_and_load "${extra}" "${E2E_DIR}/components/${extra}"
    echo "==> Applying ${extra}"
    envsubst '${NAMESPACE} ${TAG}' < "${E2E_DIR}/components/${extra}.yaml" | kubectl apply -f -
  done
fi

echo "==> Seeding starting dataset"
envsubst '${NAMESPACE} ${ENV_ID} ${TAG}' < "${SCENARIO_DIR}/seed/seed-job.yaml" | kubectl apply -f -
kubectl -n "${NAMESPACE}" wait --for=condition=complete "job/seed-${ENV_ID}" --timeout=3m

kubectl -n "${NAMESPACE}" get pods

if [ -n "${GITHUB_ENV:-}" ]; then
  echo "ENV_ID=${ENV_ID}" >> "${GITHUB_ENV}"
  echo "NAMESPACE=${NAMESPACE}" >> "${GITHUB_ENV}"
fi

echo "==> Ready"
echo "ENV_ID=${ENV_ID}"
