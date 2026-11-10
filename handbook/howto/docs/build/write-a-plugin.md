# Write a plugin

A plugin is a Go program that reads one system and returns what it finds as nodes and relations.
The catalog stores them and the portal shows them. For a kind that a viewpoint already lists, you
write no UI code.

This page is the outline. The repository's
[plugin authoring guide](https://github.com/naira-project/naira/blob/main/docs/plugin-authoring.md)
has the rules in full and a minimal example. Read it before you write code.

## What a plugin returns

A plugin returns a `pluginapi.CollectResponse` with two lists.

| List | Entry | Identity |
|---|---|---|
| Nodes | An asset, for example a model or a deployment | `kind` and `path` |
| Relations | A link between two nodes | `kind`, `from` and `to` |

The rules that matter most:

- **`kind` is one word.** It must not contain `/`. Reuse a known kind from
  `plugins/pkg/pluginapi/schema.go` when one fits, and keep it stable: changing it changes the
  identity of every node.
- **`path` carries the hierarchy.** It may contain `/`. Make it stable across runs and unique for the
  thing it names.
- **Both ends of a relation must be in the same response.**
- **Never emit the same node twice.** The catalog does not support duplicate node ids.

## Make the path unique per installation

Two installations of the same plugin, for example two LiteLLM servers, must not produce the same
paths. Take a configurable prefix as an environment variable and put it first in every path. The
`litellm` plugin does this with `PATH_PREFIX`.

## What the catalog does with it

Each run is a snapshot of that plugin. The catalog replaces what the plugin reported before with
what it reported now, so a node that is gone from the source goes from the catalog.

Several plugins can report the same node. The catalog keeps each plugin's properties under that
plugin's name, in `pluginClaims`, and never mixes them. See [Glossary](/reference/glossary#claim).

## Ship it

1. Put the plugin under `plugins/cmd/<name>`, with a Dockerfile. In the directory name, `_`
   stands for `-` in the plugin's name: `depl-calls-svc` lives in `depl_calls_svc`. The image is
   named `naira-plugin-<name>`.
2. Add an entry `catalog.plugins.<name>` to the chart values with the image, a port that no other
   plugin uses, and an optional `schedule` and `env`. The plugin then runs as a sidecar in the
   catalog pod.
3. Say in the plugin's godoc what it reads and which environment variables it takes. The README is
   generated from the godoc, so change the comment and run `go generate`, not the README.

If you want the nodes on a catalog page, check that their kind is listed for a viewpoint. See
[Find your way around](/guide/find-your-way-around#viewpoints).
