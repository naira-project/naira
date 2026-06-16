# Test Strategy — Naira Catalog PoC

## 1. Overview

This document defines the test strategy for the Naira Catalog PoC, covering the `catalog/` Go microservice and the `ui-poc/` React/TypeScript frontend. The catalog scrapes MLflow and LiteLLM, stores a node-relation graph in memory, and exposes it via a REST API. Correctness of graph data is the most critical quality attribute for the PoC.

---

## 2. Architecture Under Test

| Component | Tech | Purpose |
|---|---|---|
| `catalog/internal/catalog` | Go | Domain core: MemoryStore, Service, types |
| `catalog/internal/httpapi` | Go / chi | REST router: nodes, relations, plugins |
| `catalog/internal/plugins/mlflow` | Go | Scrapes MLflow registered models + dataset lineage |
| `catalog/internal/plugins/litellm` | Go | Scrapes LiteLLM models + Kubernetes app identities |
| `catalog/pluginapi` | Go | Public plugin contract (types, constants) |
| `ui-poc/src/lib/catalogApi.ts` | TypeScript | HTTP client and URL utilities |
| `ui-poc/src/hooks` | React | Data-fetching hooks (useCatalogGraph, useModels) |
| `ui-poc/src/pages` | React | UI pages (Dashboard, ModelRegistries, CatalogGraph, etc.) |

---

## 3. Test Pyramid

```
         ┌────────────────┐
         │   E2E / Smoke  │  (future: Kind cluster + deployed services)
         ├────────────────┤
         │  HTTP Layer    │  httptest.Server — router + service + store, no external deps
         ├────────────────┤
         │   Unit Tests   │  isolated functions, mocked HTTP / Kubernetes
         └────────────────┘
```

- **Unit tests** are the primary vehicle for the PoC. They run in milliseconds, need no infrastructure, and cover validation logic, plugin parsing, and utility functions.
- **HTTP layer tests** (already partially present in `router_test.go`) wire the router, service, and store together in-process using `httptest.NewRecorder` and `httptest.NewServer`.
- **E2E tests** are deferred until the PoC matures.

---

## 4. Tooling

| Layer | Tool | Notes |
|---|---|---|
| Go unit / HTTP | `testing`, `github.com/stretchr/testify` | already a dependency |
| Go HTTP mocking | `net/http/httptest` | stdlib, no install needed |
| Go race detection | `go test -race ./...` | run on every CI PR |
| TypeScript unit | `jest`, `@testing-library/react`, `@testing-library/jest-dom` | already installed |
| TypeScript fetch mocking | `jest.spyOn(global, 'fetch')` | works without extra packages |
| TypeScript fetch mocking (recommended) | `msw` | add as devDependency for more realistic network mocking |

---

## 5. Baseline Coverage (Existing Tests)

| File | What is covered |
|---|---|
| `catalog/internal/catalog/service_test.go` | `ListNodes`, `GetNode` (found + not-found), `ListRelations`, `RunAllPlugins` happy path |
| `catalog/internal/httpapi/router_test.go` | All endpoints happy path; plugin error in `Results` field |
| `catalog/internal/httpapi/query_test.go` | `parseFilterSyntax`, `listOptionsFromRequest`, `paginate` |
| `ui-poc/src/App.test.tsx` | Placeholder only — tests stale "learn react" text, not real behavior |

**Not covered at all:** MLflow plugin, LiteLLM plugin, MemoryStore validation logic, HTTP error paths (404, 400), UI hooks and pages.

---

## 6. Coverage Gaps & Priorities

### High Priority

| Area | Gap |
|---|---|
| `MemoryStore.UpsertGraph` | Validation of invalid node IDs and referential integrity of relations |
| MLflow plugin | Zero tests — core ingestion path, dataset lineage |
| LiteLLM plugin | Zero tests — model + app identity graph construction |

### Medium Priority

| Area | Gap |
|---|---|
| `Service.RunPlugin` | Empty/whitespace name, unknown plugin name |
| HTTP API error paths | 404 for missing node, 400 for bad filter/page token |
| UI `catalogApi.ts` | URL construction helpers, `parseNodeName`, `fetchNodes`/`fetchRelations` |
| UI hooks | `useCatalogGraph`, `useModels` loading/error states |

