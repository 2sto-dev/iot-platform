#!/usr/bin/env bash
# Faza 2.7 — Creare bucket-uri InfluxDB cu retention per plan
# Rulează pe VM-ul cu InfluxDB sau local cu acces la API-ul Influx.
#
# Usage (cu credențiale implicite din .env):
#   bash influx-buckets.sh
#
# Override:
#   INFLUX_URL=http://... INFLUX_TOKEN=<token> INFLUX_ORG=<org> bash influx-buckets.sh

set -euo pipefail

INFLUX_URL="${INFLUX_URL:-http://db-flux.airweb.ro:8086}"
INFLUX_TOKEN="${INFLUX_TOKEN:-teXchv0yTR4y4lCrrUDam_mo2H8l-OlZM4D7gRVAE80ZEeloRT1kYTjPXFoHsbRmp107O96-4kgNEk1YAzTH3A==}"
INFLUX_ORG="${INFLUX_ORG:-xCore}"

INFLUX_CLI="influx"

echo "=== Faza 2.7: Creare buckets InfluxDB ==="
echo "  URL : $INFLUX_URL"
echo "  Org : $INFLUX_ORG"
echo ""

create_bucket() {
  local name="$1"
  local retention="$2"
  echo -n "  [..] $name (retention=$retention) ... "
  if $INFLUX_CLI bucket list \
      --host "$INFLUX_URL" \
      --token "$INFLUX_TOKEN" \
      --org "$INFLUX_ORG" \
      --name "$name" 2>/dev/null | grep -q "$name"; then
    echo "SKIP (deja există)"
    return
  fi
  $INFLUX_CLI bucket create \
    --host "$INFLUX_URL" \
    --token "$INFLUX_TOKEN" \
    --org "$INFLUX_ORG" \
    --name "$name" \
    --retention "$retention"
  echo "OK"
}

# free: 7 zile
create_bucket "iot-free"       "168h"
# pro: 90 zile
create_bucket "iot-pro"        "2160h"
# enterprise: 2 ani (730 zile)
create_bucket "iot-enterprise" "17520h"

echo ""
echo "=== Done ==="
echo ""
echo "Verificare:"
$INFLUX_CLI bucket list \
  --host "$INFLUX_URL" \
  --token "$INFLUX_TOKEN" \
  --org "$INFLUX_ORG" 2>/dev/null | grep -E "iot-(free|pro|enterprise)" || true
