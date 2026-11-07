# Install with Helm

The `naira` chart installs the catalog with its plugin sidecars, the UI and the portal. It also
runs PostgreSQL for the catalog, unless you point it at your own server.

It does **not** install Keycloak, LiteLLM, MLflow, OpenMetadata or MCP servers. Those come from your
platform. The chart only needs their addresses.

This page is the short path. The chart's own
[README](https://github.com/naira-project/naira/blob/main/deploy/charts/naira/README.md) lists every
guard and constraint, and [`values.yaml`](https://github.com/naira-project/naira/blob/main/deploy/charts/naira/values.yaml)
documents every value.

::: warning Alpha
Naira is pre-1.0. Values and defaults can change between releases. Read the release notes before
you upgrade.
:::

## What you supply

The chart does not render without a Secret source for the catalog, the portal and the database.
Create these Secrets in the release's namespace:

| Secret | Keys |
|---|---|
| `catalog-secrets` | `LITELLM_API_KEY`, `OPENMETADATA_ADMIN_PASSWORD` |
| `portal-oidc` | `client-secret`, the OIDC client secret of the portal in Keycloak |
| `postgres-admin` | `postgres-password` |
| `catalog-db` | `username`, `password` |

A plugin that reads a credential, such as `github`, adds its own key to `catalog-secrets`. See
[Configure plugins](/operate/configure-plugins).

## Install

In a checkout of the repository, fetch the PostgreSQL subchart once, then install:

```sh
helm dependency build deploy/charts/naira
helm install naira deploy/charts/naira -n idp-system --create-namespace \
  -f deploy/charts/naira/ci/default-values.yaml
```

The file `ci/default-values.yaml` only names the Secrets above. Without a checkout, install a
released chart from ghcr.io:

```sh
helm install naira oci://ghcr.io/naira-project/charts/naira --version <release> \
  -n idp-system --create-namespace -f my-values.yaml
```

## Set the addresses the browser uses

The defaults point at `localhost`, which fits a port-forward and nothing else. Set these to the
addresses your users' browsers reach:

| Value | What it is |
|---|---|
| `keycloak.issuer` | The issuer of the tokens. The catalog accepts only tokens whose `iss` equals it. |
| `keycloak.authUrl` | The Keycloak login address |
| `keycloak.baseDomain` | The domain Keycloak is on |
| `portal.uiBaseUrl` | The address of the UI |

`keycloak.baseUrl` is different: it is the address the pods use inside the cluster.

## Good to know

- Use one release per namespace. The chart gives its objects fixed names.
- To use your own PostgreSQL, set `postgresql.enabled=false` and `database.host`. Set
  `database.sslmode` to `require` or stricter.
- Plugins are on or off per entry in `catalog.plugins`. See [Configure plugins](/operate/configure-plugins).
