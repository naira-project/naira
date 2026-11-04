"""Seed OpenMetadata with the claims data warehouse: tables, PII tags and lineage.

The openmetadata plugin turns tables into dataset nodes and lineage into
"derived_from" relations. Idempotent: every call is a PUT (create or update).
"""

import base64
import json
import os
import time
import urllib.error
import urllib.request

BASE = os.environ.get("OPENMETADATA_HOST_PORT", "http://openmetadata:8585").rstrip("/")
EMAIL = os.environ.get("OPENMETADATA_ADMIN_EMAIL", "admin@open-metadata.org")
PASSWORD = os.environ.get("OPENMETADATA_ADMIN_PASSWORD", "admin")

SERVICE = "claims_warehouse"
DATABASE = "claims"
SCHEMA = "curated"


def tag(fqn: str) -> dict:
    return {"tagFQN": fqn, "source": "Classification", "labelType": "Manual", "state": "Confirmed"}


TABLES = [
    {
        "name": "claims_raw",
        "description": "Claims as submitted through the claims portal, including free-text notes.",
        "tags": [tag("PII.Sensitive"), tag("Tier.Tier1")],
        "columns": [
            {"name": "claim_id", "dataType": "BIGINT"},
            {"name": "policy_holder_name", "dataType": "VARCHAR", "dataLength": 255},
            {"name": "notes", "dataType": "TEXT"},
            {"name": "submitted_at", "dataType": "TIMESTAMP"},
        ],
    },
    {
        "name": "policies",
        "description": "Policy master data.",
        "tags": [tag("PII.Sensitive")],
        "columns": [
            {"name": "policy_id", "dataType": "BIGINT"},
            {"name": "product", "dataType": "VARCHAR", "dataLength": 64},
            {"name": "holder_birth_date", "dataType": "DATE"},
        ],
    },
    {
        "name": "payments",
        "description": "Claim payouts.",
        "tags": [tag("Tier.Tier1")],
        "columns": [
            {"name": "payment_id", "dataType": "BIGINT"},
            {"name": "claim_id", "dataType": "BIGINT"},
            {"name": "amount", "dataType": "DECIMAL"},
        ],
    },
    {
        "name": "claims_labeled_2025",
        "description": "Pseudonymized 2025 claims with handling-team labels. Training data for claims-triage-classifier.",
        "tags": [tag("PII.NonSensitive")],
        "columns": [
            {"name": "claim_id", "dataType": "BIGINT"},
            {"name": "notes_redacted", "dataType": "TEXT"},
            {"name": "product", "dataType": "VARCHAR", "dataLength": 64},
            {"name": "team_label", "dataType": "VARCHAR", "dataLength": 32},
        ],
    },
    {
        "name": "fraud_features",
        "description": "Per-claim fraud features. Training data for fraud-signal-scorer.",
        "tags": [tag("PII.NonSensitive")],
        "columns": [
            {"name": "claim_id", "dataType": "BIGINT"},
            {"name": "payout_ratio", "dataType": "DOUBLE"},
            {"name": "prior_claims", "dataType": "INT"},
        ],
    },
]

LINEAGE = [
    ("claims_raw", "claims_labeled_2025"),
    ("policies", "claims_labeled_2025"),
    ("claims_labeled_2025", "fraud_features"),
    ("payments", "fraud_features"),
]


def request(method: str, path: str, payload: dict | None = None, token: str = "") -> dict:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(
        f"{BASE}/api/v1/{path}",
        data=json.dumps(payload).encode() if payload is not None else None,
        method=method,
        headers=headers,
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        body = resp.read()
        # Some endpoints (e.g. lineage) reply 200 with an empty body.
        return json.loads(body) if body else {}


def login() -> str:
    # OpenMetadata takes minutes to start in kind.
    for _ in range(120):
        try:
            payload = {"email": EMAIL, "password": base64.b64encode(PASSWORD.encode()).decode()}
            return request("POST", "users/login", payload)["accessToken"]
        except (urllib.error.URLError, OSError):
            time.sleep(10)
    raise SystemExit(f"OpenMetadata at {BASE} not ready")


def main() -> None:
    token = login()
    request(
        "PUT",
        "services/databaseServices",
        {
            "name": SERVICE,
            "serviceType": "CustomDatabase",
            "connection": {
                "config": {
                    "type": "CustomDatabase",
                    "sourcePythonClass": "metadata.ingestion.source.database.customdatabase.connection",
                }
            },
        },
        token,
    )
    request("PUT", "databases", {"name": DATABASE, "service": SERVICE}, token)
    request("PUT", "databaseSchemas", {"name": SCHEMA, "database": f"{SERVICE}.{DATABASE}"}, token)

    schema_fqn = f"{SERVICE}.{DATABASE}.{SCHEMA}"
    table_ids = {}
    for table in TABLES:
        created = request("PUT", "tables", {**table, "databaseSchema": schema_fqn}, token)
        table_ids[table["name"]] = created["id"]
        print(f"Seeded table {schema_fqn}.{table['name']}")

    for upstream, downstream in LINEAGE:
        edge = {
            "fromEntity": {"id": table_ids[upstream], "type": "table"},
            "toEntity": {"id": table_ids[downstream], "type": "table"},
        }
        request("PUT", "lineage", {"edge": edge}, token)
        print(f"Seeded lineage {upstream} -> {downstream}")


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as err:
        raise SystemExit(f"OpenMetadata seeding failed: {err} -> {err.read().decode()}")
