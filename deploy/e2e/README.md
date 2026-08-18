# E2E Test Environments

Spins up the entire Naira stack — catalog, portal, UI, MLflow, LiteLLM, llama.cpp,
Keycloak, OpenMetadata — into one fresh, isolated Kubernetes namespace, seeds it
with known data, and runs smoke tests against it. Used per-PR in CI; also
runnable locally before pushing.


## Prerequisites

`kind`, `kubectl`, `helm`, `docker`, `envsubst`, `go` — pinned versions in the
repo's `mise.toml`. Use `mise` to pick these up automatically; a `kind` older
than 0.32 will fail with `unknown containerd config version: 4` on Docker
Desktop.

## Run it locally

```bash
# 1. Create the cluster
kind create cluster --config deploy/e2e/kind-config.yaml

# 2. Build, load, deploy, and seed the whole stack into a fresh namespace
./deploy/e2e/scripts/create-environment.sh --pr local --tag localtest \
  --cluster-name naira-idp-e2e
```

This takes ~10-15 minutes cold (OpenMetadata's Helm install is the long
pole). It prints `ENV_ID=<namespace>` on success — you'll need that for the
next steps.

```bash
# 3. Port-forward catalog and keycloak (the API requires an auth token)
NS=<ENV_ID from step 2>
kubectl -n "$NS" port-forward svc/catalog 8090:8090 &
kubectl -n "$NS" port-forward svc/keycloak 8080:8080 &

# 4. Run the tests
CATALOG_URL=http://localhost:8090 KEYCLOAK_URL=http://localhost:8080 \
  go test -tags e2e -v ./tests/e2e/...

# 5. Tear down
kill %1 %2
./deploy/e2e/scripts/destroy-environment.sh --env-id "$NS" --cluster-name naira-idp-e2e
kind delete cluster --name naira-idp-e2e
```

## What's in the namespace

| Component | Notes |
|---|---|
| `catalog` + 6 plugin sidecars | Reads MLflow/OpenMetadata/LiteLLM/K8s into the graph |
| `portal`, `ui` | Frontend |
| `keycloak` | Auth — seeded with a `naira-portal` client and a `testuser`/`testpass` user |
| `mlflow`, `postgres`, `litellm`, `llama-dummy-model` | AI infra, one demo model registered |
| OpenMetadata (Helm: server + MySQL + OpenSearch) | 5 demo tables + lineage seeded |

## Infra capacity needed

Per PR/run, enforced by the namespace's `ResourceQuota`
(`deploy/e2e/quota.yaml`): **~3.5 CPU / ~8.5Gi memory requested**, up to
**11 CPU / 15Gi memory** at burst (limits). OpenMetadata (server + MySQL +
OpenSearch) is over half of that on its own. PVCs declare ~36Gi, but on
`kind` (hostPath-backed `local-path-provisioner`) actual disk used is only
~1Gi — a cloud cluster with real dynamic provisioning would actually
allocate the full 36Gi. See the SPEC's "Resource Cost" section for the
per-component breakdown.

Locally this needs a Docker daemon with at least that much CPU/memory
allocated (Docker Desktop → Settings → Resources). In CI, `ubuntu-latest`
runners (4 vCPU / 16GB / 14GB disk) fit the compute but leave disk tight
once the ~10 built images and pulled dependencies are added — the workflow's
"Free disk space" step exists for that reason.

Everything is fresh per run — no shared state between PRs, so smoke test
assertions can check exact counts (e.g. "exactly 5 tables") rather than
"at least 5".

## Troubleshooting

**`unknown containerd config version: 4`** — your local `kind` predates
0.32.0's support for it (Docker Desktop's containerd image store uses v4).
Upgrade `kind` to match `mise.toml`.

**Pods stuck `Pending` / `Forbidden: exceeded quota`** — the namespace's
`ResourceQuota` (`deploy/e2e/quota.yaml`) requires every container, including
init containers, to declare resources; a `LimitRange` in the same file
defaults any that don't. If a new component's default footprint doesn't fit,
raise the quota there.

**`/v1/*` requests return 401** — the catalog API requires a Bearer token.
Fetch one from Keycloak (see `authToken()` in `tests/e2e/smoke_test.go` or
`fetch_token()` in `deploy/e2e/seed/trigger_catalog_sync.py` for the exact
request) using the seeded `testuser`/`testpass` credentials.

**Environment stuck after a failed run** — `create-environment.sh` doesn't
clean up on failure (only the CI workflow's `if: always()` step does).
Destroy it manually before retrying:
`./deploy/e2e/scripts/destroy-environment.sh --env-id <NS> --cluster-name naira-idp-e2e`.
