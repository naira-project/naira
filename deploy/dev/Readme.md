## Deployment Guide

### Prerequisites & Order of Execution

`task platform:deploy` (from the repo root) creates the kind cluster, builds
and loads every locally-built image, and installs the two Helm charts that
are the single source of truth for what runs locally —
[`deploy/charts/naira-core`](../charts/naira-core) (catalog, plugins, ui,
portal, mcp-mock) and [`deploy/charts/components`](../charts/components)
(keycloak, postgres, litellm, llamacpp, mlflow, openmetadata, monitoring) —
with this target's overrides from [`deploy/environments/local`](../environments/local).

To run the steps individually: `task cluster:create`, then
`task naira-core:deploy` and `task components:deploy` (either order — they
don't depend on each other, though `components:deploy` includes keycloak,
which `naira-core`'s catalog/portal need reachable to actually work end to
end).



---

### Troubleshooting

#### Error: Unknown containerd config version

* **Symptom:**
```text
...ERROR: unknown containerd config version: 4 (supported versions: 2 and 3)...
```
* **Cause:** Running an older version of `kind`.
* **Resolution:** Activate [mise](https://mise.jdx.dev/) in your shell to use the correct, supported `kind` version. Refer to the [Prerequisites section in the root README](README.md#prerequisites) for detailed setup instructions.