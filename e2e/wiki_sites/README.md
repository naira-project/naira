# wiki_sites

Proves that wikis installed from the naira chart open behind the wiki gateway
and show up in the portal. It deploys `keycloak` (auth), `catalog` (the script
always deploys it, with no plugins), `wiki-sites` (the gateway and two wikis,
`starter` and `starter-two`, rendered from the chart) and `portal` into one
fresh namespace, then checks them at HTTP level. Used per-PR in CI; also
runnable locally. See [`e2e/README.md`](../README.md) for how the component
selection works. How a wiki and the gateway behave is described in
[`docs/wikis.md`](../../docs/wikis.md).

## Run it locally

Prerequisites and the cluster are as in
[`litellm_chatbot_to_catalog_api`](../litellm_chatbot_to_catalog_api/README.md#prerequisites).
The wikis are built from `wiki/starter`, and the portal image includes an
Angular build, so the first run takes several minutes.

```bash
kind create cluster --config e2e/base/kind-config.yaml
./e2e/scripts/create-environment.sh --scenario wiki_sites \
  --env-id local --tag localtest --cluster-name naira-idp-e2e

NS=local
kubectl -n "$NS" port-forward svc/wiki-gateway 8083:8080 &
kubectl -n "$NS" port-forward svc/keycloak 8080:8080 &
kubectl -n "$NS" port-forward svc/portal 3000:3000 &

WIKI_GATEWAY_URL=http://localhost:8083 KEYCLOAK_URL=http://localhost:8080 \
  PORTAL_URL=http://localhost:3000 E2E_NAMESPACE="$NS" \
  go test -tags e2e -v ./e2e/wiki_sites/assert/...

kill %1 %2 %3
./e2e/scripts/destroy-environment.sh --env-id "$NS" --cluster-name naira-idp-e2e
kind delete cluster --name naira-idp-e2e
```

## What's in the namespace

| Component | Notes |
|---|---|
| `wiki-sites` | `wiki-gateway`, `wiki-starter` and `wiki-starter-two`, rendered by `helm template` from `deploy/charts/naira` with [`wiki-sites-values.yaml`](../components/wiki-sites-values.yaml) |
| `portal` | Reads the `wikis` ConfigMap of `wiki-sites` once at start, so `components.env` lists `wiki-sites` first |
| `keycloak` | The gateway checks its tokens: issuer and keys are the in-cluster address |
| `catalog` | Deployed by the script, not looked at |

The chart takes a wiki image only by digest from an allowed registry, so the
values name a placeholder digest and `create-environment.sh` swaps it for the
image it loaded. Everything else, including the Envoy config, is what the chart
renders.

The seed Job waits until both wikis answer behind the gateway: the gateway is
ready when it listens, but its health checks need a first round before it
routes.

## What the tests assert

`assert/assert_test.go`, run with `go test -tags e2e`:

- `TestWithoutSessionTheWikiAsksToSignIn`: a request without a cookie gets a 401
  page that redirects to the wiki's sign-in page and shows none of the wiki.
- `TestOnlyTheSignInPageIsOpen`: the sign-in page is open, `naira-wiki.json` is not.
- `TestSessionOpensTheWiki`: a real Keycloak token opens a session whose cookie
  has no `Domain`, and the cookie opens the wiki.
- `TestTokenIsAcceptedOnlyAsSessionCookie`: a token in `Authorization`, in
  `x-naira-token` on another path or broken is refused.
- `TestUnknownHostIsNotFound`: a host that is no wiki's gets a 404.
- `TestStoppedWikiShowsDegradedPageAndOthersKeepWorking`: with `wiki-starter-two`
  scaled to 0 it shows the degraded page and `starter` keeps answering.
- `TestPortalListsTheWikisInOrder`: `/rest/config` of the portal lists the wikis
  in the Docs group after the portal pages, the primary first.

## Manual checks

Deep links, anchors and search need a browser. Run `tilt up` and open a wiki from
the Docs group. A wiki's page only loads when its address is on the same site as
the portal, see [`docs/wikis.md`](../../docs/wikis.md#same-site). Record the
result in the PR:

- A link to a nested page opens that page inside the portal.
- A link with a `#heading` opens the page. Luigi drops the heading today, so the
  page opens at the top.
- A result of the wiki's search opens inside the portal.

## Troubleshooting

**`helm template` fails in `create-environment.sh`**: the PostgreSQL subchart is
not vendored. The script runs `helm dependency build deploy/charts/naira` when
`charts/` has no archive, which needs network.

**`wiki/starter/Dockerfile` not found**: the starter has to be in the checkout.

**Tests fail with 503 right after setup**: the gateway has not seen the wikis
healthy yet. The seed Job waits for that; check `kubectl -n "$NS" logs job/seed-$NS`.
