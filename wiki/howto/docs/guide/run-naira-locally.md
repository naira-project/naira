# Run Naira locally

You get the whole stack on a local [kind](https://kind.sigs.k8s.io/) cluster: the catalog, the
plugins, the UI and the portal, plus Keycloak, LiteLLM, MLflow and other systems for the plugins
to read from.

## Before you start

You need Docker running, and the tools `kind`, `tilt`, `task` and `go`. The repository pins their
versions in `mise.toml`. With [mise](https://mise.jdx.dev/) installed and activated, run this once
in a checkout of [naira-project/naira](https://github.com/naira-project/naira):

```sh
mise install
```

The stack is large. On one development machine the kind node used about 11 GB of memory, so give
Docker as much as you can.

## Start it

1. Create the cluster. This also selects its kube context:

   ```sh
   kind create cluster --name naira-idp --image kindest/node:v1.36.1
   ```

2. Deploy everything:

   ```sh
   tilt up
   ```

   Tilt builds the images, installs the published `test-dependencies` chart into the namespace
   `naira-deps` and the Naira chart into `idp-system`, and forwards the ports. It stops if the kube
   context is not `kind-naira-idp`.

3. Wait until the resources in the Tilt UI are green, then open `http://localhost:3000`. Sign in to
   Keycloak with the local test user:

   | User | Password |
   |---|---|
   | `testuser` | `testpass` |

You should see the Naira portal. The catalog is empty until you [run the plugins](/guide/run-plugins).

## Ports

Tilt forwards these to your machine.

| Port | What |
|---|---|
| 3000 | Portal, the entry point |
| 3001 | UI, the pages the portal shows |
| 8090 | Catalog API, see [Call the catalog API](/guide/call-the-catalog-api) |
| 8080 | Keycloak |

## Change a component or stop

To change which components `test-dependencies` installs, or to change that chart itself, use a
checkout of it: `tilt up -- --deps-repo=<path>`. Tilt forwards only the enabled components.

`tilt down` removes everything Tilt deployed.

::: tip
The local credentials above are for development only. Do not reuse them anywhere else.
:::
