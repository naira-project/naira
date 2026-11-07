# Call the catalog API

The portal reads everything through the catalog API. You can call it yourself to script a check or
to feed another tool.

## Authenticate

Every request under `/v1` needs a Keycloak access token in the `Authorization` header. The catalog
accepts a token only if its issuer is the one your Naira is configured with. A real installation
signs users in through the portal and has its own client, so ask whoever runs your Naira for the
catalog's address and for a token.

The examples below read both from your shell:

```sh
CATALOG_URL=<the catalog's address>
TOKEN=<your access token>
```

## Endpoints

All paths below are under `$CATALOG_URL`.

| Method and path | Returns |
|---|---|
| `GET /healthz` | Liveness. The only path without a token. |
| `GET /v1/nodes` | Entries of the catalog |
| `GET /v1/nodes/{kind}/{path}` | One entry. The `kind` is one path segment, the `path` is the rest, and it may contain `/`. |
| `GET /v1/relations` | Relations between entries |
| `GET /v1/kinds` | The kinds that have entries |
| `GET /v1/plugins`, `GET /v1/plugins/{plugin}` | The registered plugins |
| `POST /v1/plugins:run`, `POST /v1/plugins/{plugin}:run` | Starts all plugins, or one |
| `GET /v1/operations`, `GET /v1/operations/{operationId}` | The plugin runs and their results |

## List and filter

```sh
curl -s -H "Authorization: Bearer $TOKEN" $CATALOG_URL/v1/kinds

curl -s -G -H "Authorization: Bearer $TOKEN" $CATALOG_URL/v1/nodes \
  --data-urlencode 'filter=kind="model"'
```

The filter takes one equality, `field="value"`, with the value in double quotes. It does not take
`AND`, `OR` or other operators. The fields you can filter on:

| List | Fields |
|---|---|
| Nodes | `name`, `kind`, `path` |
| Relations | `name`, `kind`, `fromNode`, `toNode` |
| Operations | `plugin` |

Lists return up to 50 entries by default and at most 1000. Set `pageSize` to change it. When more
entries are left, the response has a `nextPageToken`. Pass it as `pageToken` to get the next page.
`totalSize` is the total count.

## Read an entry

An entry has a `kind` and a `path`, and `pluginClaims`: one object per plugin that reports it, with
that plugin's properties.

```json
{
  "kind": "model",
  "path": "clusterA/deepseek",
  "pluginClaims": [
    { "plugin": "mlflow/clusterA", "props": { "release": "2.34", "token_price": "$10" } },
    { "plugin": "litellm", "props": { "token_price": "$5" } }
  ]
}
```

The catalog never merges two plugins' values. See [Glossary](/reference/glossary#claim).

A relation has a `kind`, a `fromNode` and a `toNode`. To see which applications use which models:

```sh
curl -s -G -H "Authorization: Bearer $TOKEN" $CATALOG_URL/v1/relations \
  --data-urlencode 'filter=kind="uses_model"'
```

## Run a plugin

```sh
curl -s -X POST -H "Authorization: Bearer $TOKEN" $CATALOG_URL/v1/plugins/litellm:run

curl -s -G -H "Authorization: Bearer $TOKEN" $CATALOG_URL/v1/operations \
  --data-urlencode 'filter=plugin="litellm"'
```

The first call starts the plugin. The second lists its runs. A run is finished when `done` is true,
and a failed run carries an `error`.
