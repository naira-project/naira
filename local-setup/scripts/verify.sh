#!/usr/bin/env bash
set -eo pipefail

echo "==> Verifying Kubernetes nodes..."
nodes=$(kubectl get nodes --no-headers | wc -l)
if [ "$nodes" -lt 3 ]; then
  echo "Error: Expected 3 nodes, found $nodes"
  exit 1
fi
echo "Nodes OK."

echo "==> Verifying core services..."
kubectl get configmap naira-dev-config >/dev/null 2>&1 || { echo "Error: Missing naira-dev-config"; exit 1; }
echo "Configuration OK."

echo "==> All health checks passed!"
