# naira

Installs the catalog with its plugin sidecars, plus `ui` and `portal`. It does
not install Keycloak, LiteLLM, MLflow, OpenMetadata or MCP servers — this
chart only needs their addresses (`keycloak.*` and each plugin's `env`); see
the test-dependencies chart.

## Install

Bare defaults do not render: the chart needs a Secret source for the catalog
and for the portal (`templates/validate.yaml`).

Minimal:

```
helm install naira deploy/charts/naira -n idp-system --create-namespace \
  -f deploy/charts/naira/ci/default-values.yaml
```

plus Secrets supplied by the operator: `catalog-secrets` (keys
`LITELLM_API_KEY`, `OPENMETADATA_ADMIN_PASSWORD`), `portal-oidc` (key
`client-secret`), `postgres-admin` (key `postgres-password`) and `catalog-db`
(keys `username`, `password`).

Run `helm dependency build deploy/charts/naira` first in a checkout; the
PostgreSQL subchart is not vendored.

Released charts are also published to ghcr.io as OCI artifacts, so no checkout
is needed:

```
helm install naira oci://ghcr.io/naira-project/charts/naira --version <release> \
  -n idp-system --create-namespace -f my-values.yaml
```

For kind, add `-f deploy/charts/naira/values-dev.yaml` instead — it creates
development Secrets from values.

## Constraints

- One release per namespace: namespaced objects have fixed names (`catalog`,
  `ui`, `portal`, `catalog-plugin-config`, `plugin-<name>-config`,
  `catalog-secrets`, `portal-oidc`, `postgres`, `postgres-admin`, `db-init`,
  `<name>-db`).
  Cluster-scoped RBAC is prefixed with the release name, so several releases
  per cluster are fine as long as their release names differ.
- Migrating from the former `deploy/dev/stacks`: delete the
  `catalog`/`ui`/`portal` Deployments in `idp-system` first. `helm install` cannot take over those objects: the Deployment
  selectors differ and are immutable.
- `keycloak.issuer`, `authUrl`, `baseDomain` and `portal.uiBaseUrl`
  default to `localhost` (kind, port-forward). Anywhere else, set them to the
  addresses the browser uses; the catalog accepts only tokens whose `iss`
  equals `issuer`.
- On ghcr, `naira-plugin-depl-uses-litellm`, `naira-plugin-openmetadata`,
  `naira-plugin-mcp-servers` and `naira-plugin-tech-radar` have no `0.1.0`
  tag; only `dev-publish.yml` builds them (branch-name tags). A registry
  install at the chart's default tag needs those plugins disabled, a dev tag
  via `image.tag`, or a release that includes them.

## PostgreSQL

The `cloudpirates/postgres` subchart (official `postgres` image) runs one server,
StatefulSet and Service `postgres`. `postgresql.enabled=false` plus
`database.host` uses an external server instead; set `database.sslmode` to
`require` or stricter there (the subchart serves no TLS, so the default is
`prefer`). The admin may be a superuser or a role with `CREATEROLE` and
`CREATEDB`, as on managed services.

Each enabled entry in `database.databases` gets a role and a database of the
same name, owned by that role (also when the database already existed), with
`CONNECT` revoked from `PUBLIC`. The `db-init` Job (`files/db-init.sh`) creates
them if missing and sets the password from `<name>-db`. It is idempotent, runs
on existing volumes, and reruns when its spec, script
or `database` values change; delete the Job to rerun it by hand, e.g. after
changing a password in an existing Secret. A server that never answers fails
the Job after `database.init.waitRetries` or `activeDeadlineSeconds`. Defaults: `catalog`. `values-dev.yaml`
adds `litellm` and `openmetadata`, used by LiteLLM and OpenMetadata from the
test-dependencies chart.

## Plugins

One entry in `catalog.plugins` drives the sidecar, its RBAC and its config
file. Plugin `env` string values are rendered with `tpl`, so a literal `{{`
in a value fails the render.

| Plugin | Port | Enabled by default | Needs |
|---|---|---|---|
| litellm | 50051 | yes | Secret key `LITELLM_API_KEY` |
| mlflow | 50052 | yes | — |
| depl-calls-svc | 50053 | yes | RBAC: namespaces, services, deployments |
| depl-uses-litellm | 50054 | no | RBAC: namespaces, secrets (cluster-wide), deployments |
| fluxcd | 50055 | yes | RBAC: deployments, kustomizations, helmreleases, gitrepositories |
| openmetadata | 50056 | yes | Secret key `OPENMETADATA_ADMIN_PASSWORD` |
| tech-radar | 50057 | no | Config file (`config.data` or `config.existingConfigMap`) |
| mcp-servers | 50058 | yes | — |
| github | 50059 | no | Secret key `GITHUB_TOKEN`; RBAC: namespaces, deployments |

## Scheduling and security

Same keys on `catalog`, `ui` and `portal` unless noted; unset values render
nothing. `imagePullSecrets` is top-level and applies to all three.

| Key | Notes |
|---|---|
| `podAnnotations`, `priorityClassName`, `nodeSelector`, `tolerations`, `affinity` | Passed through. |
| `podSecurityContext`, `securityContext` | See below. |
| `startupProbe` | Timing only; the check is the readiness path. None on the plugin sidecars: they listen on loopback only, which kubelet probes (pod IP) cannot reach. |
| `catalog.networkPolicy` | Ingress to the catalog from the `ui` pods, plus `extraIngress` rules. Egress is not restricted. |

`securityContext`: the catalog and every plugin sidecar run as uid 65532 with
no privilege escalation, all capabilities dropped and the `RuntimeDefault`
seccomp profile, which meets Pod Security `restricted`. The `ui` (nginx) and
`portal` (node) images have no `USER` and run as root, so they get only
`allowPrivilegeEscalation: false` and seccomp; a `restricted` namespace needs
non-root images for both first.

Not provided: ServiceMonitor/PodMonitor. No workload exposes a metrics
endpoint yet.

## Guards

`templates/validate.yaml` (and, for a missing `image.repository` on the
catalog, a plugin, the UI or the portal, `naira.image` in `_helpers.tpl` —
templates render in file order, before `validate.yaml` can catch it) reject: `catalog.secret`/`portal.oidc.secret` with both
`existingSecret` and `create` set; `catalog.secret.create` true with
`catalog.secret.data` empty; `portal.oidc.secret.create` true with
`portal.oidc.secret.value` empty; two enabled plugins on the same port; an enabled plugin whose name makes
`plugin-<name>` longer than 63 characters; a plugin
`config` block with neither `existingConfigMap` nor `data`; a plugin `env`
entry that is a Secret reference without a `key`; an enabled plugin without
`image.repository`; either Secret (catalog or portal) missing a source
when something reads it; `database.host` set together with the subchart, or
missing without it; `postgresql.auth.existingSecret` differing from the
`database.admin` Secret; a database name other than lowercase letters, digits and inner hyphens starting with a letter, or one of
`postgres`, `template0`, `template1` and `database.admin.username`; and a
`database.admin` or `database.databases.<name>` Secret without exactly one
source.

## Tests

`deploy/charts/naira/tests/render-test.sh`
