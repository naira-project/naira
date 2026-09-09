#!/usr/bin/env bash
# Creates an E2E environment for one scenario on an existing kind cluster:
# builds and loads only the images that scenario's components.env asks for,
# deploys them into a fresh namespace, waits for it to be genuinely ready,
# and seeds a starting dataset.
#
# e2e/components/ holds every piece any scenario might use (catalog is
# always deployed with a configurable set of plugin sidecars; keycloak is
# always deployed for auth; everything else — litellm, mlflow, llamacpp,
# openmetadata, postgres, ui, portal, chatbot1 — is opt-in). Each scenario
# under e2e/<scenario>/ picks its subset via components.env (COMPONENTS,
# PLUGINS) — see e2e/README.md for the full model.
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

CLUSTER_NAME="naira-idp-e2e"
TAG=""
ENV_ID=""
SCENARIO=""

usage() {
  cat >&2 <<'EOF'
Usage: create-environment.sh --scenario <NAME> --env-id <ID> --tag <TAG> [--cluster-name <NAME>]

Flags:
  --scenario NAME    Required. Name of a directory under e2e/ whose
                      components.env selects what to deploy (e.g.
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
if [ ! -f "${SCENARIO_DIR}/components.env" ]; then
  echo "error: no such scenario '${SCENARIO}' (${SCENARIO_DIR}/components.env not found)" >&2
  exit 1
fi
# shellcheck source=/dev/null
source "${SCENARIO_DIR}/components.env"
COMPONENTS="${COMPONENTS:-}"
PLUGINS="${PLUGINS:-}"

NAMESPACE="${ENV_ID}"
export ENV_ID NAMESPACE TAG

echo "==> Environment: ${ENV_ID} (namespace: ${NAMESPACE}, image tag: ${TAG}, scenario: ${SCENARIO})"
echo "==> Components: ${COMPONENTS:-<none>} · Plugins: ${PLUGINS:-<none>}"

kubectl config use-context "kind-${CLUSTER_NAME}"

# ---------------------------------------------------------------------------
# Component metadata lookups. Every scenario draws from the same pool in
# e2e/components/ — these functions are the only place that knows how to
# build/apply/wait-for each one, so a scenario's components.env only ever
# has to name what it wants.
# ---------------------------------------------------------------------------
# Self-context build dir for components that ship their own image, or empty
# for components that run a public image with no local build.
component_build_dir() {
  case "$1" in
    ui) echo "${REPO_ROOT}/ui-poc" ;;
    portal) echo "${REPO_ROOT}/naira-openmfp-portal" ;;
    chatbot1) echo "${E2E_DIR}/components/chatbot1" ;;
    litellm|mlflow|llamacpp|openmetadata|postgres) echo "" ;;
    *) echo "error: unknown component '$1'" >&2; exit 1 ;;
  esac
}

component_wait_timeout() {
  case "$1" in
    litellm) echo "15m" ;;
    llamacpp) echo "10m" ;;
    openmetadata) echo "10m" ;;
    *) echo "3m" ;;
  esac
}

# ---------------------------------------------------------------------------
# Build and load images.
#
# catalog and the plugins need the repo root as build context (their
# Dockerfiles COPY from multiple repo-root-relative paths, e.g.
# plugins/pkg/pluginapi/) — mirrors deploy/dev/stacks/core/tasks/docker.yml.
# Each plugin Dockerfile defaults its PLUGIN build arg to its own directory
# name, so no --build-arg is needed here.
#
# ui, portal, and chatbot1 are self-contained frontend/app projects: their
# own directory *is* the build context (matches
# deploy/dev/stacks/core/tasks/{portal,ui}.yml exactly — a repo-root context
# breaks their `COPY frontend/ ./`-style paths).
# ---------------------------------------------------------------------------
build_and_load() {
  local image="$1" dockerfile_dir="$2" build_root="${3:-$2}"
  echo "==> Building ${image}:${TAG}"
  docker build -t "${image}:${TAG}" -f "${dockerfile_dir}/Dockerfile" "${build_root}"
  kind load docker-image "${image}:${TAG}" --name "${CLUSTER_NAME}"
}

echo "==> Building and loading images"
build_and_load catalog "${REPO_ROOT}/catalog" "${REPO_ROOT}"
build_and_load seed "${SCENARIO_DIR}/seed"
for plugin in ${PLUGINS}; do
  build_and_load "${plugin}" "${REPO_ROOT}/plugins/cmd/${plugin//-/_}" "${REPO_ROOT}"
