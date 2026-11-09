# Browse a catalog

Each viewpoint lists the entries of its kinds. This page shows how to narrow the list, open an
entry and follow how it connects to others.

## Narrow the list

1. Open a viewpoint from the sidebar, for example **Model Catalog**.
2. If the viewpoint has more than one kind, pick one with the kind selector. The Software Catalog
   has `deployment` and `service`.
3. Pick a plugin tab. **All** shows every entry. A tab named after a plugin shows only the entries
   that plugin reported. The tabs list the plugins that reported at least one entry of the kind.

An entry that two plugins report appears under both tabs. That is how you see which plugins know
about the same model.

## Open an entry

Click a row. The page of the entry has two tabs.

| Tab | Shows |
|---|---|
| Graph | The entry and its relations. This is the tab that opens first. |
| Properties | The entry's properties |

### Follow relations in the graph

The graph draws the entry with the entries it is related to. Each line is labelled with the kind of
the relation, such as `uses_model`, which links an application to a model it uses.

- Click an entry in the graph to see its details in the panel next to it.
- Use **Set as root** on an entry to draw the graph around that one instead.
- The panel marks the entry you opened as the root node and the others as discovered nodes.

This is how you answer a question like "which applications use this model?" without leaving the
portal. It needs the `depl-uses-litellm` plugin to have run.

### Read the properties

Properties are grouped by plugin. When several plugins report the same entry, the catalog keeps
each plugin's values under that plugin's name and does not merge them. So you can see that MLflow
and LiteLLM report different values for the same field, instead of one hiding the other.

## When something is missing

- A viewpoint that has never been filled shows a prompt with a link to **Plugins**. Run the plugins
  it names.
- A viewpoint whose plugins have run but found nothing shows an empty state.
- An inference endpoint appears for a model only after the model has received traffic within the
  metrics lookback window. A model with no traffic shows none.
