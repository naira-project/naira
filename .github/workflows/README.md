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

For more information, see: [`e2e/litellm_chatbot_to_catalog_api/README.md`](../../e2e/litellm_chatbot_to_catalog_api/README.md)
(and [`e2e/README.md`](../../e2e/README.md) for the shared component pool
model behind it).