### Lower Priority

| Area | Gap |
|---|---|
| MemoryStore concurrency | Run with `-race` flag |
| `RunAllPlugins` ordering | Deterministic alphabetical run order |
| UI pages | Smoke render tests |
| `pluginapi` schema | Constants are trivial; coverage via integration with other tests |

---

## 7. Test Cases

### 7.1 MemoryStore (`catalog/internal/catalog`)

> Test file: `catalog/internal/catalog/memory_store_test.go`

| ID | Title | Expected |
|---|---|---|
| MS-001 | `UpsertGraph` — node with empty `Kind` | Returns `ErrInvalidIngestion` |
| MS-002 | `UpsertGraph` — node with empty `Path` | Returns `ErrInvalidIngestion` |
| MS-003 | `UpsertGraph` — node `Kind` containing `/` | Returns `ErrInvalidIngestion` (reserved separator) |
| MS-004 | `UpsertGraph` — relation with empty `Kind` | Returns `ErrInvalidIngestion` |
| MS-005 | `UpsertGraph` — relation `Kind` containing `/` | Returns `ErrInvalidIngestion` |
| MS-006 | `UpsertGraph` — relation `From` references node absent from payload | Returns `ErrInvalidIngestion` |
| MS-007 | `UpsertGraph` — relation `To` references node absent from payload | Returns `ErrInvalidIngestion` |
| MS-008 | `UpsertGraph` — empty nodes and relations | Succeeds, returns 0, 0 counts |
| MS-009 | `UpsertGraph` — upsert existing node | Node properties are fully replaced (not merged) |
| MS-010 | `UpsertGraph` — upsert existing relation | Relation properties are fully replaced |
| MS-011 | `UpsertGraph` — returns correct upserted node and relation counts | Counts equal len(nodes) and len(relations) |
| MS-012 | `ListNodes` on empty store | Returns empty slice (not nil) |
| MS-013 | `ListRelations` on empty store | Returns empty slice (not nil) |

### 7.2 Service (`catalog/internal/catalog`)

> Test file: `catalog/internal/catalog/service_test.go` (extend)

| ID | Title | Expected |
|---|---|---|
| SVC-001 | `RunPlugin` — empty plugin name | Returns `ErrInvalidPluginName` |
| SVC-002 | `RunPlugin` — whitespace-only name | Returns `ErrInvalidPluginName` (after normalization) |
| SVC-003 | `RunPlugin` — unknown plugin name | Returns `ErrPluginNotFound` |
| SVC-004 | `RunPlugin` — name is case-insensitive | "MLflow" matches plugin registered as "mlflow" |
| SVC-005 | `NewService` — nil plugin in variadic list | Nil plugins are silently skipped |
| SVC-006 | `RunAllPlugins` — mixed success and failure | Both results appear; failed plugin has non-empty `Error` field |
| SVC-007 | `RunAllPlugins` — plugin run order | Results are sorted alphabetically by plugin name |

### 7.3 HTTP API (`catalog/internal/httpapi`)

> Test file: `catalog/internal/httpapi/router_test.go` (extend)

| ID | Title | Expected |
|---|---|---|
| API-001 | `GET /healthz` | `200 {"status":"ok"}` |
| API-002 | `GET /v1/nodes` — no filter | Returns all nodes, `totalSize` matches count |
| API-003 | `GET /v1/nodes?filter=kind="model"` | Returns only model nodes |
| API-004 | `GET /v1/nodes?filter=unsupportedField="x"` | `400` with error body |
| API-005 | `GET /v1/nodes?pageSize=1` | Returns first node + `nextPageToken` |
| API-006 | `GET /v1/nodes?pageToken=<next>` | Returns next page; last page has no token |
| API-007 | `GET /v1/nodes?pageToken=<relations-token>` | `400` — wrong scope token |
| API-008 | `GET /v1/nodes/{kind}/{path}` — exists | `200` with node body |
| API-009 | `GET /v1/nodes/{kind}/{path}` — missing | `404` with error body |
| API-010 | `GET /v1/relations` — no filter | Returns all relations |
| API-011 | `GET /v1/relations?filter=kind="uses_model"` | Returns only `uses_model` relations |
| API-012 | `GET /v1/relations?filter=unsupportedField="x"` | `400` |
| API-013 | `POST /v1/plugins:run` — all plugins succeed | `202`, results with empty `Error` fields |
| API-014 | `POST /v1/plugins:run` — plugin fails | `202`, result has non-empty `Error` |
| API-015 | Unknown path | `404` |

