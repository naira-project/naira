#!/usr/bin/env bash
set -eo pipefail

echo "==> Checking required tools..."
for tool in kind kubectl; do
  if ! command -v "$tool" &> /dev/null; then
    echo "Error: $tool is required."
    exit 1
  fi
done

echo "==> Creating kind cluster (3 nodes)..."
if ! kind get clusters | grep -q "^naira-local$"; then
  # Prefer podman if available
  if command -v podman &> /dev/null; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
  fi
  kind create cluster --config local-setup/kind/config.yaml
else
  echo "Cluster naira-local already exists."
fi

echo "==> Provisioning base ConfigMaps and Secrets..."
kubectl apply -k local-setup/kustomize/base

echo "==> Setup complete! Run 'make verify' to check health."
