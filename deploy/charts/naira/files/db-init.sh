#!/bin/sh
# Usage: db-init.sh <name>... ; the password of the Nth name is in DB_PASSWORD_<N-1>.
# Ensures a login role and a database per name, owned by that role.
set -eu
until pg_isready -q; do echo "waiting for $PGHOST:$PGPORT"; sleep 2; done
ensure() {
  psql -v ON_ERROR_STOP=1 -q -d postgres -v name="$1" -v password="$2" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN', :'name')
  WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'name')\gexec
ALTER ROLE :"name" WITH LOGIN PASSWORD :'password';
SELECT format('CREATE DATABASE %I OWNER %I', :'name', :'name')
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'name')\gexec
-- A database created earlier by another role would otherwise lock this one out.
ALTER DATABASE :"name" OWNER TO :"name";
REVOKE CONNECT ON DATABASE :"name" FROM PUBLIC;
SQL
  echo "ensured database and role $1"
}
i=0
for name in "$@"; do
  password=$(printenv "DB_PASSWORD_$i")
  ensure "$name" "$password"
  i=$((i + 1))
done
