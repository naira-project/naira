# litellm_chatbot_to_catalog_api

Proves 2 catalog plugins merge into one subgraph: `litellm` (discovers
LiteLLM's models) and `depl_uses_litellm` (discovers which Deployments use
which of those models by scanning for a matching API key). Spins up just
what that needs — `catalog` (with those 2 plugin sidecars), `keycloak`
(auth), `litellm` (seeded with 2 models), and `chatbot1` (a Deployment that
actually calls one of them) — into one fresh, isolated Kubernetes namespace,
then asserts the resulting graph via the catalog API. Used per-PR in CI;
also runnable locally before pushing. See [`e2e/README.md`](../README.md)
for how this scenario's component selection works and how to add another
scenario.

## Prerequisites

`kind`, `kubectl`, `helm`, `go`, `python` — pinned versions in the repo's
`mise.toml`; use `mise` to pick these up automatically. `docker` and
`envsubst` are not mise-managed — install them separately (Docker
Desktop/Engine, and `envsubst` from your OS's `gettext` package). A `kind`
older than 0.32 will fail with `unknown containerd config version: 4` on
Docker Desktop.

## Run it locally

```bash
# 1. Create the cluster
kind create cluster --config e2e/base/kind-config.yaml

# 2. Build, load, deploy, and seed this scenario into a fresh namespace
./e2e/scripts/create-environment.sh --scenario litellm_chatbot_to_catalog_api \
  --env-id local --tag localtest --cluster-name naira-idp-e2e
```

It prints `ENV_ID=<namespace>` on success — you'll need that for the next
steps.

```bash
# 3. Port-forward catalog and keycloak (the API requires an auth token)
NS=<ENV_ID from step 2>
kubectl -n "$NS" port-forward svc/catalog 8090:8090 &
kubectl -n "$NS" port-forward svc/keycloak 8080:8080 &

# 4. Run the tests
CATALOG_URL=http://localhost:8090 KEYCLOAK_URL=http://localhost:8080 \
  go test -tags e2e -v ./e2e/litellm_chatbot_to_catalog_api/assert/...

# 5. Tear down
kill %1 %2
./e2e/scripts/destroy-environment.sh --env-id "$NS" --cluster-name naira-idp-e2e
kind delete cluster --name naira-idp-e2e
```

## What's in the namespace

| Component | Notes |
|---|---|
| `catalog` + 2 plugin sidecars (`litellm`, `depl-uses-litellm`) | Only these 2 — see `components.env` |
| `keycloak` | Auth — seeded with a `naira-portal` client and a `testuser`/`testpass` user |
| `litellm` | Seeded (via static config, no seed script) with exactly 2 models: `openai`, `mistral` |
| `postgres` | `litellm`'s backing store — needed for its virtual-key API (see below), nothing else uses it |
| `chatbot1` | Calls `litellm`'s `openai` model — the Deployment `depl_uses_litellm` discovers |

Nothing else — no MLflow, OpenMetadata, llama.cpp, UI, or portal. They're
available in [`e2e/components/`](../components/) for other scenarios to opt
into, but this one doesn't deploy them.

`seed/provision_chatbot1_key.py` mints `chatbot1` a LiteLLM virtual key
scoped to just the `openai` model (via `litellm`'s `/key/generate`, which
needs `postgres`) and patches it into `chatbot1-secrets` before the catalog
sync runs. Without this, `chatbot1` would authenticate with the shared
master key — valid for *every* model — and `depl_uses_litellm` would
discover an edge to `mistral` too, breaking the exactly-1-relation
assertion below.

## What the tests assert

`assert/assert_test.go`, run with `go test -tags e2e`:

- `TestHealthz` — the catalog API is up.
- `TestLitellmModelsSeeded` — `GET /v1/nodes?filter=kind="model"` returns
  exactly 2 nodes: `litellm1/openai` and `litellm1/mistral`.
- `TestChatbotUsesLitellmModel` — `GET /v1/relations?filter=kind="uses_model"`
  returns exactly 1 relation, from `chatbot1`'s Deployment node to
  `litellm1/openai`.

Since this is the only scenario running in the namespace, these can be exact
counts rather than "at least N" — nothing else is present to add unrelated
nodes or relations.

## Infra capacity needed

Bounded by the namespace's `ResourceQuota` (`e2e/base/quota.yaml`), sized
for the full component pool in `e2e/components/` (not just what this
scenario deploys) — an unused ceiling costs nothing per run, so it isn't
resized per scenario. What this scenario actually uses is small: `catalog` +
2 sidecars, `keycloak`, `litellm`, `chatbot1`, plus the transient `seed` Job.

## Troubleshooting

**`unknown containerd config version: 4`** — your local `kind` predates
0.32.0's support for it (Docker Desktop's containerd image store uses v4).
Upgrade `kind` to match `mise.toml`.

**Pods stuck `Pending` / `Forbidden: exceeded quota`** — the namespace's
`ResourceQuota` (`e2e/base/quota.yaml`) requires every container, including
init containers, to declare resources; a `LimitRange` in the same file
defaults any that don't. If a new component's default footprint doesn't fit,
raise the quota there.

**`/v1/*` requests return 401** — the catalog API requires a Bearer token.
Fetch one from Keycloak (see `authToken()` in `assert/assert_test.go` or
`fetch_token()` in `seed/trigger_catalog_sync.py` for the exact request)
using the seeded `testuser`/`testpass` credentials.

**Environment stuck after a failed run** — `create-environment.sh` doesn't
clean up on failure (only the CI workflow's `if: always()` step does).
Destroy it manually before retrying:
`./e2e/scripts/destroy-environment.sh --env-id <NS> --cluster-name naira-idp-e2e`.
