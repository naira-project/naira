# tech_radar plugin

tech\_radar renders an admin-managed technology radar configuration file into the catalog: a read-only governance view showing which technologies — models, agentic patterns, knowledge techniques — the organization has decided to adopt, trial, assess, or hold, next to the assets the catalogs already describe. Decisions are managed in a single YAML configuration file; the portal renders it and offers no write path, so governance stays auditable through whatever review process already owns that file.

The radar deliberately carries no links into the catalog: it expresses the organization's target vision, not what is currently deployed or in use.

## How it works

The plugin reads the config file (default /etc/naira/techradar/radar.yaml, overridable via TECH\_RADAR\_CONFIG\_PATH) on every collect, so changes take effect on the next sync without a redeployment.

A valid file becomes one tech\_radar node (radar metadata plus the quadrant and ring taxonomies) and one tech\_radar\_entry node per entry.

An invalid file fails the collect with errors naming the offending line, field, and reason. A failed collect never touches the store, so the previous radar stays visible ("last known good"). The errors appear in the plugin log and in the sync operation's error message on the Plugins page.

Radar data is served by the standard catalog API - no dedicated endpoints:

	GET /v1/nodes?filter=kind="tech_radar"
	GET /v1/nodes?filter=kind="tech_radar_entry"

## Loading and updating tech radar information

The dev environment mounts the config from the tech-radar-config ConfigMap (deploy/dev/stacks/core/infra/kubernetes/catalog.yaml). The ConfigMap is mounted as a directory - not via subPath - so edits propagate into the running pod (the kubelet syncs it within about a minute).

 1. Edit the config: kubectl -n idp-system edit configmap tech-radar-config.
 2. Wait for the kubelet to sync the mounted file (~1 min).
 3. Trigger a sync from the portal's Plugins page, or POST /v1/tech-radar:run (or POST /v1/plugins:run).
 4. Open Tech Radar in the portal: the overview shows the radar circle, ring definitions, edition label, and moved count; each quadrant links to a filterable table with every entry's rationale.

If the sync fails, the operation on the Plugins page shows the validation errors; the previously synced radar keeps rendering until a valid config syncs successfully.

## Configuration schema

schema\_version is required and must be 1.

  - schema\_version (required) - schema version; only 1 is accepted.
  - radar.id (optional) - radar identifier, defaults to "default". Becomes the node path prefix (\<radar.id>/\<entry.id>).
  - radar.title (required) - display title of the radar.
  - radar.edition (required) - free-form edition label, e.g. 2026-09.
  - radar.owner (required) - owning team or board.
  - quadrants (required) - exactly 4 quadrants, in display order. Each has id and name.
  - rings (required) - 1-6 rings, ordered innermost to outermost. Each has id, name, and an optional description.
  - entries (required) - the radar entries (may be empty).
  - entries\[].id (required) - stable identifier; with radar.id it forms the node path.
  - entries\[].name (required) - display name.
  - entries\[].quadrant (required) - must match a declared quadrant id.
  - entries\[].ring (required) - must match a declared ring id.
  - entries\[].moved (optional) - in, out, or none (default). Movement since the previous edition.
  - entries\[].owner (required) - team accountable for the decision.
  - entries\[].rationale (required) - why the entry sits in its ring. Truncated at 2000 characters.

All id fields must match ^\[a-z0-9]\[a-z0-9\_-]\*$ and be at most 100 characters; ids become node paths, so oversized ids are rejected rather than clipped. Unknown fields are rejected, so typos surface as validation errors instead of being silently ignored. Entry order in the file is canonical: it drives the numbering on the radar chart and in the quadrant summaries.

Free-form text is clipped rather than rejected, so an oversized value never blocks a sync: short labels (titles, names, owners, the edition) are capped at 200 runes and long-form text (an entry's rationale, a ring's description) at 2000 runes, each with a trailing ellipsis and a warning in the plugin log.

## Annotated example

See sample\_config.yaml in this directory for a ready-to-use annotated example. The plugin's unit tests parse that file, so it is guaranteed to stay valid as the schema evolves.

## Out of scope

In-portal editing, historic editions, enforcement of radar decisions, multiple radars per instance, and Git-based config sync are all out of scope for now - the mounted-file mechanism composes with any external GitOps tooling.

## Environment Variables

  - TECH\_RADAR\_CONFIG\_PATH (optional) - path to the radar YAML file; defaults to /etc/naira/techradar/radar.yaml.
