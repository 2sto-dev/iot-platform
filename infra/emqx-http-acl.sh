#!/usr/bin/env bash
# Faza 2.1 — Re-scrie /etc/emqx/emqx.conf cu HTTP Auth + ACL hook.
# Utilizat pentru reinstalare / reconfigurare EMQX pe emqx-1 (172.16.0.103).
#
# Precondiție: EMQX 5.8.6 instalat via dpkg (infra/emqx-install.sh).
#
# Usage (de pe mașina locală cu acces SSH la emqx-1):
#   HOOK_SECRET=<valoare din django-bakend/.env MQTT_HOOK_SECRET> \
#   DJANGO_URL=http://172.16.0.105:8000 \
#   SSH_KEY=~/.ssh/id_ed25519_new \
#   bash infra/emqx-http-acl.sh
#
# Sau direct pe emqx-1:
#   HOOK_SECRET=... DJANGO_URL=... bash emqx-http-acl.sh

set -euo pipefail

DJANGO_URL="${DJANGO_URL:-http://172.16.0.105:8000}"
HOOK_SECRET="${HOOK_SECRET:?HOOK_SECRET must be set (see django-bakend/.env MQTT_HOOK_SECRET)}"
SSH_KEY="${SSH_KEY:-~/.ssh/id_ed25519_new}"
EMQX_HOST="c@172.16.0.103"

echo "=== EMQX HTTP Auth/ACL config ==="
echo "  Django URL  : $DJANGO_URL"
echo "  EMQX host   : $EMQX_HOST"
echo ""

ssh -i "$SSH_KEY" "$EMQX_HOST" "sudo tee /etc/emqx/emqx.conf > /dev/null" << CONF
## /etc/emqx/emqx.conf — generat de infra/emqx-http-acl.sh
## Config precedence: etc/base.hocon < cluster.hocon < emqx.conf < env vars

node {
  name = "emqx@127.0.0.1"
  cookie = "emqxsecretcookie"
  data_dir = "/var/lib/emqx"
}

cluster {
  name = emqxcl
  discovery_strategy = manual
}

dashboard {
  listeners {
    http.bind = 18083
  }
}

listeners.tcp.default {
  bind = "0.0.0.0:1883"
  max_connections = 1024000
}

authentication = [
  {
    mechanism = password_based
    backend = http
    enable = true

    method = post
    url = "${DJANGO_URL}/api/mqtt/auth/"
    headers {
      "Content-Type"  = "application/json"
      "X-Hook-Secret" = "${HOOK_SECRET}"
    }
    body {
      username = "\${username}"
      password = "\${password}"
      clientid = "\${clientid}"
    }
    connect_timeout   = 5s
    request_timeout   = 5s
    pool_size         = 8
    enable_pipelining = 100
    ssl.enable        = false
  }
]

authorization {
  no_match    = deny
  deny_action = disconnect

  cache {
    enable   = true
    max_size = 1024
    ttl      = 60s
  }

  sources = [
    {
      type   = http
      enable = true

      method = post
      url    = "${DJANGO_URL}/api/mqtt/acl/"
      headers {
        "Content-Type"  = "application/json"
        "X-Hook-Secret" = "${HOOK_SECRET}"
      }
      body {
        username = "\${username}"
        clientid = "\${clientid}"
        topic    = "\${topic}"
        action   = "\${action}"
      }
      connect_timeout   = 5s
      request_timeout   = 5s
      pool_size         = 8
      enable_pipelining = 100
      ssl.enable        = false
    }
  ]
}
CONF

echo "[OK] Config scris pe $EMQX_HOST"

ssh -i "$SSH_KEY" "$EMQX_HOST" "
  sudo emqx check_config && echo '[OK] Config valid'
  sudo systemctl restart emqx
  sleep 4
  sudo emqx ping && echo '[OK] EMQX running'
  ss -tlnp | grep 1883 && echo '[OK] Port 1883 open'
"
