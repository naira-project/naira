"""
Seed dummy tables into OpenMetadata via the REST API.

Creates:
  - 1 database service  (PostgreSQL)
  - 1 database          (ecommerce)
  - 1 schema            (public)
  - 3 tables            (users, orders, products)

Usage:
    python seed_dummy_data.py

Environment variables:
    OPENMETADATA_URL       OpenMetadata base URL  (default: http://localhost:8585)
    OPENMETADATA_USERNAME  Login email            (default: admin@open-metadata.org)
    OPENMETADATA_PASSWORD  Login password         (default: admin)
"""

import json
import os
import sys
import urllib.error
import urllib.request


BASE_URL = os.environ.get("OPENMETADATA_URL", "http://localhost:8585").rstrip("/")


# ---------------------------------------------------------------------------
# Auth helpers
# ---------------------------------------------------------------------------

def _login() -> str:
    """Obtain a JWT access token via username/password."""
    import base64
    username = os.environ.get("OPENMETADATA_USERNAME", "admin@open-metadata.org")
    password = os.environ.get("OPENMETADATA_PASSWORD", "admin")
    encoded_password = base64.b64encode(password.encode()).decode()
    url = f"{BASE_URL}/api/v1/users/login"
    body = json.dumps({"email": username, "password": encoded_password}).encode()
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json", "Accept": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())["accessToken"]
    except urllib.error.HTTPError as e:
        msg = e.read().decode(errors="replace")
        raise RuntimeError(f"Login failed (HTTP {e.code}): {msg}") from e


_token: str = ""


def _get_token() -> str:
    global _token
    if not _token:
        _token = _login()
    return _token


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def _headers() -> dict:
    h = {"Content-Type": "application/json", "Accept": "application/json"}
    token = _get_token()
    if token:
        h["Authorization"] = f"Bearer {token}"
    return h


def _request(method: str, path: str, body: dict | None = None) -> dict:
    url = f"{BASE_URL}{path}"
    data = json.dumps(body).encode() if body else None
    req = urllib.request.Request(url, data=data, headers=_headers(), method=method)
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        msg = e.read().decode(errors="replace")
        # 409 Conflict = already exists, treat as success
        if e.code == 409:
            print(f"  [exists] {method} {path}")
            # Re-fetch the resource
            return get(path.split("?")[0])
        raise RuntimeError(f"HTTP {e.code} {method} {url}: {msg}") from e


def get(path: str) -> dict:
    return _request("GET", path)


def post(path: str, body: dict) -> dict:
    return _request("POST", path, body)


def put(path: str, body: dict) -> dict:
    return _request("PUT", path, body)


# ---------------------------------------------------------------------------
# Seed helpers
# ---------------------------------------------------------------------------

def create_database_service(name: str) -> str:
    print(f"Creating database service: {name}")
    resp = put("/api/v1/services/databaseServices", {
        "name": name,
        "serviceType": "Postgres",
        "connection": {
            "config": {
                "type": "Postgres",
                "scheme": "postgresql+psycopg2",
                "username": "demo",
                "authType": {"password": "demo"},
                "hostPort": "localhost:5432",
                "database": "ecommerce",
            }
        },
    })
    fqn = resp["fullyQualifiedName"]
    print(f"  -> {fqn}")
    return fqn


def create_database(service_fqn: str, db_name: str) -> str:
    print(f"Creating database: {db_name}")
    resp = put("/api/v1/databases", {
        "name": db_name,
        "service": service_fqn,
    })
    fqn = resp["fullyQualifiedName"]
    print(f"  -> {fqn}")
    return fqn


def create_schema(db_fqn: str, schema_name: str) -> str:
    print(f"Creating schema: {schema_name}")
    resp = put("/api/v1/databaseSchemas", {
        "name": schema_name,
        "database": db_fqn,
    })
    fqn = resp["fullyQualifiedName"]
    print(f"  -> {fqn}")
    return fqn


def create_table(schema_fqn: str, table: dict) -> str:
    print(f"Creating table: {table['name']}")
    resp = put("/api/v1/tables", {
        "name": table["name"],
        "description": table.get("description", ""),
        "databaseSchema": schema_fqn,
        "tableType": "Regular",
        "columns": table["columns"],
        "tags": table.get("tags", []),
    })
    fqn = resp["fullyQualifiedName"]
    print(f"  -> {fqn}")
    return fqn


# ---------------------------------------------------------------------------
# Dummy data definitions
# ---------------------------------------------------------------------------

TABLES = [
    {
        "name": "users",
        "description": "Registered platform users",
        "columns": [
            {"name": "id",         "dataType": "BIGINT",    "description": "Primary key"},
            {"name": "email",      "dataType": "VARCHAR",   "dataLength": 255, "constraint": "UNIQUE",
             "description": "User email address"},
            {"name": "name",       "dataType": "VARCHAR",   "dataLength": 255, "description": "Display name"},
            {"name": "created_at", "dataType": "TIMESTAMP", "description": "Account creation time"},
            {"name": "is_active",  "dataType": "BOOLEAN",   "description": "Whether the account is active"},
        ],
        "tags": [
            {"tagFQN": "PII.Sensitive", "source": "Classification", "labelType": "Manual", "state": "Confirmed"},
        ],
    },
    {
        "name": "orders",
        "description": "Customer purchase orders",
        "columns": [
            {"name": "id",         "dataType": "BIGINT",    "description": "Primary key"},
            {"name": "user_id",    "dataType": "BIGINT",    "description": "FK → users.id"},
            {"name": "total",      "dataType": "DECIMAL",   "description": "Order total in USD"},
            {"name": "status",     "dataType": "VARCHAR",   "dataLength": 50,
             "description": "pending | paid | shipped | cancelled"},
            {"name": "created_at", "dataType": "TIMESTAMP", "description": "Order placement time"},
        ],
        "tags": [],
    },
    {
        "name": "products",
        "description": "Product catalogue",
        "columns": [
            {"name": "id",       "dataType": "BIGINT",  "description": "Primary key"},
            {"name": "sku",      "dataType": "VARCHAR", "dataLength": 100, "constraint": "UNIQUE",
             "description": "Stock-keeping unit"},
            {"name": "name",     "dataType": "VARCHAR", "dataLength": 255, "description": "Product name"},
            {"name": "price",    "dataType": "DECIMAL", "description": "Unit price in USD"},
            {"name": "stock",    "dataType": "INT",     "description": "Available stock count"},
            {"name": "category", "dataType": "VARCHAR", "dataLength": 100, "description": "Product category"},
        ],
        "tags": [],
    },
]


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> None:
    service_fqn = create_database_service("demo-postgres")
    db_fqn      = create_database(service_fqn, "ecommerce")
    schema_fqn  = create_schema(db_fqn, "public")

    for table in TABLES:
        create_table(schema_fqn, table)

    print("\nDone! Tables created under demo-postgres.ecommerce.public.*")
    print(f"Browse at: {BASE_URL}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        sys.exit(1)
