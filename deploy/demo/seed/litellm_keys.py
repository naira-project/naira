"""Create the demo's scoped LiteLLM virtual keys (manifests/claims.yaml).

Idempotent: an existing key gets its model list reset instead.
"""

import json
import os
import time
import urllib.error
import urllib.request

BASE = os.environ.get("LITELLM_BASE_URL", "http://litellm:4000").rstrip("/")
MASTER_KEY = os.environ["LITELLM_MASTER_KEY"]

KEYS = [
    {
        "key": "sk-claimsassistant2026nov",
        "key_alias": "claims-assistant",
        "models": ["idp-claude-sonnet", "idp-embedding-small"],
        "metadata": {"team": "claims-ai", "app": "claims-assistant"},
    },
    {
        "key": "sk-piiredactorgateway2026",
        "key_alias": "pii-redactor",
        "models": ["idp-llama-qwen25-05b"],
        "metadata": {"team": "claims-ai", "app": "pii-redactor"},
    },
    {
        "key": "sk-claimsfaqbotlegacy2024",
        "key_alias": "claims-faq-bot",
        "models": ["openrouter-gpt-3.5-turbo"],
        "metadata": {"team": "claims-service", "app": "claims-faq-bot"},
    },
]


def call(method: str, path: str, payload: dict | None = None) -> dict:
    req = urllib.request.Request(
        f"{BASE}{path}",
        data=json.dumps(payload).encode() if payload is not None else None,
        method=method,
        headers={"Authorization": f"Bearer {MASTER_KEY}", "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.load(resp)


def wait_for_litellm() -> None:
    for _ in range(90):
        try:
            call("GET", "/health/readiness")
            return
        except (urllib.error.URLError, OSError):
            time.sleep(5)
    raise SystemExit(f"LiteLLM at {BASE} not ready")


def key_exists(key: str) -> bool:
    try:
        call("GET", f"/key/info?key={key}")
        return True
    except urllib.error.HTTPError as err:
        if err.code in (400, 404):
            return False
        raise


def main() -> None:
    wait_for_litellm()
    for spec in KEYS:
        if key_exists(spec["key"]):
            call("POST", "/key/update", spec)
            print(f"Updated key {spec['key_alias']}: {spec['models']}")
        else:
            call("POST", "/key/generate", spec)
            print(f"Created key {spec['key_alias']}: {spec['models']}")


if __name__ == "__main__":
    main()
