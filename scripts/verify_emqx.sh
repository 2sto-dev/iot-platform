#!/usr/bin/env bash
set -euo pipefail

echo "== EMQX service =="
sudo systemctl status emqx --no-pager -l || true
echo

echo "== Listeners =="
sudo ss -lntp | awk 'NR==1 || $4 ~ /:(1883|8883|8083|8084|18083)$/'
echo

echo "== UFW =="
sudo ufw status numbered || true
echo

echo "== EMQX ctl =="
sudo emqx ctl status || true
sudo emqx ctl listeners list || true
echo

echo "== Local MQTT pub/sub test (anonymous) =="
# Într-o sesiune separată poți rula:
#   mosquitto_sub -h 127.0.0.1 -t "diag/test" -C 1 &
# și apoi:
mosquitto_pub -h 127.0.0.1 -t "diag/test" -m "ok-$(date +%s)" || true
echo "Verifică dacă subscriber-ul a primit mesajul."
echo

echo "== (Opțional) Test ACL HTTP către Django =="
# Înlocuiește DJ_HOST/PORT dacă endpoint-ul e disponibil.
DJ=${DJ:-"http://172.16.0.105:8000/api/mqtt-acl/"}
curl -sS -X POST "$DJ" -H 'Content-Type: application/json' \
  -d '{"clientid":"DEV1","username":"","topic":"tenants/1/devices/DEV1/up/telemetry","action":"publish"}' || true
echo
