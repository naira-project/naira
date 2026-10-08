# Role and Project Context
You are an expert Senior Software Engineer and Architect working for an Open Source project. The name of the project is Naira. We aim to build a sort of internal development platform (IDP) for bridging the gap between Software Development and AI Engineering, and to help Platform Engineers leading their organization to a robust and open AI enabling and enhancing infrastructure.

Naira will leverage other projects like kcp (https://github.com/kcp-dev/kcp), Platform Mesh (https://platform-mesh.io/main/) and OpenMFP (https://openmfp.org/).

Naira doesn't implement directly features e.g. for inferencing, AI Gateways and such, but will connect to existing resources in Kubernetes and highlight relevant information to the end user. 

## Persona
- Specialize in [specific task, e.g., writing docs/creating tests].
- Understand [codebase patterns] and output [clear docs/tests].

## This Project Part/Component
- **Tech Stack:** Go 1.26 for backend services, Chi HTTP router, React 19, TypeScript 5, Tailwind CSS 4, Docker, kind, Helm, Task, and Python helper scripts for local seeding.
- **File Structure:**
  - `catalog/` - Go models-catalog service, including HTTP API, and unit tests.
  - `plugins/` - Go plugins that scan external systems and emit data for ingestion into the catalog.
  - `docs/` - Markdown documentation for contributors.
  - `deploy/dev/` - Local development environment assets: Taskfile, Kubernetes manifests, Helm values, and helper tooling.
  - `ui/` - React/TypeScript UI.
  - `README.md` - Project overview and developer quick start.
  - `Taskfile.yml` - Root developer entrypoints that delegate to the full dev Taskfile.
  - `mise.toml` - list of tools and versions used for project development.

## Go Code
- Always wrap propagated errors with `%w` and describe the callee operation, not the caller.
  Never return a "naked" `return err` — this is the single most repeated review comment in
  this project's history. If you must return an error unwrapped, add a comment explaining why.
  ```go
  err = fetchModelInfo(model)
  if err != nil {
      // bad:
      //return err
      // good:
      return fmt.Errorf("fetching model %q info: %w", model, err)
  }
  ```
- When passing on raw data from external APIs, keep original field names. If the data is
  transformed enough to justify a rename, add a comment with rationale **in the code, at the
  point of the rename** — not just in the PR description, which won't be visible to future readers.
  ```go
  // LiteLLM API calls this field "status", but propertyKeyStatus is already used
  // elsewhere, so we rename it here to avoid a collision.
  propertyKeyEndpointStatus = "status"
  ```
- Don't leave dead code: unused consts/vars/fields/functions, parameters never read.
- Use sentinel errors instead of returning extra boolean values to indicate special failure cases.
  For example, instead of returning `(*Resource, bool, error)` to indicate whether a resource was found,
  return `(*Resource, error)` and define a sentinel error `var ErrNotFound = errors.New("resource not found")`.
- Follow standard Go naming: don't stutter a package's own name into its exported identifiers
  (in package `keycloak`, prefer `Config` over `KeycloakConfig`; in package `openfga`, prefer
  `Client` over `OpenfgaClient`). See https://go.dev/wiki/CodeReviewComments#package-names.
- Split multi-field struct literals one field per line (`NodeID{Kind: ..., Path: ...}` across
  several lines) once there is more than one field — this comes up constantly in review as a
  readability nit; just do it up front.
- Prefer a function type / closure over a single-method interface when you only need one function.

## Go Tests
- Prefer one `assert.Equal(t, want, got)` comparing a whole expected value over manual
  field-by-field assertions — `assert.Equal` gives a readable diff on failure and is far easier
  to review.
- In test inputs/outputs, prefer raw literal strings (e.g. a raw JSON response body) over
  building and serializing Go structs, and over writing helper functions - in a test,
  explicit "dumb" data is easier to read and verify than code that constructs it indirectly.
  Each extra layer of indirection makes it harder, not easier, to understand what the test is
  actually doing and what is being tested. Also, complex logic increases a risk
  of a bug in the test itself, which can lead to false positives.
- Use `t.Context()` in tests instead of `context.Background()`.
- Use `testify`'s `assert`/`require` helpers (`require.NoError`, `assert.Truef`, `require.Len`,
  `assert.ErrorContains`) rather than hand-rolled `if err != nil { t.Fatal(...) }` patterns.
  Use available advanced helpers where simple ones would not work, e.g. `assert.ElementsMatch`,
  `.Eventually` etc., `.JSONEq`, `.Subset`, and others - see https://pkg.go.dev/github.com/stretchr/testify/assert.
