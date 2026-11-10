# handbook_sites

Proves that handbooks installed from the naira chart open behind the handbook gateway
and show up in the portal. It deploys `keycloak` (auth), `catalog` (the script
always deploys it, with no plugins), `handbook-sites` (the gateway and two handbooks,
`starter` and `starter-two`, rendered from the chart) and `portal` into one
fresh namespace, then checks them at HTTP level. Used per-PR in CI; also
runnable locally. See [`e2e/README.md`](../README.md) for how the component
selection works. How a handbook and the gateway behave is described in
[`docs/handbooks.md`](../../docs/handbooks.md).

## Run it locally

Prerequisites and the cluster are as in
[`litellm_chatbot_to_catalog_api`](../litellm_chatbot_to_catalog_api/README.md#prerequisites).
The handbooks are built from `handbook/starter`, and the portal image includes an
Angular build, so the first run takes several minutes.

```bash
kind create cluster --config e2e/base/kind-config.yaml
./e2e/scripts/create-environment.sh --scenario handbook_sites \
  --env-id local --tag localtest --cluster-name naira-idp-e2e

NS=local
kubectl -n "$NS" port-forward svc/handbook-gateway 8083:8080 &
kubectl -n "$NS" port-forward svc/keycloak 8080:8080 &
kubectl -n "$NS" port-forward svc/portal 3000:3000 &

HANDBOOK_GATEWAY_URL=http://localhost:8083 KEYCLOAK_URL=http://localhost:8080 \
  PORTAL_URL=http://localhost:3000 E2E_NAMESPACE="$NS" \
  go test -tags e2e -v ./e2e/handbook_sites/assert/...

kill %1 %2 %3
./e2e/scripts/destroy-environment.sh --env-id "$NS" --cluster-name naira-idp-e2e
kind delete cluster --name naira-idp-e2e
```

## What's in the namespace

| Component | Notes |
|---|---|
| `handbook-sites` | `handbook-gateway`, `handbook-starter` and `handbook-starter-two`, rendered by `helm template` from `deploy/charts/naira` with [`handbook-sites-values.yaml`](../components/handbook-sites-values.yaml) |
| `portal` | Reads the `handbooks` ConfigMap of `handbook-sites` once at start, so `components.env` lists `handbook-sites` first |
| `keycloak` | The gateway checks its tokens: issuer and keys are the in-cluster address |
| `catalog` | Deployed by the script, not looked at |

The chart takes a handbook image only by digest from an allowed registry, so the
values name a placeholder digest and `create-environment.sh` swaps it for the
image it loaded. Everything else, including the Envoy config, is what the chart
renders.

The seed Job waits until both handbooks answer behind the gateway: the gateway is
ready when it listens, but its health checks need a first round before it
routes.

## What the tests assert

`assert/assert_test.go`, run with `go test -tags e2e`:

- `TestWithoutSessionTheHandbookAsksToSignIn`: a request without a cookie gets a 401
  page that redirects to the handbook's sign-in page and shows none of the handbook.
- `TestOnlyTheSignInPageIsOpen`: the sign-in page is open, `naira-handbook.json` is not.
- `TestSessionOpensTheHandbook`: a real Keycloak token opens a session whose cookie
  has no `Domain`, and the cookie opens the handbook.
- `TestTokenIsAcceptedOnlyAsSessionCookie`: a token in `Authorization`, in
  `x-naira-token` on another path or broken is refused.
- `TestUnknownHostIsNotFound`: a host that is no handbook's gets a 404.
- `TestStoppedHandbookShowsDegradedPageAndOthersKeepWorking`: with `handbook-starter-two`
  scaled to 0 it shows the degraded page and `starter` keeps answering.
- `TestPortalListsTheHandbooksInOrder`: `/rest/config` of the portal lists the handbooks
  in the Docs group after the portal pages, the primary first.

## Manual checks

Deep links, anchors and search need a browser. Run `tilt up` and open a handbook from
the Docs group. A handbook's page only loads when its address is on the same site as
the portal, see [`docs/handbooks.md`](../../docs/handbooks.md#same-site). Record the
result in the PR:

- A link to a nested page opens that page inside the portal.
- A link with a `#heading` opens the page. Luigi drops the heading today, so the
  page opens at the top.
- A result of the handbook's search opens inside the portal.

## Troubleshooting

**`helm template` fails in `create-environment.sh`**: the PostgreSQL subchart is
not vendored. The script runs `helm dependency build deploy/charts/naira` when
`charts/` has no archive, which needs network.

**`handbook/starter/Dockerfile` not found**: the starter has to be in the checkout.

**Tests fail with 503 right after setup**: the gateway has not seen the handbooks
healthy yet. The seed Job waits for that; check `kubectl -n "$NS" logs job/seed-$NS`.
