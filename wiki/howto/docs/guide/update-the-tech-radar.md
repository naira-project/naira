# Update the tech radar

The tech radar is one YAML file. To change the radar, edit the file, apply it, and run the
`tech-radar` plugin. You cannot edit the radar in the portal. What the page shows is described in
[Read the tech radar](/guide/read-the-tech-radar).

## Where the file lives

The file is `radar.yaml` in a ConfigMap that the catalog pod mounts at `/etc/naira/techradar`. The
plugin is off by default. Whoever runs your Naira turns it on and gives the Helm chart the file as
`catalog.plugins.tech-radar.config.data`, or as the name of a ConfigMap in
`catalog.plugins.tech-radar.config.existingConfigMap`. Ask them where your radar is kept.

## The file

```yaml
schema_version: 1
radar:
  id: naira
  title: Naira Tech Radar
  edition: 2026-11
  owner: platform-team
quadrants:
  - {id: models, name: Models}
  - {id: agentic, name: Agentic Patterns}
  - {id: knowledge, name: Knowledge Techniques}
  - {id: others, name: Others}
rings:
  - {id: adopt, name: Adopt, description: Proven; the default for new work.}
  - {id: trial, name: Trial, description: Worth pursuing on projects that can absorb risk.}
  - {id: assess, name: Assess, description: Explore to understand the impact.}
  - {id: hold, name: Hold, description: Do not start new work with this.}
entries:
  - id: claude-sonnet
    name: Claude Sonnet
    quadrant: models
    ring: adopt
    moved: in
    owner: ml-platform
    rationale: Default general-purpose model.
    catalog_refs:
      - match: claude-sonnet
        kinds: [model, inference_endpoint]
  - id: naive-rag
    name: Naive RAG
    quadrant: knowledge
    ring: hold
    owner: ai-board
    rationale: Superseded by hybrid retrieval.
```

| Field | Required | Meaning |
|---|---|---|
| `schema_version` | yes | Always `1`. |
| `radar.id` | no | Names the radar in the catalog. Defaults to `default`. |
| `radar.title`, `radar.edition`, `radar.owner` | yes | The page title, a free edition label such as `2026-11`, and who owns the radar. |
| `quadrants` | yes | Exactly four, each with an `id` and a `name`. The order is the display order. |
| `rings` | yes | One to six, from the centre outwards, each with an `id`, a `name` and an optional `description`. |
| `entries` | yes | The technologies. Write `entries: []` for a radar without any. Leaving the key out is an error. |

Each entry has an `id`, a `name`, a `quadrant`, a `ring`, an `owner` and a `rationale`, all
required. `quadrant` and `ring` must be the `id` of a quadrant and a ring from the same file.
`moved` is optional and is `in`, `out` or `none`, which is the default.

An `id` uses lowercase letters, digits, `-` and `_`, starts with a letter or digit, has at most 100
characters, and is unique within its list. A field the file format does not know is an error, so a
typo does not pass unnoticed.

## Make a change

**Add an entry.** Add a block under `entries` with a new `id`.

**Move an entry.** Change its `ring` and set `moved`: `in` for a move towards adoption, `out` for a
move away from it. The file keeps no history, so the marker is yours to maintain. When you publish a
new edition, set `moved` back to `none` for entries that did not move in it, and change
`radar.edition`.

**Remove an entry.** Delete its block, or move it to the outermost ring and say why in the
`rationale`. A run keeps only what the file lists, so a deleted entry is gone after the next run.

**Link an entry to the catalog.** `catalog_refs` fills the **Catalog matches** box of the entry. A
catalog entry matches a ref when its kind is one of `kinds`, its path contains `match` (case does
not matter), and, if the ref lists `plugins`, one of those plugins reported it. `kinds` is required.
`match` and `plugins` are optional, but a ref sets at least one of them.

## Apply it

1. Change the file where it is kept, with Helm or in the ConfigMap.
2. Wait for the new file to reach the catalog pod. A change to `config.data` restarts the catalog
   pod. A change to a ConfigMap you manage yourself restarts nothing, and Kubernetes refreshes the
   mounted file within a minute or two.
3. Run the plugin. Open **Plugins** and click **Run** in the `tech-radar` row. The plugin has no
   schedule by default. [Call the catalog API](/guide/call-the-catalog-api#run-a-plugin) shows the
   same run as an API call.
4. Reload the **Tech Radar** page and check the edition and the entries.

::: warning The catalog keeps its data in memory
After the catalog pod restarts, the catalog is empty until the plugins run again, the radar
included. This includes the restart that a change to `config.data` causes.
:::

Keep the file in Git and let your pipeline apply it, so that every change to the radar is reviewed.

## When a run fails

A file that does not fit makes the run fail, and the previous radar stays on the page. On the
Plugins page the **Status** shows **Failed**. **Details** names the plugin and the file, then lists
the problems, each on a line of its own with the line in the file and the field. For example, after
the plugin and file name:

```text
validating config: line 18: entries[0] (id "claude-sonnet"): field "ring" references undeclared ring "adpot"
parsing YAML: in radar config: decoding node: in entry: checking fields: line 31: unknown field "ring_id"
validating config: line 1: quadrants: exactly 4 quadrants are required, got 3
```

For an entry, the line is the one where its block starts. Fix what the message names and run the
plugin again.

The plugin clips text that is too long instead of failing: names, titles, owners and the edition at
200 characters, and a `rationale` or ring `description` at 2000, each with a trailing `…`. It never
clips an `id` or a `catalog_refs` `match`, because they are matched and not displayed. Those fail the
run when they are too long.

## What it does not do

The plugin reads one file, so Naira shows one radar. It keeps no earlier editions, and nothing
checks whether teams follow a ring.
