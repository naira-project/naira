#!/usr/bin/env bash
# Probes the naira release installed by CI (values-dev.yaml) through its API:
# all enabled plugins are registered, a run of every plugin reaches SUCCEEDED,
# and the UI proxies /v1 to the catalog. Needs port-forward access to the
# cluster, curl, jq, yq and helm.
set -euo pipefail
kubectl -n naira-deps port-forward svc/keycloak 18080:8080 &
kubectl -n idp-system port-forward svc/catalog 18090:8090 &
kubectl -n idp-system port-forward svc/ui 13001:80 &
for _ in $(seq 1 30); do
  curl -sf -o /dev/null localhost:18080/realms/naira && curl -sf -o /dev/null localhost:18090/healthz && break
  sleep 1
done
TOKEN=$(curl -sf -H 'Host: localhost:8080' -d grant_type=password -d client_id=naira-portal \
  -d client_secret=naira-local-dev-secret -d username=testuser -d password=testpass \
  localhost:18080/realms/naira/protocol/openid-connect/token | jq -r .access_token)
AUTH="Authorization: Bearer ${TOKEN}"
expected=$(helm template naira deploy/charts/naira -n idp-system -f deploy/charts/naira/values-dev.yaml \
  | yq 'select(.kind == "ConfigMap" and .metadata.name == "catalog-plugin-config") | .data["plugins.yaml"]' \
  | yq '.plugins | keys | sort | join(",")')
got=$(curl -sf -H "${AUTH}" localhost:18090/v1/plugins | jq -r '[.plugins[].name] | sort | join(",")')
echo "plugins: ${got}"
[ "${got}" = "${expected}" ] || { echo "::error::registered plugins differ from the chart: expected ${expected}"; exit 1; }
# Judge only the operations this run created: scheduled plugin runs
# (fluxcd every minute) keep adding unfinished entries to the list.
# Operations are AIP-151: `done` false is pending or running, `error`
# is set only on failure.
ours=$(curl -sf -X POST -H "${AUTH}" 'localhost:18090/v1/plugins:run' | jq -r '[.operations[].name] | join(",")')
want=$(jq -r 'split(",") | length' <<<"\"${ours}\"")
for _ in $(seq 1 60); do
  report=$(curl -sf -H "${AUTH}" localhost:18090/v1/operations \
    | jq -r --arg ours "${ours}" '[.operations[] | select(.name as $n | ($ours | split(",")) | index($n))]
        | map(.metadata.plugin + "=" + (if .done | not then "RUNNING" elif .error then "FAILED (" + .error.message + ")" else "SUCCEEDED" end))
        | sort | join(" ")')
  case "${report}" in *RUNNING*) sleep 2 ;; *) break ;; esac
done
echo "plugin runs: ${report}"
# Count the successes: a report that matches no failure pattern must not pass by default.
ok=$(grep -o 'SUCCEEDED' <<<"${report}" | wc -l)
if [ "${want}" -eq 0 ] || [ "${ok}" -ne "${want}" ]; then echo "::error::plugin runs did not all succeed (${ok}/${want})"; exit 1; fi
curl -sf -o /dev/null -H "${AUTH}" localhost:13001/v1/plugins || { echo "::error::UI does not proxy /v1 to the catalog"; exit 1; }
# The UI's /prometheus/ proxy sits behind the catalog's token check. Auth is
# decided before the upstream is contacted, so these hold without Prometheus.
status() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
PROM="localhost:13001/prometheus/api/v1/query?query=up"
[ "$(status "${PROM}")" = 401 ] || { echo "::error::Prometheus API reachable without a token"; exit 1; }
[ "$(status -H 'Authorization: Bearer garbage' "${PROM}")" = 401 ] || { echo "::error::Prometheus API accepts an invalid token"; exit 1; }
code=$(status -H "${AUTH}" "${PROM}")
case "${code}" in 401|403) echo "::error::Prometheus API rejects a valid token (${code})"; exit 1 ;; esac
[ "$(status -X POST -H "${AUTH}" localhost:13001/prometheus/-/quit)" = 403 ] || { echo "::error::Prometheus management endpoint not blocked"; exit 1; }
[ "$(status -X POST -H "${AUTH}" localhost:13001/prometheus/api/v1/admin/tsdb/snapshot)" = 403 ] || { echo "::error::Prometheus admin API not blocked"; exit 1; }
echo "prometheus proxy: auth enforced"
