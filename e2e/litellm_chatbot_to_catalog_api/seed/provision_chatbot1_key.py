"""Mints chatbot1 a LiteLLM virtual key scoped to just the "openai" model,
and writes it into chatbot1's Secret.

Without this, chatbot1 would authenticate to LiteLLM with the shared master
key (see components/litellm.yaml's LITELLM_MASTER_KEY), which is valid for
every model on the instance — depl_uses_litellm would then discover
chatbot1 -> openai AND chatbot1 -> mistral, not the single edge the
litellm_chatbot_to_catalog_api scenario asserts on. A model-scoped virtual
key needs LiteLLM's key-management API, which needs a database — that's why
this scenario deploys postgres.

Uses the seed Job's own in-cluster ServiceAccount (RBAC: get/patch on
Secrets in this namespace, see seed-job.yaml) to patch the Secret directly —
stdlib-only, no Kubernetes client library needed for one PATCH call.
"""

import json
import os
import ssl
import urllib.request

LITELLM_URL = os.environ.get("LITELLM_URL", "http://localhost:4000").rstrip("/")
LITELLM_MASTER_KEY = os.environ["LITELLM_MASTER_KEY"]
NAMESPACE = os.environ["POD_NAMESPACE"]

K8S_API_SERVER = "https://kubernetes.default.svc"
SA_DIR = "/var/run/secrets/kubernetes.io/serviceaccount"


def generate_scoped_key():
    req = urllib.request.Request(
        f"{LITELLM_URL}/key/generate",
        data=json.dumps({"models": ["openai"]}).encode(),
        method="POST",
        headers={
            "Authorization": f"Bearer {LITELLM_MASTER_KEY}",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.load(resp)["key"]


def patch_chatbot1_secret(key):
    with open(f"{SA_DIR}/token") as f:
        token = f.read().strip()
    ctx = ssl.create_default_context(cafile=f"{SA_DIR}/ca.crt")

    url = f"{K8S_API_SERVER}/api/v1/namespaces/{NAMESPACE}/secrets/chatbot1-secrets"
    req = urllib.request.Request(
        url,
        data=json.dumps({"stringData": {"LITELLM_API_KEY": key}}).encode(),
        method="PATCH",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/strategic-merge-patch+json",
        },
    )
    with urllib.request.urlopen(req, timeout=10, context=ctx) as resp:
        if resp.status != 200:
            raise RuntimeError(f"PATCH {url}: status {resp.status}")


def main():
    key = generate_scoped_key()
    patch_chatbot1_secret(key)
    print("chatbot1-secrets patched with an openai-scoped LiteLLM key")


if __name__ == "__main__":
    main()
