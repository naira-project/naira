# Call the catalog API

The portal reads everything through the catalog API. You can call it yourself to script a check or
to feed another tool.

## Authenticate

Every request under `/v1` needs a Keycloak access token in the `Authorization` header. The catalog
accepts a token only if its issuer is the one the chart is configured with (`keycloak.issuer`).

In the local setup, fetch a token for the test user. Tilt forwards Keycloak to port 8080:

```sh
TOKEN=$(curl -s http://localhost:8080/realms/naira/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=naira-portal -d client_secret=naira-local-dev-secret \
  -d username=testuser -d password=testpass | jq -r .access_token)
```

::: warning Local credentials
The client secret and the user exist only in the local setup. A real installation signs users in
through the portal and has its own client.
:::

## Endpoints

Tilt forwards the catalog to port 8090. All paths below are under `http://localhost:8090`.

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
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8090/v1/kinds

curl -s -G -H "Authorization: Bearer $TOKEN" http://localhost:8090/v1/nodes \
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
curl -s -G -H "Authorization: Bearer $TOKEN" http://localhost:8090/v1/relations \
  --data-urlencode 'filter=kind="uses_model"'
```

## Run a plugin

```sh
curl -s -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8090/v1/plugins/litellm:run

curl -s -G -H "Authorization: Bearer $TOKEN" http://localhost:8090/v1/operations \
  --data-urlencode 'filter=plugin="litellm"'
```

The first call starts the plugin. The second lists its runs. A run is finished when `done` is true,
and a failed run carries an `error`.
