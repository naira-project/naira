# CI Workflows

This repository uses [naira-github-workflows](https://github.com/naira-project/naira-github-workflows) for centralized CI logic.

---

## Workflow Summary

| Workflow | Trigger | Primary Goal |
| --- | --- | --- |
| **PR Validation** | PRs/Pushes to `main` | Test code and verify container builds. |
| **Release** | Version tags (`v*`) | Build and publish images to `ghcr.io`. |

---

## 1. PR Validation (`pr-validation.yml`)

Ensures code quality and buildability. Gatekeeper: `All Checks Passed`.

* **`go-test`**: Runs tests in `catalog/` (Go 1.26, race detector enabled).
* **`container-build-catalog` / `ui**`: Multi-arch build checks (`linux/amd64`, `linux/arm64`)—push disabled.
* **`all-checks`**: Required status check; aggregates results from above jobs.

---

## 2. Release (`release-please.yml`)
 refer to naira-project/naira/.github/workflows/release.md


---
## Test E2E locally

See [`e2e/README.md`](../../e2e/README.md) for the full model (a shared
component pool under `e2e/components/`, selected per scenario) and
[`e2e/litellm_chatbot_to_catalog_api/README.md`](../../e2e/litellm_chatbot_to_catalog_api/README.md)
for this scenario's exact run steps:

```bash
# 1. Create the cluster
kind create cluster --config e2e/base/kind-config.yaml

# 2. Build, load, deploy, and seed this scenario into a fresh namespace
./e2e/scripts/create-environment.sh --scenario litellm_chatbot_to_catalog_api \
  --env-id local --tag localtest --cluster-name naira-idp-e2e
# → prints ENV_ID=<namespace> on success, note it for step 3

# 3. Port-forward catalog + keycloak (API needs a Bearer token from keycloak)
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
