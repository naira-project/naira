#!/usr/bin/env bash
# Bring up the RFC-009 PoC cluster with a local OCI registry.
set -euo pipefail
CLUSTER=naira-poc
REG_NAME=naira-poc-registry
REG_PORT=5001

if ! docker inspect -f '{{.State.Running}}' "$REG_NAME" >/dev/null 2>&1; then
  docker run -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" \
    --network bridge --name "$REG_NAME" registry:2 >/dev/null
  echo "registry started on localhost:${REG_PORT}"
else
  echo "registry already running"
fi

if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  kind create cluster --name "$CLUSTER" --config "$(dirname "$0")/kind-config.yaml"
else
  echo "cluster $CLUSTER already exists"
fi

# Point the node's containerd at the registry.
# Two host entries, not one: `localhost:5001` is how a laptop pushes, and
# `naira-poc-registry:5000` is how ArgoCD (and therefore the kubelet) pulls.
# Both must resolve to the same plain-HTTP registry or images fail with
# ErrImagePull only on the GitOps path.
for node in $(kind get nodes --name "$CLUSTER"); do
  for host in "${REG_NAME}:5000" "localhost:${REG_PORT}"; do
    docker exec "$node" mkdir -p "/etc/containerd/certs.d/${host}"
    docker exec "$node" sh -c "printf '[host.\"http://${REG_NAME}:5000\"]\n  capabilities = [\"pull\", \"resolve\"]\n' > /etc/containerd/certs.d/${host}/hosts.toml"
  done
done

docker network connect kind "$REG_NAME" 2>/dev/null || true

kubectl --context "kind-${CLUSTER}" apply -f - <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:5001"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
YAML
echo "done. context: kind-${CLUSTER}"
