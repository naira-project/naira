#!/usr/bin/env bash
# Tears down an E2E environment. Namespace deletion removes everything
# namespace-scoped, including Helm-managed resources — there is no separate
# Cleanup Job or GC sweeper. ClusterRoles/ClusterRoleBindings are
# cluster-scoped and survive namespace deletion, so they're removed
# explicitly by the naira.io/env-id label.
#
# Usage: destroy-environment.sh --env-id <ID> [--cluster-name <NAME>] [--context <NAME>]
set -euo pipefail

CLUSTER_NAME="naira-idp-e2e"
ENV_ID=""
CONTEXT=""

usage() {
  echo "Usage: $0 --env-id <ID> [--cluster-name <NAME>] [--context <NAME>]" >&2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --env-id) ENV_ID="$2"; shift 2 ;;
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    --context) CONTEXT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [ -z "${ENV_ID}" ]; then echo "error: --env-id is required" >&2; usage; exit 1; fi

CONTEXT="${CONTEXT:-kind-${CLUSTER_NAME}}"

# The cluster may already be gone (e.g. the "Create kind cluster" step never
# ran) — tolerate that rather than failing an always()-run teardown step.
if ! kubectl config use-context "${CONTEXT}" 2>/dev/null; then
  echo "==> ${CONTEXT} context not found, nothing to tear down"
  exit 0
fi

FAILED=0

echo "==> Deleting namespace ${ENV_ID}"
if ! kubectl delete namespace "${ENV_ID}" --ignore-not-found --wait=true --timeout=120s; then
  echo "error: failed to delete namespace ${ENV_ID}" >&2
  FAILED=1
fi

echo "==> Deleting cluster-scoped resources for ${ENV_ID}"
if ! kubectl delete clusterrole,clusterrolebinding -l "naira.io/env-id=${ENV_ID}" --ignore-not-found; then
  echo "error: failed to delete cluster-scoped resources for ${ENV_ID}" >&2
  FAILED=1
fi

echo "==> Verifying cleanup"
LEFTOVER_CR="$(kubectl get clusterrole,clusterrolebinding -l "naira.io/env-id=${ENV_ID}" -o name)"
if [ -n "${LEFTOVER_CR}" ]; then
  echo "warning: cluster-scoped resources still present after delete:" >&2
  echo "${LEFTOVER_CR}" >&2
fi

if [ "${FAILED}" -ne 0 ]; then
  echo "==> Failed to fully destroy ${ENV_ID}" >&2
  exit 1
fi

echo "==> Destroyed ${ENV_ID}"
