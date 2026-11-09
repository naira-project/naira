"""Waits until every handbook answers behind the gateway.

The gateway is ready when it listens, but its health checks need a first round
before it routes to a handbook, and it fetches Keycloak's keys on the first token.
The assert step starts after this, so it does not race either.
"""
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

KEYCLOAK_URL = os.environ["KEYCLOAK_URL"]
GATEWAY_URL = os.environ["GATEWAY_URL"]
# The addresses a browser would use, comma separated.
HANDBOOK_HOSTS = os.environ["HANDBOOK_HOSTS"].split(",")
DEADLINE = time.time() + 180


def fetch_token():
    form = urllib.parse.urlencode({
        "grant_type": "password",
        "client_id": "naira-portal",
        "client_secret": "naira-e2e-test-secret",
        "username": "testuser",
        "password": "testpass",
    }).encode()
    url = f"{KEYCLOAK_URL}/realms/naira/protocol/openid-connect/token"
    with urllib.request.urlopen(urllib.request.Request(url, data=form), timeout=10) as resp:
        return json.load(resp)["access_token"]


def request(host, method, path, headers):
    req = urllib.request.Request(GATEWAY_URL + path, method=method, headers={"Host": host, **headers})
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.headers
    except urllib.error.HTTPError as err:
        return err.code, err.headers


def wait_for(host):
    token = fetch_token()
    while time.time() < DEADLINE:
        status, headers = request(host, "POST", "/_naira/session", {"x-naira-token": token})
        if status == 204:
            cookie = headers["Set-Cookie"].split(";")[0]
            status, _ = request(host, "GET", "/naira-handbook.json", {"Cookie": cookie})
            if status == 200:
                print(f"{host} answers")
                return
        print(f"{host}: not yet ({status})")
        time.sleep(3)
    sys.exit(f"{host} did not answer in time")


for handbook_host in HANDBOOK_HOSTS:
    wait_for(handbook_host)
