#!/usr/bin/env bash
# Configurare Redis pe redis-1 (172.16.0.108).
# Instalează Redis dacă lipsește, configurează bind + requirepass.
# Rulează de pe mașina locală:
#   REDIS_PASS=egoqwedc/12 bash infra/redis-configure.sh

set -euo pipefail

SSH_HOST="c@172.16.0.108"
SSH_KEY="${SSH_KEY:-~/.ssh/id_ed25519}"   # redis-1 acceptă id_ed25519 default
REDIS_PASS="${REDIS_PASS:-egoqwedc/12}"
BIND_IP="172.16.0.108"

echo "=== Redis configure on $SSH_HOST ==="
echo "  Bind  : 127.0.0.1 $BIND_IP"
echo "  Port  : 6379"
echo ""

ssh "$SSH_HOST" "
  set -e

  # Instalează Redis dacă lipsește
  if ! command -v redis-server &>/dev/null; then
    echo '[..] Install redis-server...'
    sudo apt-get install -y redis-server
  fi

  # Scrie config curat
  echo '[..] Write /etc/redis/redis.conf...'
  sudo tee /etc/redis/redis.conf > /dev/null <<CFG
# Redis config — generat de infra/redis-configure.sh
bind 127.0.0.1 $BIND_IP
port 6379
protected-mode yes
requirepass $REDIS_PASS

# Persistență minimă
save 900 1
save 300 10
save 60  10000
appendonly no

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log
CFG

  echo '[..] Restart redis-server...'
  sudo systemctl enable redis-server
  sudo systemctl restart redis-server
  sleep 2
  echo '[..] Test ping...'
  redis-cli -h $BIND_IP -p 6379 -a '$REDIS_PASS' --no-auth-warning ping
  echo '[OK] Redis configured and running'
"