### 7.4 MLflow Plugin (`catalog/internal/plugins/mlflow`)

> Test file: `catalog/internal/plugins/mlflow/plugin_test.go` (**new**)
> Mock strategy: `httptest.NewServer` serving fixed JSON responses.

| ID | Title | Expected |
|---|---|---|
| MLF-001 | `Collect` — models with no `run_id` | Returns model nodes only, no relations, no error |
| MLF-002 | `Collect` — model with run containing dataset | Returns model + dataset nodes + `trained_on` relation |
| MLF-003 | `Collect` — two models share the same dataset | Dataset node appears once; two `trained_on` relations |
| MLF-004 | `Collect` — dataset name is empty/whitespace | Dataset node and relation are skipped |
| MLF-005 | `Collect` — run fetch fails for one model | Partial result (model node present) + error returned |
| MLF-006 | `Collect` — `/registered-models/search` returns non-2xx | Error returned, wraps status string |
| MLF-007 | `Collect` — `/registered-models/search` returns malformed JSON | Error with "decoding" in message |
| MLF-008 | `Collect` — `BearerToken` set in config | `Authorization: Bearer <token>` header sent |
| MLF-009 | `Collect` — `BearerToken` empty | No `Authorization` header sent |
| MLF-010 | `Collect` — `/runs/get` returns non-2xx | Treated as run fetch failure (MLF-005 path) |
| MLF-011 | `Collect` — empty registered models list | No nodes, no relations, no error |
| MLF-012 | Model node `description` property | Populated from MLflow model description field |

### 7.5 LiteLLM Plugin (`catalog/internal/plugins/litellm`)

> Test file: `catalog/internal/plugins/litellm/plugin_test.go` (**new**)
> Mock strategy: `httptest.NewServer` + stub `AppIdentityProvider`.

| ID | Title | Expected |
|---|---|---|
| LLM-001 | `Collect` — no `AppIdentityProvider` (nil) | Returns model nodes only, no app nodes, no relations |
| LLM-002 | `Collect` — models + app with virtual key | Returns model + app nodes + `uses_model` relations |
| LLM-003 | `Collect` — app with empty `LiteLLMVirtualKey` | App node is created but no `fetchAllowedModels` call and no relations |
| LLM-004 | `Collect` — model in key info not in `/v1/models` | Ghost model node created with `discovered_via=key_info` property |
| LLM-005 | `Collect` — same app-model pair encountered twice | Relation is deduplicated (appears once) |
| LLM-006 | `Collect` — duplicate model nodes | `dedupeNodes` ensures each model node appears once |
| LLM-007 | `Collect` — `ListAppIdentities` returns error | Error is logged and swallowed; only model nodes returned |
| LLM-008 | `Collect` — `/v1/models` returns non-2xx | Error returned |
| LLM-009 | `Collect` — `/v1/models` returns malformed JSON | Error returned |
| LLM-010 | `Collect` — `/key/info` HTTP error | Error returned (not swallowed) |
| LLM-011 | `applicationNodeID` — app with namespace | Path is `litellm/{namespace}/{name}` |
| LLM-012 | `applicationNodeID` — app with empty namespace | Path is `litellm/{name}` |

### 7.6 UI — `catalogApi.ts` (`ui-poc/src/lib`)

> Test file: `ui-poc/src/lib/catalogApi.test.ts` (**new**)

