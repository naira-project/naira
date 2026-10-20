#!/bin/sh
# Usage: db-init.sh <name>... ; the password of the Nth name is in DB_PASSWORD_<N-1>.
# Ensures a login role and a database per name, owned by that role.
set -eu
tries=0
until pg_isready -q; do
  tries=$((tries + 1))
  if [ "$tries" -ge "$DB_WAIT_RETRIES" ]; then
    # pg_isready reports a TLS mismatch as no response too.
    echo "no server ready at $PGHOST:$PGPORT (sslmode=$PGSSLMODE) after $tries tries" >&2
    exit 1
  fi
  echo "waiting for $PGHOST:$PGPORT"
  sleep 2
done
# The password is read from the environment by psql, so it is not in argv.
ensure() {
  psql -v ON_ERROR_STOP=1 -q -d postgres -v name="$1" -v pwvar="$2" <<'SQL'
\getenv password :pwvar
SELECT format('CREATE ROLE %I LOGIN', :'name')
  WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'name')\gexec
ALTER ROLE :"name" WITH LOGIN PASSWORD :'password';
-- Owning a database as another role needs SET ROLE on it; a non-superuser
-- admin (e.g. on a managed service) does not get that from CREATEROLE.
SELECT format('GRANT %I TO CURRENT_USER', :'name')
  WHERE NOT pg_has_role(CURRENT_USER, :'name', 'SET')\gexec
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
  printenv "DB_PASSWORD_$i" >/dev/null
  ensure "$name" "DB_PASSWORD_$i"
  i=$((i + 1))
done
