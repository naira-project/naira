# Configure plugins

Each plugin is an entry under `catalog.plugins` in the chart values. One entry creates the plugin's
sidecar in the catalog pod, its line in the catalog's plugin list, its permissions and its config
file. Switching a plugin on or off is one value.

## The plugins

| Plugin | On by default | Needs |
|---|---|---|
| `litellm` | yes | Secret key `LITELLM_API_KEY` |
| `mlflow` | yes | Nothing else |
| `depl-calls-svc` | yes | Read access to namespaces, services and deployments |
| `depl-uses-litellm` | no | Read access to Secrets in all namespaces, and deployments |
| `fluxcd` | yes | Read access to deployments and the Flux kustomizations, helm releases and git repositories |
| `openmetadata` | yes | Secret key `OPENMETADATA_ADMIN_PASSWORD` |
| `tech-radar` | no | A radar file, see [below](#maintain-the-tech-radar) |
| `mcp-servers` | yes | Nothing else |
| `github` | no | Secret key `GITHUB_TOKEN`, and read access to namespaces and deployments |
| `bedrock` | no | Secret keys `BEDROCK_AWS_ACCESS_KEY_ID` and `BEDROCK_AWS_SECRET_ACCESS_KEY` |

The chart creates the permissions for you. `depl-uses-litellm` is off by default because its read
access to Secrets covers every namespace.

## Turn a plugin on

Set `enabled`, give the plugin the address or settings it needs in `env`, and put credentials in
the catalog Secret:

```yaml
catalog:
  plugins:
    github:
      enabled: true
      env:
        GITHUB_ORG: my-org
```

Add the key `GITHUB_TOKEN` to the Secret `catalog-secrets` first. A value in `env` is either a
string or a reference to a Secret key. A string is rendered as a Helm template, so it cannot
contain template syntax of its own, or the render fails.

::: warning One broken plugin stops the catalog
The catalog connects to every enabled plugin when it starts and exits if one is unreachable. Turn
on one plugin at a time.
:::

## Change when a plugin runs

Set `schedule` to a cron expression. Leave it out and the plugin runs only when someone starts it
from the Plugins page.

```yaml
catalog:
  plugins:
    litellm:
      schedule: "*/30 * * * *"
```

## Maintain the tech radar

The `tech-radar` plugin turns a YAML file into the radar. The file is the only source. Pass it as
`config.data`, or as an existing ConfigMap with `config.existingConfigMap`:

```yaml
catalog:
  plugins:
    tech-radar:
      enabled: true
      config:
        data:
          radar.yaml: |
            schema_version: 1            # required; only 1 is accepted
            radar:
              title: Platform Tech Radar # required
              edition: 2026-11           # required; any label
              owner: platform-team       # required
            quadrants:                   # exactly 4, in display order
              - {id: models, name: Models}
              - {id: agentic, name: Agentic Patterns}
              - {id: knowledge, name: Knowledge Techniques}
              - {id: others, name: Others}
            rings:                       # 1 to 6, from the centre outwards
              - {id: adopt, name: Adopt, description: Proven; the default for new work.}
              - {id: hold, name: Hold, description: Do not start new work with this.}
            entries:
              - id: hybrid-retrieval
                name: Hybrid Retrieval
                quadrant: knowledge      # must match a quadrant id
                ring: adopt              # must match a ring id
                moved: in                # in, out or none; default none
                owner: ai-board
                rationale: Combines dense and sparse retrieval.
```

The plugin reads the file on every run. After you change the ConfigMap, the next run shows the new
radar, with no redeployment. A file that does not fit makes the run fail, and the error names the
line and the field. Check **Details** on the Plugins page.