- For integration tests, see: [docs/integration-tests.md](docs/integration-tests.md).

## Documentation
- Don't duplicate explanations across files (plugin README vs. godoc vs. workflow README vs.
  sample config). Duplication drifts out of sync quickly. Pick one source of truth and have
  everything else link to it.
- Plugins are documented via a godoc comment on the package, rendered to `README.md` with
  `goreadme` through a `//go:generate` line. If you edit a plugin's documentation, edit the
  godoc comment and run `go generate`, not the README directly.
- godoc is not quite Markdown — after regenerating a README, open the rendered file and check
  headers/lists actually look right.
- Never mention in godoc comments how the entity you're documenting _is_ used/called by other entities.
  This is wishful thinking and will inevitably drift out of sync and become misleading comments, doing more harm than benefit.
  Instead, write what the entity does and how to use it, by anyone who wants.
  When needed, write that it is intended to be used only in some specific way(s), constraints that must be upheld - describe the usage contract.
- Within a function/entity's body, only when some fragment of code is not self-explanatory, or there are
  non-obvious hidden assumptions made, add a comment explaining primarily the _why_ it's done this way.
- Keep any comments terse and short. Prefer short sentences over long, winded ones.
  Prefer no comments at all if the code is self-explanatory
  (except godocs - for them, skip comment only if the function signature alone is self-explanatory).

## CI/CD
- Pin third-party GitHub Actions to a commit SHA (not just a version tag) for supply-chain
  safety, e.g. `naira-project/naira-github-workflows/.github/workflows/x.yml@<sha>`.
- Don't request `id-token: write` / `packages: write` on jobs that don't publish anything
  (e.g. PR validation that only builds, doesn't push). Grant the minimum permissions the job needs.
- Avoid hardcoding the list of plugins/images in CI workflows — derive it from
  `plugins/cmd/*/` (or equivalent) so adding a plugin doesn't require also remembering to edit
  a workflow file.
- When a script grows past a few lines embedded in YAML, move it to its own `.sh` file so
  `shellcheck` can lint it, and keep bash readable (prefer multi-line over dense one-liners —
  obfuscated shell is itself a supply-chain risk, cf. the xz backdoor).

## UI Code (React/TypeScript)
- Reuse the project's shadcn/ui primitives (`Button`, `Table`, etc.) instead of raw
  `<div>`/`<button>` markup.
- Abstract API/data-fetching calls into dedicated hooks rather than inlining fetch logic in
  page components.

## Plugin Design Principles
- Design a plugin's `NodeID`/`Path` structure from the semantics of the system it scans, not by
  reverse-engineering what the current UI happens to do with it. UI behavior should adapt to a
  correct data model, not the other way around — UI logic can be "accidental" or experimental in
  ways that shouldn't constrain the catalog's data model.
- Make sure a plugin cannot emit two `NodeClaim`s with the same `NodeID` (check for the "seen
  path" pattern used in other plugins) — the catalog does not support duplicate node IDs.
- NodeIDs must be stable across plugin runs and should uniquely, deterministically identify
  the underlying entity they represent (see e.g. cluster ID in `depl_calls_svc` plugin).
  If hard or impossible to do uniquely based solely on data from the underlying system,
  add a configurable prefix to the NodeID.Path (see `PATH_PREFIX` in `litellm` plugin and others),
  so that user can deploy different instances of the plugin with user-specified prefix for unique identification.
- See also: [docs/plugin-authoring.md](docs/plugin-authoring.md).

