#!/usr/bin/env bash
# Build every first-party image and push it to the PoC registry.
# Registry-based, not `kind load`: ArgoCD has to be able to pull them.
#
# Build contexts differ per component and match .github/workflows/dev-publish.yml:
#   catalog, plugins, mcp-mock -> repo root (they need go.mod)
#   ui                         -> ui/
#   portal                     -> naira-openmfp-portal/
set -euo pipefail
cd "$(dirname "$0")/../.."          # repo root
REG=${REG:-localhost:5001}
TAG=${TAG:-0.1.0-poc}
FAILED=()

build() {  # build <image-name> <dockerfile> <context>
  local name=$1 dockerfile=$2 context=$3
  printf '── %-36s' "${name}"
  if docker build -q -t "${REG}/${name}:${TAG}" -f "${dockerfile}" "${context}" >/dev/null 2>"/tmp/build-${name}.log"; then
    docker push -q "${REG}/${name}:${TAG}" >/dev/null 2>&1 && echo "ok" || { echo "PUSH FAILED"; FAILED+=("$name"); }
  else
    echo "BUILD FAILED (see /tmp/build-${name}.log)"; FAILED+=("$name")
  fi
}

build naira-catalog                    catalog/Dockerfile                                  .
build naira-ui                         ui/Dockerfile                                       ui
build naira-portal                     naira-openmfp-portal/Dockerfile                     naira-openmfp-portal
build naira-plugin-litellm             plugins/cmd/litellm/Dockerfile                      .
build naira-plugin-mlflow              plugins/cmd/mlflow/Dockerfile                       .
build naira-plugin-depl-calls-svc      plugins/cmd/depl_calls_svc/Dockerfile               .
build naira-plugin-depl-uses-litellm   plugins/cmd/depl_uses_litellm/Dockerfile            .
build naira-plugin-fluxcd              plugins/cmd/fluxcd/Dockerfile                       .
build naira-plugin-openmetadata        plugins/cmd/openmetadata/Dockerfile                 .
build naira-plugin-mcp-servers         plugins/cmd/mcp_servers/Dockerfile                  .
# RFC-009 prerequisite 2: mcp-mock is published from dev-publish only.
build naira-mcp-mock                   deploy/dev/stacks/core/tools/mcp-mock/Dockerfile    .

if [ ${#FAILED[@]} -gt 0 ]; then
  echo "FAILED: ${FAILED[*]}"; exit 1
fi
echo "── all 11 images pushed to ${REG} at ${TAG}"
