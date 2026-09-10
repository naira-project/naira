# E2E environments

`e2e/components/` holds every piece any E2E scenario might deploy: `catalog`
(always deployed, with a configurable set of plugin sidecars), `keycloak`
(always deployed, for auth), and opt-in standalone components — `litellm`,
`mlflow`, `llamacpp`, `openmetadata`, `postgres`, `ui`, `portal`, `chatbot1`.

Each scenario is a directory directly under `e2e/` (e.g.
[`litellm_chatbot_to_catalog_api/`](litellm_chatbot_to_catalog_api/README.md))
containing:

- `components.env` — which of the shared components/plugins this scenario
  actually deploys (`COMPONENTS=...`, `PLUGINS=...`)
- `seed/` — a Job image that seeds/triggers whatever this scenario's assert
  step needs before it queries the catalog
- `assert/` — the Go tests that verify the scenario, tagged `e2e`
- its own `README.md` with scenario-specific run instructions

`e2e/base/` holds cluster-level prerequisites shared by every scenario:
`kind-config.yaml` and a `ResourceQuota`/`LimitRange` (`quota.yaml`) sized
for the whole component pool — an unused ceiling costs nothing per run, so
it isn't resized per scenario.

`e2e/scripts/create-environment.sh --scenario <name> ...` reads a scenario's
`components.env` and only builds/deploys/waits-on what it lists — see that
script's header comment for the full mechanics. `destroy-environment.sh`
tears down any scenario's environment the same way (it's driven by env-id,
not by scenario).

Adding a new scenario means adding a new directory with those four things —
nothing in `e2e/components/` or the scripts needs to change unless the new
scenario needs a component that doesn't exist in the pool yet.
