# Run plugins

The catalog starts empty. Plugins fill it: each one reads another system, such as LiteLLM or
MLflow, and reports what it finds. Until a plugin has run, its viewpoints show a prompt to run it
instead of data.

## Run them from the portal

1. Open **Plugins** in the sidebar. The page is called Plugins & Ingestion.
2. Click **Run All Plugins**, or **Run** in one row to run a single plugin.
3. Watch the **Status** column until the plugin has run.
4. Open a viewpoint, for example the Model Catalog, to see the result.

The table has these columns:

| Column | Meaning |
|---|---|
| Plugin | The plugin's name |
| Last run | How long ago the latest run was, or Never |
| Duration | How long that run took |
| Status | Not run yet, running, succeeded, or **Failed** with a **Details** link to the error |
| Result | How many nodes and relations the run reported |
| Schedule | The cron schedule, if the plugin has one |
| Action | **Run** starts that plugin now. It is disabled while the plugin runs. |

::: tip
In the local setup some plugins can fail because the system they read is not part of the stack.
That is expected. Open **Details** to see what the plugin could not reach.
:::

## Schedules

A plugin with a `schedule` runs on its own, for example `litellm` on `0 0 * * *`, once a day. A
plugin without one runs only when you start it. The schedule is part of the chart values, see
[Configure plugins](/operate/configure-plugins).

## What a run does to the data

Each run is a complete snapshot. When a plugin finishes, the catalog keeps what that run reported
and removes whatever the same plugin reported earlier and did not report now. Another plugin's data
is not touched. So if a model disappears from LiteLLM, it disappears from the catalog on the next
run of the `litellm` plugin.

## Run them from the API

The same two actions exist as API calls, and the operations list shows the results. See
[Call the catalog API](/guide/call-the-catalog-api#run-a-plugin).
