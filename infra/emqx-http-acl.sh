#!/usr/bin/env bash
# Faza 2.1 — Activare HTTP Auth + ACL hook în EMQX 5.x
# Rulează pe emqx-1 (172.16.0.103) ca user cu sudo (c@emqx-1)
#
# Usage:
#   HOOK_SECRET=<secret> DJANGO_URL=http://172.16.0.105:8000 bash emqx-http-acl.sh
#
# Dacă HOOK_SECRET e gol, verificarea secretului e dezactivată (nu recomandat în prod).
# Dacă DJANGO_URL e gol, folosește default-ul de mai jos.

set -euo pipefail

DJANGO_URL="${DJANGO_URL:-http://172.16.0.105:8000}"
HOOK_SECRET="${HOOK_SECRET:-}"
CONF_DIR="/etc/emqx"
CONF_FILE="$CONF_DIR/emqx.conf"
OVERRIDE="$CONF_DIR/emqx-acl-override.conf"

echo "=== Faza 2.1: EMQX HTTP Auth/ACL ==="
echo "  Django URL : $DJANGO_URL"
echo "  Hook Secret: ${HOOK_SECRET:+(set)}"
echo ""

# Creează fișierul de override (EMQX 5.x merges conf files la pornire)
sudo tee "$OVERRIDE" > /dev/null <<CONF
## HTTP Authenticator — verifică device-ul la CONNECT
authentication = [
  {
    mechanism = password_based
    backend = http
    enable = true

    method = post
    url = "${DJANGO_URL}/api/mqtt/auth/"
    body {
      username = "\${username}"
      password = "\${password}"
      clientid = "\${clientid}"
    }
    headers {
      "Content-Type"  = "application/json"
      "X-Hook-Secret" = "${HOOK_SECRET}"
    }
    connect_timeout    = 5s
    request_timeout    = 5s
    pool_size          = 8
    enable_pipelining  = 100
    ssl.enable         = false
  }
]

## HTTP Authorizer — verifică PUBLISH/SUBSCRIBE la nivel de topic
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
      body {
        username = "\${username}"
        clientid = "\${clientid}"
        topic    = "\${topic}"
        action   = "\${action}"
      }
      headers {
        "Content-Type"  = "application/json"
        "X-Hook-Secret" = "${HOOK_SECRET}"
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

echo "[OK] Scris $OVERRIDE"

# Adaugă include în emqx.conf dacă nu există deja
if ! grep -q "emqx-acl-override.conf" "$CONF_FILE"; then
  echo "" | sudo tee -a "$CONF_FILE" > /dev/null
  echo "include \"$OVERRIDE\"" | sudo tee -a "$CONF_FILE" > /dev/null
  echo "[OK] Include adăugat în $CONF_FILE"
else
  echo "[SKIP] Include deja prezent în $CONF_FILE"
fi

# Restart EMQX
echo "[..] Restart EMQX..."
sudo systemctl restart emqx
sleep 3
sudo systemctl status emqx --no-pager | head -5

echo ""
echo "=== Done ==="
echo "Testează cu:"
echo "  curl -s -X POST $DJANGO_URL/api/mqtt/auth/ -H 'Content-Type: application/json' -d '{\"username\":\"SERIAL\",\"password\":\"\",\"clientid\":\"SERIAL\"}'"
