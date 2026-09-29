#!/bin/sh
export NAMESERVER=$(grep '^nameserver' /etc/resolv.conf | head -1 | awk '{print $2}')
exec /docker-entrypoint.sh "$@"
