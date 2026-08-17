#!/usr/bin/env bash
# Creates a full per-PR E2E environment on an existing kind cluster: builds and
# loads every image, deploys the whole platform into a fresh namespace, waits
# for it to be genuinely ready, and seeds a starting dataset.
#
# Only --profile full is implemented. See SPEC-Kubernetes-Test-Environments-Implementation.md
# "Deployment Profiles" for why `core` (shared dependencies) is deferred.
#
# Usage:
#   create-environment.sh --env-id <ID> --tag <TAG> [--cluster-name <NAME>] [--profile full]
#   create-environment.sh --pr <N> --tag <TAG> [--run-id <ID>] [--cluster-name <NAME>] [--profile full]
#
# --env-id is preferred: pass an ID the caller already computed (e.g. the
# workflow, before this script runs) so a teardown step can find the right
# environment even if this script fails before it would otherwise have
# derived one. --pr/--run-id remain for local/manual convenience.
#
# On success, prints ENV_ID=<id> and (if GITHUB_ENV is set) appends ENV_ID and
# NAMESPACE to it for later workflow steps.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${E2E_DIR}/../.." && pwd)"

PROFILE="full"
CLUSTER_NAME="naira-idp-e2e"
RUN_ID="$(date +%s)"
PR=""
TAG=""
ENV_ID=""

usage() {
  echo "Usage: $0 --env-id <ID> --tag <TAG> [--cluster-name <NAME>] [--profile full]" >&2
  echo "       $0 --pr <N> --tag <TAG> [--run-id <ID>] [--cluster-name <NAME>] [--profile full]" >&2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --env-id) ENV_ID="$2"; shift 2 ;;
    --pr) PR="$2"; shift 2 ;;
    --tag) TAG="$2"; shift 2 ;;
    --run-id) RUN_ID="$2"; shift 2 ;;
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    --profile) PROFILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [ -z "${TAG}" ]; then echo "error: --tag is required" >&2; usage; exit 1; fi
if [ -z "${ENV_ID}" ] && [ -z "${PR}" ]; then
  echo "error: either --env-id or --pr is required" >&2
  usage
  exit 1
fi

if [ "${PROFILE}" != "full" ]; then
  echo "error: --profile ${PROFILE} is not implemented — only 'full' ships in this iteration." >&2
  echo "       See SPEC-Kubernetes-Test-Environments-Implementation.md, 'Deployment Profiles'." >&2
  exit 1
fi

if [ -z "${ENV_ID}" ]; then
  ENV_ID="pr-${PR}-${RUN_ID}"
fi
NAMESPACE="${ENV_ID}"
export ENV_ID NAMESPACE TAG

echo "==> Environment: ${ENV_ID} (namespace: ${NAMESPACE}, image tag: ${TAG})"

kubectl config use-context "kind-${CLUSTER_NAME}"

# ---------------------------------------------------------------------------
# Build and load images.
#
# catalog and the plugins need the repo root as build context (their
# Dockerfiles COPY from multiple repo-root-relative paths, e.g.
# plugins/pkg/pluginapi/) — mirrors deploy/dev/stacks/core/tasks/docker.yml.
# Each plugin Dockerfile defaults its PLUGIN build arg to its own directory
# name, so no --build-arg is needed here.
#
# portal and ui are self-contained frontend projects: their own directory
# *is* the build context (matches deploy/dev/stacks/core/tasks/{portal,ui}.yml
# exactly — a repo-root context breaks their `COPY frontend/ ./`-style paths).
# ---------------------------------------------------------------------------
build_and_load() {
  local image="$1" dockerfile_dir="$2"
  echo "==> Building ${image}:${TAG}"
  docker build -q -t "${image}:${TAG}" -f "${dockerfile_dir}/Dockerfile" "${REPO_ROOT}" >/dev/null
  kind load docker-image "${image}:${TAG}" --name "${CLUSTER_NAME}"
}

build_and_load_self_context() {
  local image="$1" dir="$2"
  echo "==> Building ${image}:${TAG}"
  docker build -q -t "${image}:${TAG}" "${dir}" >/dev/null
  kind load docker-image "${image}:${TAG}" --name "${CLUSTER_NAME}"
}

