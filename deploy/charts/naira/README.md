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
  `<name>-db`, `wiki-<id>`, `wikis`, `wiki-gateway`, `wiki-ingress`,
  `wiki-gateway-egress`).
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
adds `langfuse`, `litellm` and `openmetadata`, used by Langfuse, LiteLLM and
OpenMetadata from the test-dependencies chart.

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
| bedrock | 50060 | no | Secret keys `BEDROCK_AWS_ACCESS_KEY_ID`, `BEDROCK_AWS_SECRET_ACCESS_KEY`; no RBAC |

## Wikis

Each enabled entry in `wikis` installs a Deployment and a Service `wiki-<id>`
from a digest-pinned image and adds the wiki to `wikis.json` in ConfigMap
`wikis`. The fields are documented in `values.yaml`; the image contract and a
starter are in [`wiki/starter/README.md`](../../../wiki/starter/README.md).

- A listed wiki is installed unless it sets `enabled: false`. Disabling or
  removing it removes its Deployment, Service and `wikis.json` entry and
  nothing else. Upgrading is a new digest.
- `wikis.json` lists the enabled wikis, the primary one first, then by `order`,
  then by `id`. Each entry has `id`, `label`, `url`, `primary` and `order`, and
  `owner` and `source` when set. The ConfigMap is always rendered, as `[]` when
  no wiki is enabled. The `url` is `<scheme>://<id>.<domain>[:port]/` from
  `wikiGateway`, with the port left out when it is the scheme's default.
- The domain needs wildcard DNS and TLS, and the wiki address has to be on the
  same site as the portal, or the browser drops the session cookie (see the
  starter README).
- Wiki pods run non-root with a read-only root filesystem, a small `/tmp` and no
  Kubernetes API token. They do not use the catalog's ServiceAccount.
  `wikiDefaults` sets replicas, resources and the scheduling fields.
- `wikiGateway.networkPolicy.enabled` renders `wiki-ingress` (wiki pods accept
  traffic from the pods named `wiki-gateway` only) and `wiki-gateway-egress`
  (those pods may reach DNS, the wiki pods and the Keycloak port of
  `keycloak.baseUrl`). Keycloak and DNS are admitted on any address, because a
  policy cannot select a host name.
- A wiki image needs a digest and a registry from `wikiGateway.allowedRegistries`,
  so an image built into kind cannot be listed and `values-dev.yaml` lists no wiki.

The portal does not read `wikis.json` yet, so no wiki appears in the sidebar.

### Gateway

While at least one wiki is enabled, the chart renders one Envoy as Deployment,
Service and ConfigMap `wiki-gateway`. It listens on 8080 over plain HTTP: expose
the Service on `wikiGateway.port` and end TLS in front of it, with a wildcard
certificate for `*.<wikiGateway.domain>`. The admin interface listens on
loopback only.

- A virtual host and a cluster per enabled wiki. A host that is no wiki's
  address gets a 404.
- Every request needs a Keycloak access token. Envoy checks it against
  `keycloak.issuer` and the keys at `keycloak.baseUrl`, cached for 10 minutes
  (an https Keycloak needs a certificate the system CAs trust). The token is read
  from the `naira_wiki` cookie, never from `Authorization` or the query.
  `/_naira/bootstrap.html` and `/_naira/assets/` are open, so a signed-out frame
  can sign in.
- The bootstrap page posts the token to `/_naira/session` in the header
  `x-naira-token`. The answer sets `naira_wiki`: HttpOnly, SameSite=Lax and no
  `Domain`, so each wiki address keeps its own cookie. It is `Secure` when
  `wikiGateway.scheme` is `https`. The `Cookie` header is removed from requests
  to the wiki pod.
- Without a valid cookie the answer is a 401 page that sends the browser to the
  bootstrap page and back.
- Each wiki is health-checked on `/naira-wiki.json` every 5 seconds. A wiki that
  is stopped, unreachable or failing its check gets a 503 page with its name,
  owner and a link to its `source`, on its own address. Other wikis are not
  affected.
- Responses carry `Content-Security-Policy: frame-ancestors
  <wikiGateway.portalOrigin>` and no `Server` or `x-envoy-*` header.
- The gateway restarts when its Envoy config changes, not when a wiki's digest
  does. The two pages are in `files/wiki-gateway/`.

## Scheduling and security

Same keys on `catalog`, `ui` and `portal` unless noted; unset values render
nothing. `imagePullSecrets` is top-level and applies to all three. `wikiGateway`
(the gateway) and `wikiDefaults` (the wiki pods) take the same keys, except
`startupProbe`.

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
non-root images for both first. The wiki gateway and the wiki pods run non-root
with a read-only root filesystem and no capabilities, which also meets
`restricted`.

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
source; and, for `wikis`, an entry that is not a map or has a missing, invalid,
reserved (`gateway`) or duplicate `id`. For an enabled wiki: a contract other
than `naira.wiki/v1alpha1`, an `image` that is missing, not pinned as
`name@sha256:<digest>` or outside `wikiGateway.allowedRegistries`, an `order`
that is not a whole number, and a second `primary`. Once a wiki is enabled,
a `wikiGateway.domain`, `scheme` or `port` that cannot form an address, a
`wikiGateway.image` without a repository or a `sha256:` digest, a
`wikiGateway.portalOrigin` that is not `scheme://host[:port]`, a
`keycloak.baseUrl` that is not an http(s) URL with a host, and an empty
`keycloak.issuer` or `keycloak.realm`. The message names the wiki or the value. A bad entry fails the whole render, so the previous
release keeps running.

## Tests

`deploy/charts/naira/tests/render-test.sh`

The render tests read the Envoy config back from the ConfigMap but cannot tell
whether Envoy accepts it. After changing `templates/wiki-gateway.yaml`, run
`envoy --mode validate -c envoy.yaml` on the rendered `envoy.yaml` with the image
from `wikiGateway.image`.
