# Plugin authoring

Plugins return graph data through `pluginapi.CollectResponse`.

## Node IDs

- `kind` must not contain `/`
- `path` may contain `/`

Why: node reads use the route `/v1/nodes/{kind}/*`, so `kind` is one path segment and `path` is the remaining opaque path.

## Current conventions

- Reuse known kinds from `pluginapi/schema.go` when they fit.
- Keep `kind` stable. Changing it changes node identity.
- Use `path` for source-specific hierarchy such as `mlflow/model-a` or `litellm/team/app`.

## Relations

- `kind` must not contain `/`
- Relation endpoints currently identify links by `kind`, `from`, and `to`.
- `from` and `to` nodes must exist in the same collect response.

## Minimal example

```go
return pluginapi.CollectResponse{
	Nodes: []pluginapi.NodeClaim{
		{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: "mlflow/fraud-detector",
			},
		},
	},
}, nil
```

## Snapshots & Stale Data

Each execution of a plugin is treated as a single, atomic snapshot. New data is ingested, and all previous data from this plugin not present in the current batch is removed.