echo "==> Building and loading images"
build_and_load catalog "${REPO_ROOT}/catalog"
build_and_load_self_context portal "${REPO_ROOT}/naira-openmfp-portal"
build_and_load_self_context ui "${REPO_ROOT}/ui-poc"
build_and_load litellm "${REPO_ROOT}/plugins/cmd/litellm"
build_and_load mlflow "${REPO_ROOT}/plugins/cmd/mlflow"
build_and_load depl-calls-svc "${REPO_ROOT}/plugins/cmd/depl_calls_svc"
build_and_load depl-uses-litellm "${REPO_ROOT}/plugins/cmd/depl_uses_litellm"
build_and_load fluxcd "${REPO_ROOT}/plugins/cmd/fluxcd"
build_and_load openmetadata "${REPO_ROOT}/plugins/cmd/openmetadata"
build_and_load seed "${E2E_DIR}/seed"

# ---------------------------------------------------------------------------
# Deploy. catalog.yaml first — it creates the Namespace every other manifest
# depends on. quota.yaml right after, before anything else lands, so nothing
# is ever deployed unmetered even transiently.
# ---------------------------------------------------------------------------
apply() {
  echo "==> Applying $(basename "$1")"
  # Restrict substitution to our three template vars — manifests embed shell
  # snippets (e.g. llamacpp.yaml's $MODEL_PATH, postgres.yaml's $POSTGRES_DB)
  # with their own $VAR syntax that an unrestricted envsubst would blank out.
  envsubst '${NAMESPACE} ${ENV_ID} ${TAG}' < "$1" | kubectl apply -f -
}

apply "${E2E_DIR}/catalog.yaml"
apply "${E2E_DIR}/quota.yaml"
apply "${E2E_DIR}/keycloak.yaml"
apply "${E2E_DIR}/mlflow.yaml"
apply "${E2E_DIR}/postgres.yaml"
apply "${E2E_DIR}/litellm.yaml"
apply "${E2E_DIR}/llamacpp.yaml"
apply "${E2E_DIR}/portal.yaml"
apply "${E2E_DIR}/ui.yaml"

echo "==> Deploying OpenMetadata (Helm) — this is the long pole, 5-9 min"
helm repo add open-metadata https://helm.open-metadata.org/ --force-update
helm repo update open-metadata
apply "${E2E_DIR}/openmetadata-secrets.yaml"
helm upgrade --install openmetadata-dependencies open-metadata/openmetadata-dependencies \
  --namespace "${NAMESPACE}" --values "${E2E_DIR}/openmetadata-deps-values.yaml" --wait --timeout 15m
helm upgrade --install openmetadata open-metadata/openmetadata \
  --namespace "${NAMESPACE}" --values "${E2E_DIR}/openmetadata-values.yaml" --wait --timeout 15m

# ---------------------------------------------------------------------------
# Readiness. Each rollout status wait only succeeds once the Deployment's own
# readinessProbe passes — no fixed sleeps anywhere in this script.
# ---------------------------------------------------------------------------
echo "==> Waiting for dependency rollouts"
kubectl -n "${NAMESPACE}" rollout status deploy/keycloak --timeout=300s
kubectl -n "${NAMESPACE}" rollout status deploy/mlflow --timeout=180s
kubectl -n "${NAMESPACE}" rollout status deploy/postgres --timeout=180s
kubectl -n "${NAMESPACE}" rollout status deploy/litellm --timeout=900s
kubectl -n "${NAMESPACE}" rollout status deploy/llama-dummy-model --timeout=600s
kubectl -n "${NAMESPACE}" rollout status deploy/openmetadata --timeout=600s
kubectl -n "${NAMESPACE}" rollout status deploy/catalog --timeout=180s

echo "==> Seeding starting dataset"
apply "${E2E_DIR}/seed-job.yaml"
kubectl -n "${NAMESPACE}" wait --for=condition=complete "job/seed-${ENV_ID}" --timeout=180s

kubectl -n "${NAMESPACE}" get pods

if [ -n "${GITHUB_ENV:-}" ]; then
  echo "ENV_ID=${ENV_ID}" >> "${GITHUB_ENV}"
  echo "NAMESPACE=${NAMESPACE}" >> "${GITHUB_ENV}"
fi

echo "==> Ready"
echo "ENV_ID=${ENV_ID}"
