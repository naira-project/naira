# Wikis

A wiki is a static site that a team writes in Git, builds into a container image
and installs in Naira. Naira shows it in the sidebar, in the Docs group, and
opens it inside the portal. Naira does not edit or index wikis, and a wiki's
pages never become catalog properties.

This page is for the operator who installs wikis. The author of a wiki starts
from [`wiki/starter`](../wiki/starter/README.md), which also defines what a wiki
image has to provide. [`wiki/howto`](../wiki/howto/README.md) and
[`wiki/example`](../wiki/example/README.md) are two finished wikis built from it. What the chart renders and what each value does is in the
[chart README](../deploy/charts/naira/README.md#wikis) and in
[`values.yaml`](../deploy/charts/naira/values.yaml).

## Install a wiki

Add the wiki to `wikis` and the gateway's address to `wikiGateway`:

```yaml
wikiGateway:
  domain: wiki.naira.example.com
  scheme: https
  port: 443
  portalOrigin: https://naira.example.com
  allowedRegistries: [ghcr.io/acme]
wikis:
  - id: platform-handbook
    contract: naira.wiki/v1alpha1
    label: Platform Handbook
    primary: true
    image: ghcr.io/acme/platform-handbook@sha256:<digest>
```

The wiki opens at `https://platform-handbook.wiki.naira.example.com/` and in the
portal at `/platform-handbook`. An upgrade is a new digest, and the address stays.
`enabled: false` or removing the entry removes the wiki and nothing else. The
chart refuses an image without a digest or from a registry that is not in
`allowedRegistries`, and it refuses a duplicate or invalid `id`; a refused render
leaves the running release as it is.

## DNS and TLS

Every wiki has its own address, `<id>.<wikiGateway.domain>`, so that its scripts
run on an origin of their own and cannot read the portal's session or another
wiki's pages. Point `*.<wikiGateway.domain>` at the Service `wiki-gateway` and
get a wildcard certificate for it. The gateway speaks plain HTTP on 8080: end TLS
in front of it, in your Ingress, Gateway or load balancer, and pass the `Host`
header on.

## Same site

The wiki's address and the portal's address have to share a registrable domain,
for example `naira.example.com` and `*.wiki.naira.example.com`, which share
`example.com`. The gateway sets its session cookie from inside the portal's
frame. A browser drops a `SameSite=Lax` cookie that a frame of another site sets,
and the wiki's sign-in page then stops with "The browser did not keep the wiki
session". A same-site cookie is first-party, which blocking of third-party
cookies does not touch.

For local development this means the default does not work in Chrome: `localhost`
and `wiki.localhost` are different sites. Serve the portal on `naira.localhost`
and set `wikiGateway.domain` to `naira.localhost` and `wikiGateway.portalOrigin`
to the portal's origin. The dev Keycloak realm of `test-dependencies` allows
redirects only from `http://localhost:3000/*` and `http://localhost:3001/*`, so
a portal on `naira.localhost` cannot sign in until the realm lists
`http://naira.localhost:3000/*` too. `tilt up` installs the how-to and the example wiki,
which the portal lists in the Docs group, behind a gateway on port 3003.

## What a wiki can do with a user's token

Wiki code runs in the user's browser with the user's access token. The portal
hands the token to every frame it shows, and a script in a wiki can call
`LuigiClient.getToken()`. The catalog accepts any token of the configured
issuer, so a wiki can call the catalog API as the user. VitePress sites ship
JavaScript, and Markdown can embed Vue components.

In this version Naira does not prevent that. Install only wikis whose code your
organisation trusts:

- Pin the image by digest. The chart enforces this.
- Pull it from a registry you control or trust. The chart enforces this with
  `wikiGateway.allowedRegistries`.
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

- Every signed-in user can open every wiki. Restricting a wiki to a group needs
  an authorization decision that has not been taken.
- Luigi drops a `#heading` from a link, so the wiki opens at the top of the page.
