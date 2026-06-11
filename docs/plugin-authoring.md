# Plugin authoring

Plugins return graph data through `pluginapi.IngestionRequest`.

## Node IDs

- `kind` must not contain `/`
- `path` may contain `/`

Why: node reads use the route `/v1/nodes/{kind}/*`, so `kind` is one path segment and `path` is the remaining opaque path.

## Current conventions

- Reuse known kinds from `pluginapi/schema.go` when they fit.
- Keep `kind` stable. Changing it changes node identity.
- Use `path` for hierarchy.

## Relations

- `kind` must not contain `/`
- Relation endpoints currently identify links by `kind`, `from`, and `to`.
- `from` and `to` nodes must exist in the same ingestion request.

## Minimal example

```go
return pluginapi.IngestionRequest{
	Nodes: []pluginapi.NodeClaim{
		{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: "fraud-detector",
			},
		},
	},
}, nil
```

## Snapshots & Stale Data

Each execution of a plugin is treated as a single, atomic snapshot. New data is ingested, and all previous data from this plugin not present in the current batch is removed.

## Multi-plugin properties

Multiple plugins can independently report the **same node or relation** (identified by identical `NodeID` or relation `kind`/`from`/`to` triple). The catalog merges their data rather than overwriting it.

**How it works:**

- Each plugin's properties are stored under that plugin's name as a namespace — the catalog never mixes or overwrites another plugin's data.
- The API returns `props` as an ordered list of `{ plugin, entries }` objects, one per contributing plugin:

  ```json
  {
    "kind": "model",
    "path": "clusterA/deepseek",
    "props": [
      { "plugin": "mlflow/clusterA", "entries": { "release": "2.34", "token_price": "$10" } },
      { "plugin": "litellm",         "entries": { "token_price": "$5" } }
    ]
  }
  ```