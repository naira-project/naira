# Glossary

The words Naira uses, in the sense the code and the UI use them.

| Term | Meaning |
|---|---|
| **Node** | One asset in the catalog, identified by its kind and path. |
| **Kind** | The type of a node, such as `model` or `deployment`. One path segment. |
| **Path** | The rest of a node's identity. It may contain `/` and carries the hierarchy. |
| **Relation** | A link between two nodes. It has a kind, such as `uses_model`, a `fromNode` and a `toNode`. |
| **Plugin** | A program that reads one system and reports nodes and relations. |
| **Claim** | The properties one plugin reports for a node, kept under that plugin's name. See [Claim](#claim). |
| **Snapshot** | One complete run of a plugin. It replaces what the same plugin reported before. |
| **Operation** | The record of a plugin run: when it ran, how long it took, the result or the error. |
| **Viewpoint** | A catalog page for one or two kinds, such as the Model Catalog. |
| **Catalog** | The service that stores nodes and relations and serves the API. |
| **Portal** | The OpenMFP portal that hosts the Naira UI and the handbooks. |
| **Handbook** | A static site installed in Naira and opened in the portal's Docs group. |

## Claim

An API response lists a node's claims in `pluginClaims`, one object per plugin with that plugin's
properties. If two plugins report a `token_price` for the same model, the response has both values,
each under its plugin's name. The catalog does not pick one.

## Known kinds

Plugins reuse these kinds. A plugin may add more.

| Kind | What it is |
|---|---|
| `model` | A model |
| `inference_endpoint` | An endpoint that serves a model |
| `dataset` | A dataset |
| `deployment`, `service` | A Kubernetes deployment or service |
| `application` | An application |
| `mcp_server`, `mcp_tool` | An MCP server and a tool it exposes |
| `git_repository`, `owner` | A source repository and who owns it |
| `Kustomization.fluxcd`, `HelmChart.fluxcd`, `GitRepository.fluxcd` | Flux resources |
| `tech_radar`, `tech_radar_entry` | A tech radar and one of its entries |

## Relation kinds

| Kind | Links |
|---|---|
| `uses_model` | An application to a model it uses |
