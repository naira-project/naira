# CI Workflows

This document describes the GitHub Actions CI/CD workflows used in this repository.
All workflows call reusable jobs from [`naira-project/naira-github-workflows`](https://github.com/naira-project/naira-github-workflows).

---

## Overview

| Workflow | File | Trigger |
|---|---|---|
| PR Validation | `.github/workflows/pr-validation.yml` | PRs to `main`, pushes to `main` |
| Release | `.github/workflows/release.yml` | Version tags (`v*`) |

---

## PR Validation

**File:** `.github/workflows/pr-validation.yml`

Runs on every pull request targeting `main` and on every push to `main` (i.e. after a PR is merged).
Concurrent runs on the same branch/PR are cancelled automatically.

### Jobs

#### `go-test` — Test · Catalog

Runs the catalog service test suite.

| Input | Value |
|---|---|
| Go version | `1.26` |
| Working directory | `catalog/` |
| Race detector | enabled |
| Coverage threshold | 0 (informational only — revisit once a baseline is established) |

Uploads a coverage report as a build artifact (`go-coverage-<sha>`, retained 7 days).

#### `container-build-catalog` — Container Build · Catalog

Builds the catalog container image without pushing it. Used to catch `Dockerfile` breakages on PRs before they reach `main`.

| Input | Value |
|---|---|
| Image name | `naira-catalog` |
| Dockerfile | `catalog/Dockerfile` |
| Build context | `catalog/` |
| Platforms | `linux/amd64`, `linux/arm64` |
| Push | `false` |

#### `container-build-ui` — Container Build · UI

Builds the UI container image without pushing it.

| Input | Value |
|---|---|
| Image name | `naira-ui` |
| Dockerfile | `ui-poc/Dockerfile` |
| Build context | `ui-poc/` |
| Platforms | `linux/amd64`, `linux/arm64` |
| Push | `false` |

#### `all-checks` — Gate

Depends on all three jobs above. Always runs (`if: always()`), fails if any upstream job result is not `success` (covers failure, cancellation, and unexpected skips).

Configure this job name (`All Checks Passed`) as the **only** required status check in branch protection — it acts as a single gate so you never need to update branch protection when jobs are added or renamed.

---

## Release

**File:** `.github/workflows/release.yml`

Triggered by a version tag push (e.g. `git tag v1.2.0 && git push --tags`).
Builds both container images for `linux/amd64` and `linux/arm64` and pushes them to `ghcr.io`.

### Jobs

#### `container-publish-catalog` — Publish · Catalog

Builds and pushes the catalog image to `ghcr.io/naira-project/naira-catalog`.

| Input | Value |
|---|---|
| Image name | `naira-catalog` |
| Dockerfile | `catalog/Dockerfile` |
| Build context | `catalog/` |
| Platforms | `linux/amd64`, `linux/arm64` |
| Push | `true` |

#### `container-publish-ui` — Publish · UI

Builds and pushes the UI image to `ghcr.io/naira-project/naira-ui`.

| Input | Value |
|---|---|
| Image name | `naira-ui` |
| Dockerfile | `ui-poc/Dockerfile` |
| Build context | `ui-poc/` |
| Platforms | `linux/amd64`, `linux/arm64` |
| Push | `true` |

#### `all-published` — Gate

Depends on both publish jobs. Always runs (`if: always()`), fails if either job result is not `success`. Provides a single status check that surfaces the overall release outcome.

### Image Tags

Both images are tagged automatically by the shared workflow:

| Tag pattern | Example | When applied |
|---|---|---|
| Full semver | `1.2.3` | On `v1.2.3` tag |
| Minor semver | `1.2` | On `v1.2.3` tag |
| Major semver | `1` | On `v1.2.3` tag |
| `latest` | `latest` | On `v*` tag pushed from the default branch |
| Branch name | `main` | On branch push |
| PR number | `pr-42` | On pull request |
| Short SHA | `sha-abc1234` | Always |

---

## Repository Setup

### Required permissions

Go to **Settings → Actions → General → Workflow permissions** and select:
- **Read and write permissions**

This grants the auto-provisioned `GITHUB_TOKEN` the `packages: write` scope needed to push images to `ghcr.io`. The "Allow GitHub Actions to create and approve pull requests" setting is unrelated to image publishing and is not required.

### Branch protection

Add a branch protection rule for `main` with:
- Require pull requests before merging
- Require status checks: add `All Checks Passed` as the only required check
- Require branches to be up to date before merging

### Secrets

`GITHUB_TOKEN` is provisioned automatically by GitHub — no manual configuration needed.

---

## Shared Workflows Reference

| Reusable workflow | Purpose |
|---|---|
| `reusable-go-test.yml` | `go test` with race detector and coverage reporting |
| `reusable-container-build.yml` | Multi-arch Docker build and optional GHCR push |
