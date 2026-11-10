# Handbooks

A handbook is a static site that a team writes in Git, builds into a container image
and installs in Naira. Naira shows it in the sidebar, in the Docs group, and
opens it inside the portal. Naira does not edit or index handbooks, and a handbook's
pages never become catalog properties.

This page is for the operator who installs handbooks. The author of a handbook starts
from [`handbook/starter`](../handbook/starter/README.md), which also defines what a handbook
image has to provide. Two finished handbooks are built from it:
[`handbook/howto`](../handbook/howto/README.md), the Naira how-to, and
[`naira-project/example-handbook`](https://github.com/naira-project/example-handbook), an
example team handbook in a repository of its own. What the chart renders and what each
value does is in the [chart README](../deploy/charts/naira/README.md#handbooks) and in
[`values.yaml`](../deploy/charts/naira/values.yaml).

## Publish a handbook from its own repository

A handbook lives in its owner's repository, not in `naira`. The repository calls
`reusable-handbook-build.yml` from
[`naira-github-workflows`](https://github.com/naira-project/naira-github-workflows), which
builds the image, signs it, attests its provenance, pushes it and checks its
`naira-handbook.json`. The run summary names the image with its digest; that reference goes
into `handbooks`. `naira-project/example-handbook` is a complete example: its README lists
what a team changes to start its own handbook from it.

## Install a handbook

Add the handbook to `handbooks` and the gateway's address to `handbookGateway`:

```yaml
handbookGateway:
  domain: handbook.naira.example.com
  scheme: https
  port: 443
  portalOrigin: https://naira.example.com
  allowedRegistries: [ghcr.io/acme]
handbooks:
  - id: platform-handbook
    contract: naira.handbook/v1alpha1
    label: Platform Handbook
    primary: true
    image: ghcr.io/acme/platform-handbook@sha256:<digest>
```

The handbook opens at `https://platform-handbook.handbook.naira.example.com/` and in the
portal at `/platform-handbook`. An upgrade is a new digest, and the address stays.
`enabled: false` or removing the entry removes the handbook and nothing else. The
chart refuses an image without a digest or from a registry that is not in
`allowedRegistries`, and it refuses a duplicate or invalid `id`; a refused render
leaves the running release as it is.

## DNS and TLS

Every handbook has its own address, `<id>.<handbookGateway.domain>`, so that its scripts
run on an origin of their own and cannot read the portal's session or another
handbook's pages. Point `*.<handbookGateway.domain>` at the Service `handbook-gateway` and
get a wildcard certificate for it. The gateway speaks plain HTTP on 8080: end TLS
in front of it, in your Ingress, Gateway or load balancer, and pass the `Host`
header on.

## Same site

The handbook's address and the portal's address have to share a registrable domain,
for example `naira.example.com` and `*.handbook.naira.example.com`, which share
`example.com`. The gateway sets its session cookie from inside the portal's
frame. A browser drops a `SameSite=Lax` cookie that a frame of another site sets,
and the handbook's sign-in page then stops with "The browser did not keep the handbook
session". A same-site cookie is first-party, which blocking of third-party
cookies does not touch.

For local development this means the chart default does not work in Chrome:
`localhost` and `handbook.localhost` are different sites. `values-dev.yaml` therefore
sets `handbookGateway.domain` to `naira.localhost` and `handbookGateway.portalOrigin` to
`http://naira.localhost:3000`. Open the portal at that address, not at
`localhost:3000`. `tilt up` installs the how-to, which the portal lists in the Docs
group, behind a gateway on port 3003, at `<id>.naira.localhost:3003`.

### Add the example handbook

To try more than one handbook, or to try a handbook you are writing, let Tilt build it from
a local clone:

```sh
git clone https://github.com/naira-project/example-handbook ../example-handbook
tilt up -- --handbook-repo ../example-handbook
# or, while Tilt runs:
tilt args -- --handbook-repo ../example-handbook
```

`--handbook-repo` can be given more than once. The handbook's id is the `name` in its
`package.json`, and it opens at `<id>.naira.localhost:3003`. Tilt rebuilds it when its files
change.

To install the published example instead, add it to `values-dev.yaml`. Take the digest
from the latest run of its `Handbook Build` workflow. Tilt pulls an image that is pinned by
digest instead of building it.

```yaml
handbookGateway:
  allowedRegistries: [local, ghcr.io/naira-project]
handbooks:
  # after the how-to entry
  - id: example
    contract: naira.handbook/v1alpha1
    label: Example Handbook
    order: 1
    image: ghcr.io/naira-project/example-handbook@sha256:<digest>
```

The portal on `naira.localhost` signs in only when the client `naira-portal` of the
dev Keycloak realm lists `http://naira.localhost:3000/*` as a redirect URI. The
`test-dependencies` chart 0.1.0, which the Tiltfile pins, lists only
`http://localhost:3000/*` and `http://localhost:3001/*`. Until the Tiltfile pins a
release that lists it, add the URI in the Keycloak admin console (realm `naira`,
Clients, `naira-portal`, Valid redirect URIs).

## What a handbook can do with a user's token

Handbook code runs in the user's browser with the user's access token. The portal
hands the token to every frame it shows, and a script in a handbook can read it
from `LuigiClient.getContext().token`. The catalog accepts any token of the configured
issuer, so a handbook can call the catalog API as the user. VitePress sites ship
JavaScript, and Markdown can embed Vue components.

In this version Naira does not prevent that. Install only handbooks whose code your
organisation trusts:

- Pin the image by digest. The chart enforces this.
- Pull it from a registry you control or trust. The chart enforces this with
  `handbookGateway.allowedRegistries`.
- Install it only if it is signed by a workflow identity you allow. The chart
  cannot check signatures. Check them before the digest goes into your values.

## Verify the signature

`reusable-container-build.yml` in
[`naira-github-workflows`](https://github.com/naira-project/naira-github-workflows)
signs images with Cosign (keyless) and attests their provenance, and
`reusable-container-verify.yml` in the same repository verifies them. Run the
verify workflow in the pipeline that updates your values, or enforce the same in
the cluster with an admission policy. By hand:

```bash
cosign verify ghcr.io/acme/platform-handbook@sha256:<digest> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/acme/.+/.github/workflows/.+@refs/heads/main$'
```

Set the identity to the workflows of your organisation that you trust.

## Known limits

- Every signed-in user can open every handbook. Restricting a handbook to a group needs
  an authorization decision that has not been taken.
- Luigi drops a `#heading` from a link, so the handbook opens at the top of the page.
