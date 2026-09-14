# E2E environments

`deploy/charts/naira-core` and `deploy/charts/components` are the single
source of truth for every piece any E2E scenario might deploy — the same two
Helm charts `deploy/dev`'s Taskfile installs locally (see CONTEXT.md's
"Component" and "Plugin" entries, and `docs/adr/0001-two-flat-charts-with-a-dependency-exception.md`).
`naira-core` covers catalog (always deployed, with a configurable set of
plugins) and ui/portal/mcp-mock; `components` covers keycloak (always
deployed, for auth) and the opt-in third-party pieces — litellm, mlflow,
llamacpp, openmetadata, postgres, vllm, monitoring.

`deploy/environments/e2e/*.values.yaml` is the e2e-wide baseline layered on
top of each chart's defaults: mostly everything off, since a scenario should
only pay for what it actually exercises.

Each scenario is a directory directly under `e2e/` (e.g.
[`litellm_chatbot_to_catalog_api/`](litellm_chatbot_to_catalog_api/README.md))
containing:

- `naira-core.values.yaml` / `components.values.yaml` (either is optional) —
  which components/plugins this scenario turns on, plus any scenario-specific
  config. `naira-core.values.yaml` is itself an envsubst template when it
  needs to reach another in-cluster service by this run's own namespace (see
  its own comment for why — Helm replaces list values wholesale, so a
  plugin's env list is fully restated rather than layered).
- `extras.env` (optional) — space-separated names of scenario-only test
  fixtures that aren't in either chart (e.g. `chatbot1`), applied as plain
  manifests from `e2e/components/<name>.yaml`.
- `seed/` — a Job image that seeds/triggers whatever this scenario's assert
  step needs before it queries the catalog.
- `assert/` — the Go tests that verify the scenario, tagged `e2e`.
- its own `README.md` with scenario-specific run instructions.

`e2e/base/` holds cluster-level prerequisites shared by every scenario:
`kind-config.yaml` and a `ResourceQuota`/`LimitRange` (`quota.yaml`) sized
for the whole component pool — an unused ceiling costs nothing per run, so
it isn't resized per scenario.

`e2e/scripts/create-environment.sh --scenario <name> ...` layers chart
defaults → e2e baseline → scenario values → a small generated runtime
overlay, then installs both charts into one namespace named after the
run's env-id — collapsing every component into that single namespace (unlike
dev's fixed per-component namespaces) is what lets concurrent PR
environments share one cluster without colliding; see that script's header
comment for the full mechanics. `destroy-environment.sh` tears down any
scenario's environment the same way (it's driven by env-id, not by
scenario) — namespace deletion cascades through both Helm releases since
they're installed into it.

Adding a new scenario means adding a new directory with those things —
nothing in the charts or scripts needs to change unless the new scenario
needs a component that doesn't exist in either chart yet.