done
for component in ${COMPONENTS}; do
  build_dir="$(component_build_dir "${component}")"
  if [ -n "${build_dir}" ]; then
    build_and_load "${component}" "${build_dir}"
  fi
done

# ---------------------------------------------------------------------------
# Deploy the namespace and quota before any workload, so nothing is ever
# deployed unmetered even transiently.
# ---------------------------------------------------------------------------
apply() {
  echo "==> Applying $(basename "$1")"
  # Restrict substitution to our template vars — manifests embed shell
  # snippets (e.g. postgres.yaml's $POSTGRES_DB) with their own $VAR syntax
  # that an unrestricted envsubst would blank out.
  envsubst '${NAMESPACE} ${ENV_ID} ${TAG} ${PLUGIN_ADDRESSES}' < "$1" | kubectl apply -f -
}

render_catalog() {
  echo "==> Rendering catalog (plugins: ${PLUGINS:-<none>})"
  local addresses="" plugin port=50051
  for plugin in ${PLUGINS}; do
    addresses="${addresses:+${addresses},}${plugin}=localhost:${port}"
    port=$((port + 1))
  done
  export PLUGIN_ADDRESSES="${addresses}"

  {
    envsubst '${NAMESPACE} ${ENV_ID} ${TAG} ${PLUGIN_ADDRESSES}' < "${E2E_DIR}/components/catalog/header.yaml"
    port=50051
    for plugin in ${PLUGINS}; do
      export PLUGIN_PORT="${port}"
      envsubst '${NAMESPACE} ${ENV_ID} ${TAG} ${PLUGIN_ADDRESSES} ${PLUGIN_PORT}' < "${E2E_DIR}/components/catalog/plugin-${plugin}.yaml"
      port=$((port + 1))
    done
    envsubst '${NAMESPACE} ${ENV_ID} ${TAG} ${PLUGIN_ADDRESSES}' < "${E2E_DIR}/components/catalog/footer.yaml"
  } | kubectl apply -f -
}

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
apply "${E2E_DIR}/base/quota.yaml"
apply "${E2E_DIR}/components/keycloak.yaml"
render_catalog

for component in ${COMPONENTS}; do
  if [ "${component}" = "openmetadata" ]; then
    echo "==> Deploying OpenMetadata (Helm) — this is the long pole, 5-9 min"
    helm repo add open-metadata https://helm.open-metadata.org/ --force-update
    helm repo update open-metadata
    apply "${E2E_DIR}/components/openmetadata-secrets.yaml"
    helm upgrade --install openmetadata-dependencies open-metadata/openmetadata-dependencies \
      --version 1.13.5 --namespace "${NAMESPACE}" --values "${E2E_DIR}/components/openmetadata-deps-values.yaml" --wait --timeout 15m
    helm upgrade --install openmetadata open-metadata/openmetadata \
      --version 1.13.5 --namespace "${NAMESPACE}" --values "${E2E_DIR}/components/openmetadata-values.yaml" --wait --timeout 15m
    continue
  fi
  apply "${E2E_DIR}/components/${component}.yaml"
done

# ---------------------------------------------------------------------------
# Readiness. Each rollout status wait only succeeds once the Deployment's own
# readinessProbe passes — no fixed sleeps anywhere in this script.
# ---------------------------------------------------------------------------
echo "==> Waiting for dependency rollouts"
for component in keycloak ${COMPONENTS} catalog; do
  kubectl -n "${NAMESPACE}" rollout status "deploy/${component}" \
    --timeout="$(component_wait_timeout "${component}")"
done

echo "==> Seeding starting dataset"
apply "${SCENARIO_DIR}/seed/seed-job.yaml"
kubectl -n "${NAMESPACE}" wait --for=condition=complete "job/seed-${ENV_ID}" --timeout=3m

kubectl -n "${NAMESPACE}" get pods

if [ -n "${GITHUB_ENV:-}" ]; then
  echo "ENV_ID=${ENV_ID}" >> "${GITHUB_ENV}"
  echo "NAMESPACE=${NAMESPACE}" >> "${GITHUB_ENV}"
fi

echo "==> Ready"
echo "ENV_ID=${ENV_ID}"