| ID | Title | Expected |
|---|---|---|
| UI-API-001 | `parseNodeName` — valid name `nodes/model/mlflow/fraud-detector` | `{ kind: "model", path: "mlflow/fraud-detector" }` |
| UI-API-002 | `parseNodeName` — missing `nodes/` prefix | Returns `null` |
| UI-API-003 | `parseNodeName` — fewer than 3 segments | Returns `null` |
| UI-API-004 | `encodeCatalogPath` — path with spaces | Each segment percent-encoded, `/` preserved |
| UI-API-005 | `buildEqualityFilter` | Returns `field="value"` string |
| UI-API-006 | `buildNodeUrl` — simple path | `/v1/nodes/{kind}/{path}` |
| UI-API-007 | `buildListNodesUrl` — no options | `/v1/nodes` |
| UI-API-008 | `buildListNodesUrl` — with filter, pageSize, pageToken | All params serialised as query string |
| UI-API-009 | `fetchNodes` — OK response | Returns `nodes` array |
| UI-API-010 | `fetchNodes` — non-OK response | Throws an error |
| UI-API-011 | `fetchNodes` — response missing `nodes` field | Returns empty array |
| UI-API-012 | `fetchRelations` — OK response | Returns `relations` array |
| UI-API-013 | `fetchNode` — OK response | Returns single `NodeResource` |

### 7.7 UI — Hooks (`ui-poc/src/hooks`)

> Test file: `ui-poc/src/hooks/useModels.test.ts`, `useCatalogGraph.test.ts` (**new**)

| ID | Title | Expected |
|---|---|---|
| UI-HOOK-001 | `useModels` — initial state | `loading: true`, `models: []` |
| UI-HOOK-002 | `useModels` — successful fetch | `loading: false`, `models` populated |
| UI-HOOK-003 | `useModels` — fetch error | `loading: false`, `error` set |
| UI-HOOK-004 | `useCatalogGraph` — successful fetch | Returns nodes and edges derived from API response |
| UI-HOOK-005 | `useCatalogGraph` — empty catalog | Returns empty nodes and edges |
| UI-HOOK-006 | `useCatalogGraph` — fetch error | Error state set |

### 7.8 UI — Pages (smoke tests)

> Render tests verifying pages mount without crashing and show key content.

| ID | Title | Expected |
|---|---|---|
| UI-PAGE-001 | `Dashboard` renders | No crash; heading or nav visible |
| UI-PAGE-002 | `ModelRegistries` with model list | Model names rendered |
| UI-PAGE-003 | `ModelRegistries` with empty list | Empty-state message shown |
| UI-PAGE-004 | `CatalogGraph` renders graph | Node elements in DOM |
| UI-PAGE-005 | `ModelSpec` — valid model node | Properties rendered |
| UI-PAGE-006 | `ModelSpec` — invalid node name | Error or fallback shown |
| UI-PAGE-007 | `DatasetRegistries` renders | No crash |
| UI-PAGE-008 | `Stats` renders | No crash |

---

## 8. Running Tests

### Go

```bash
cd catalog

# All tests
go test ./...

# With race detection (required for PRs)
go test -race ./...

# With coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### TypeScript / React

```bash
cd ui-poc

# Run all tests (watch mode)
npm test

# Single run with coverage (CI)
CI=true npm test -- --coverage
```

---

## 9. CI Recommendations

| Check | Command | Gate |
|---|---|---|
| Go tests + race | `cd catalog && go test -race ./...` | Block merge on failure |
| Go coverage | `go test -coverprofile=... ./...` | Warn below 70% line coverage |
| UI tests | `cd ui-poc && CI=true npm test` | Block merge on failure |
| Go vet + lint | `go vet ./...` + `golangci-lint run` | Block merge on failure |
| TypeScript types | `npx tsc --noEmit` | Block merge on failure |

---

## 10. Non-Functional Testing Notes

- **Concurrency:** `MemoryStore` uses `sync.RWMutex`. Run `go test -race ./...` to surface data races.
- **Performance:** For the PoC, pagination is the only load-control mechanism. If the catalog grows beyond ~10k nodes, the in-memory list scan in `ListNodes` / `ListRelations` will be the bottleneck. Add a benchmark (`func BenchmarkListNodes`) when this becomes relevant.
- **Security:** The catalog HTTP API has no authentication. Ensure it is not exposed outside the cluster in production. LiteLLM and MLflow tokens are passed via config; they must not be logged (currently safe) or returned in API responses (currently safe).
