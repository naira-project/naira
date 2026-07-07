"""Seed OpenMetadata with a sample database service, tables, and lineage."""

import base64
import os
import sys

import requests

def login(base: str, email: str, password: str) -> str:
    resp = requests.post(
        f"{base}/api/v1/users/login",
        json={"email": email, "password": base64.b64encode(password.encode()).decode()},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()["accessToken"]


def put(base: str, token: str, path: str, payload: dict) -> dict:
    resp = requests.put(
        f"{base}/api/v1/{path}",
        json=payload,
        headers={"Authorization": f"Bearer {token}"},
        timeout=30,
    )
    resp.raise_for_status()
    # Some endpoints (e.g. lineage) reply 200 with an empty body.
    return resp.json() if resp.content else {}


SERVICE = "naira_sample"
DATABASE = "analytics"
SCHEMA = "public"

TABLES = [
    {
        "name": "customers",
        "description": "Customer master records.",
        "columns": [
            {"name": "id", "dataType": "BIGINT", "description": "Surrogate key."},
            {"name": "email", "dataType": "VARCHAR", "dataLength": 255},
            {"name": "created_at", "dataType": "TIMESTAMP"},
        ],
    },
    {
        "name": "orders",
        "description": "Placed orders, one row per order.",
        "columns": [
            {"name": "id", "dataType": "BIGINT"},
            {"name": "customer_id", "dataType": "BIGINT", "description": "FK to customers.id."},
            {"name": "amount", "dataType": "DECIMAL"},
            {"name": "status", "dataType": "VARCHAR", "dataLength": 32},
        ],
    },
    {
        "name": "events",
        "description": "Raw product analytics events.",
        "columns": [
            {"name": "event_id", "dataType": "VARCHAR", "dataLength": 64},
            {"name": "user_id", "dataType": "BIGINT"},
            {"name": "event_type", "dataType": "VARCHAR", "dataLength": 64},
            {"name": "payload", "dataType": "JSON"},
        ],
    },
    {
        "name": "customer_orders",
        "description": "Per-customer order rollup derived from customers and orders.",
        "columns": [
            {"name": "customer_id", "dataType": "BIGINT", "description": "FK to customers.id."},
            {"name": "customer_email", "dataType": "VARCHAR", "dataLength": 255},
            {"name": "order_count", "dataType": "BIGINT"},
            {"name": "total_amount", "dataType": "DECIMAL"},
        ],
    },
    {
        "name": "user_activity",
        "description": "Per-user activity rollup derived from events and customers.",
        "columns": [
            {"name": "user_id", "dataType": "BIGINT"},
            {"name": "customer_email", "dataType": "VARCHAR", "dataLength": 255},
            {"name": "event_count", "dataType": "BIGINT"},
            {"name": "last_event_at", "dataType": "TIMESTAMP"},
        ],
    },
]


# Table-level lineage edges (upstream -> downstream) with optional
LINEAGE = [
    {
        "from": "customers",
        "to": "customer_orders",
        "columns": [
            (["customers.id"], "customer_orders.customer_id"),
            (["customers.email"], "customer_orders.customer_email"),
        ],
    },
    {
        "from": "orders",
        "to": "customer_orders",
        "columns": [
            (["orders.id"], "customer_orders.order_count"),
            (["orders.amount"], "customer_orders.total_amount"),
        ],
    },
    {
        "from": "events",
        "to": "user_activity",
        "columns": [
            (["events.user_id"], "user_activity.user_id"),
            (["events.event_id"], "user_activity.event_count"),
        ],
    },
    {
        "from": "customers",
        "to": "user_activity",
        "columns": [
            (["customers.email"], "user_activity.customer_email"),
        ],
    },
]


def main() -> None:
    base = os.environ.get("OPENMETADATA_HOST_PORT", "http://127.0.0.1:8585").rstrip("/")
    email = os.environ.get("OPENMETADATA_ADMIN_EMAIL", "admin@open-metadata.org")
    password = os.environ.get("OPENMETADATA_ADMIN_PASSWORD", "admin")

    token = login(base, email, password)

    put(
        base,
        token,
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
    )
    put(base, token, "databases", {"name": DATABASE, "service": SERVICE})
    put(
        base,
        token,
        "databaseSchemas",
        {"name": SCHEMA, "database": f"{SERVICE}.{DATABASE}"},
    )

    schema_fqn = f"{SERVICE}.{DATABASE}.{SCHEMA}"
    table_ids: dict[str, str] = {}
    for table in TABLES:
        created = put(base, token, "tables", {**table, "databaseSchema": schema_fqn})
        table_ids[table["name"]] = created["id"]
        print(f"Seeded table: {schema_fqn}.{table['name']}")

    for edge in LINEAGE:
        column_lineage = [
            {
                "fromColumns": [f"{schema_fqn}.{c}" for c in from_cols],
                "toColumn": f"{schema_fqn}.{to_col}",
            }
            for from_cols, to_col in edge.get("columns", [])
        ]
        put(
            base,
            token,
            "lineage",
            {
                "edge": {
                    "fromEntity": {"id": table_ids[edge["from"]], "type": "table"},
                    "toEntity": {"id": table_ids[edge["to"]], "type": "table"},
                    "lineageDetails": {"columnsLineage": column_lineage},
                }
            },
        )
        print(f"Seeded lineage: {edge['from']} -> {edge['to']}")

    print(f"OpenMetadata: {base}")
    print(
        f"Seeded {len(TABLES)} tables and {len(LINEAGE)} lineage edges "
        f"under service '{SERVICE}'."
    )


if __name__ == "__main__":
    try:
        main()
    except requests.HTTPError as err:
        print(f"OpenMetadata seeding failed: {err} -> {err.response.text}", file=sys.stderr)
        sys.exit(1)
