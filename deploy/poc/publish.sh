#!/usr/bin/env bash
# RFC-009 §3 — dual publish.
#
#   1. helm push          -> the OCI chart ArgoCD consumes (tag-pinned)
#   2. ocm component      -> digest-pinned, signable, transferable
#
# The resource list is generated from the chart's own values.yaml, so the
# component descriptor cannot drift from what the chart actually deploys.
#
# PoC-only: the local registry is plain HTTP, so references carry an http://
# scheme and helm gets --plain-http. Against ghcr neither is used.
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
CHART="$ROOT/deploy/charts/naira"
REG=${REG:-localhost:5001}
SCHEME=${SCHEME:-http://}
VERSION=$(yq '.version' "$CHART/Chart.yaml")
WORK=$(mktemp -d); trap 'rm -rf "$WORK"' EXIT

ACCEPT='application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json'
digest() { curl -sI -H "Accept: ${ACCEPT}" "http://${REG}/v2/$1/manifests/$2" \
           | rg -i '^docker-content-digest' | tr -d '\r' | awk '{print $2}'; }

echo "── 1. helm package + push"
helm package "$CHART" -d "$WORK" >/dev/null
helm push "$WORK/naira-${VERSION}.tgz" "oci://${REG}/charts" --plain-http 2>&1 | rg -o '(Pushed|Digest).*'
CHART_DIGEST=$(digest "charts/naira" "$VERSION")

echo "── 2. component constructor (generated from values.yaml, digest-pinned)"
{
  printf 'components:\n- name: github.com/naira-project/naira\n  version: "%s"\n  provider:\n    name: naira-project\n  resources:\n' "$VERSION"
  printf '    - name: naira-chart\n      type: helmChart\n      version: "%s"\n      relation: local\n      access:\n        type: ociArtifact\n        imageReference: %s%s/charts/naira@%s\n' \
    "$VERSION" "$SCHEME" "$REG" "$CHART_DIGEST"
  yq -r '[.catalog.image.repository, .ui.image.repository, .portal.image.repository]
         + [.catalog.plugins[].image.repository] | .[]' "$CHART/values.yaml" |
  while read -r repo; do
    d=$(digest "$repo" "$VERSION")
    [ -n "$d" ] || { echo "no digest for ${repo}:${VERSION} — is it pushed?" >&2; exit 1; }
    printf '    - name: %s\n      type: ociImage\n      version: "%s"\n      relation: local\n      access:\n        type: ociArtifact\n        imageReference: %s%s/%s@%s\n' \
      "$repo" "$VERSION" "$SCHEME" "$REG" "$repo" "$d"
  done
} > "$WORK/cc.yaml"
cp "$WORK/cc.yaml" ./component-constructor.generated.yaml
echo "   $(grep -c '    - name:' "$WORK/cc.yaml") resources, all digest-pinned"

echo "── 3. ocm add component-version -> CTF"
ocm add component-version --repository "ctf::${WORK}/ctf" --constructor "$WORK/cc.yaml" 2>&1 | tail -3

echo "── 4. ocm transfer CTF -> ${REG}/ocm"
ocm transfer component-version \
  "ctf::${WORK}/ctf//github.com/naira-project/naira:${VERSION}" \
  "oci::http://${REG}/ocm" 2>&1 | tail -3

echo "── 5. verify from the registry"
ocm get component-version "oci::http://${REG}/ocm//github.com/naira-project/naira:${VERSION}" -o yaml 2>/dev/null \
  | yq -r '.. | select(type == "!!map" and has("resources")) | .resources[] | "   " + .name + "  " + .type' 2>/dev/null | sort -u | head -20
