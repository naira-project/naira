#!/usr/bin/env bash
# Validates the rendered chart against the Kubernetes API schemas; helm lint
# does not see wrong field names or types. One run per values file the chart
# supports (bare defaults do not render by design).
# Requires helm and kubeconform (both pinned in mise.toml).
set -euo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)"
# Keep in sync with the kind node image and deploy/dev/stacks/core/infra/kind/kind-config.yaml.
KUBERNETES_VERSION=${KUBERNETES_VERSION:-1.36.1}

for values in ci/default-values.yaml ci/all-fields-values.yaml values-dev.yaml; do
  helm template naira "$CHART" --namespace idp-system -f "$CHART/$values" \
    | kubeconform -strict -summary -kubernetes-version "$KUBERNETES_VERSION"
done
