#!/usr/bin/env bash
set -euo pipefail

: "${REDIS_PASSWORD:?Set REDIS_PASSWORD env var}" || exit 0
REDIS_CLI=(redis-cli -a "$REDIS_PASSWORD")

echo "== Redis service =="
sudo systemctl status redis-server --no-pager -l || true
echo

echo "== Bind/requirepass/protected-mode =="
"${REDIS_CLI[@]}" CONFIG GET bind
"${REDIS_CLI[@]}" CONFIG GET requirepass
"${REDIS_CLI[@]}" CONFIG GET protected-mode
echo

echo "== AOF status =="
"${REDIS_CLI[@]}" INFO persistence | egrep 'aof_enabled|aof_rewrite_in_progress|aof_last_bgrewrite_status'
echo

echo "== PING/SET/GET =="
"${REDIS_CLI[@]}" PING
"${REDIS_CLI[@]}" SET health:check $(date +%s) EX 60
"${REDIS_CLI[@]}" GET health:check
echo

echo "== UFW =="
sudo ufw status numbered || true
echo

echo "== Ports =="
sudo ss -lntp | awk 'NR==1 || $4 ~ /:6379$/'
