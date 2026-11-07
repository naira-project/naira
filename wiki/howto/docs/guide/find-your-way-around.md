# Find your way around

The portal has a sidebar. Each entry opens a page of the Naira UI inside the portal.

## The sidebar

| Entry | What it shows |
|---|---|
| Overview | A card for every catalog viewpoint, with a search box |
| Plugins | The plugins, their last run and status, and a button to run them |
| Catalogs | Model, MCP, Dataset, Inference Endpoints and Software Catalog |
| Explore | The [tech radar](/guide/read-the-tech-radar) |
| Docs | The wikis installed in this Naira, like this one. The group is absent while there are none. |

## Viewpoints

A viewpoint is a catalog page for one kind of asset. The Overview lists them. Each one shows the
entries of one or two kinds and names the plugins that fill it.

| Viewpoint | Shows | Filled by |
|---|---|---|
| Model Catalog | `model` | `litellm`, `mlflow`, `bedrock` |
| MCP Catalog | `mcp_server` and the `mcp_tool` entries of each | `mcp-servers`, `litellm` |
| Dataset Catalog | `dataset` | `openmetadata` |
| Inference Endpoints Catalog | `inference_endpoint` | `litellm`, `bedrock` |
| Software Catalog | `deployment` and `service` | `depl-calls-svc`, `depl-uses-litellm`, `fluxcd` |

A viewpoint with nothing in it yet tells you which plugins to run.

## Where to start

- **rAIner**, who builds with models, starts at the Model Catalog and the Inference Endpoints
  Catalog.
- **Abdel**, who consumes AI in an application, starts at the Model Catalog and the MCP Catalog to
  see what he can use, and opens an entry's graph to see which applications already use it.
- **Peter**, who runs the platform, starts at Plugins, to see what data is coming in and what
  failed.

## The UI only reads

You cannot edit a catalog entry in the portal. The data comes from plugins that read other systems,
so you change it by changing those systems and running the plugins again. See
[Run plugins](/guide/run-plugins).
