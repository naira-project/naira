#!/bin/sh
NAMESERVER="$(grep '^nameserver' /etc/resolv.conf | head -1 | awk '{print $2}')"
export NAMESERVER
exec /docker-entrypoint.sh "$@"
