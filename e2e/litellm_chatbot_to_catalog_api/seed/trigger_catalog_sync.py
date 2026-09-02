"""Triggers a full catalog plugin run and waits for it to finish.

The catalog's node store is populated on demand (POST /v1/plugins:run), not
by polling plugins in the background — so configuring a plugin's backend
alone (e.g. litellm.yaml's static model list) leaves the catalog's graph
empty until a run executes. This makes sure one
has completed before the seed Job reports success, so E2E tests that query
/v1/nodes right after don't race an empty store.
"""

import json
import os
import sys
import time
import urllib.parse
import urllib.request

CATALOG_URL = os.environ.get("CATALOG_URL", "http://localhost:8090").rstrip("/")
KEYCLOAK_URL = os.environ.get("KEYCLOAK_URL", "http://localhost:8080").rstrip("/")
KEYCLOAK_REALM = os.environ.get("KEYCLOAK_REALM", "naira")
KEYCLOAK_CLIENT_ID = os.environ.get("KEYCLOAK_CLIENT_ID", "naira-portal")
KEYCLOAK_CLIENT_SECRET = os.environ.get("KEYCLOAK_CLIENT_SECRET", "naira-e2e-test-secret")
KEYCLOAK_USERNAME = os.environ.get("KEYCLOAK_USERNAME", "testuser")
KEYCLOAK_PASSWORD = os.environ.get("KEYCLOAK_PASSWORD", "testpass")
POLL_TIMEOUT_SECONDS = 120
POLL_INTERVAL_SECONDS = 2


def fetch_token():
    """Password-grant token from the realm/user/client seeded by keycloak.yaml
    (see deploy/dev/stacks/core/infra/keycloak/naira-realm.json for the
    source of truth this mirrors). The catalog API requires a Bearer token
    on every /v1/* route as of the Keycloak auth integration."""
    form = urllib.parse.urlencode({
        "grant_type": "password",
        "client_id": KEYCLOAK_CLIENT_ID,
        "client_secret": KEYCLOAK_CLIENT_SECRET,
        "username": KEYCLOAK_USERNAME,
        "password": KEYCLOAK_PASSWORD,
    }).encode()
    url = f"{KEYCLOAK_URL}/realms/{KEYCLOAK_REALM}/protocol/openid-connect/token"
    req = urllib.request.Request(url, data=form, method="POST")
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.load(resp)["access_token"]


def request(method, path, token, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(f"{CATALOG_URL}{path}", data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.load(resp)


def main():
    token = fetch_token()
    triggered = request("POST", "/v1/plugins:run", token)
    operations = {op["name"]: op for op in triggered["operations"]}
    print(f"Triggered {len(operations)} plugin run(s): {sorted(operations)}")

    deadline = time.monotonic() + POLL_TIMEOUT_SECONDS
    while True:
        pending = [name for name, op in operations.items() if op["state"] in ("PENDING", "RUNNING")]
        if not pending:
            break
        if time.monotonic() > deadline:
            print(f"Timed out after {POLL_TIMEOUT_SECONDS}s waiting on: {pending}", file=sys.stderr)
            sys.exit(1)
        time.sleep(POLL_INTERVAL_SECONDS)
        for name in pending:
            operations[name] = request("GET", f"/v1/operations/{name}", token)

    failed = {name: op for name, op in operations.items() if op["state"] == "FAILED"}
    for name, op in operations.items():
        print(f"  {name}: {op['state']} (plugin={op['plugin']}, nodesUpserted={op.get('nodesUpserted', 0)})")

    if failed:
        print(f"{len(failed)} plugin run(s) failed", file=sys.stderr)
        sys.exit(1)

    print("Catalog sync complete")


if __name__ == "__main__":
    main()
