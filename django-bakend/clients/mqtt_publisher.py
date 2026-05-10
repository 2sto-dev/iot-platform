"""Publică shadow delta ca retained message MQTT (Faza 3.4).

Mecanismul retained message rezolvă ambele cerințe din plan:
  - 3.4a: device conectat → subscribe down/shadow → broker livrează automat ultima retained
  - 3.4b: desired changes → publică retained → device online primește imediat; offline îl
          primește la reconectare

Dacă MQTT_BROKER e gol sau brokerul e indisponibil: no-op cu log warning (nu ridică excepție).
"""
import json
import logging

from django.conf import settings

logger = logging.getLogger(__name__)


def _parse_broker(broker_str: str):
    """Parsează 'tcp://host:port' sau 'host:port' în (host, port)."""
    s = broker_str.strip()
    for prefix in ("tcp://", "ssl://", "ws://", "wss://"):
        if s.startswith(prefix):
            s = s[len(prefix):]
    if ":" in s:
        host, port_str = s.rsplit(":", 1)
        try:
            return host, int(port_str)
        except ValueError:
            pass
    return s, 1883


def publish_shadow_delta(device, delta: dict) -> None:
    """Publică delta ca retained message pe tenants/{tid}/devices/{serial}/down/shadow.

    delta = {} → publică payload gol reținut (device-ul știe că e sincronizat).
    """
    broker = getattr(settings, "MQTT_BROKER", "")
    if not broker:
        return

    try:
        import paho.mqtt.publish as mqttpublish
    except ImportError:
        logger.warning("paho-mqtt nu e instalat; shadow delta push dezactivat")
        return

    host, port = _parse_broker(broker)
    topic = f"tenants/{device.tenant_id}/devices/{device.serial_number}/down/shadow"
    payload = json.dumps(delta) if delta else "{}"

    auth = None
    user = getattr(settings, "MQTT_SERVICE_USER", "")
    passwd = getattr(settings, "MQTT_SERVICE_PASS", "")
    if user:
        auth = {"username": user, "password": passwd}

    try:
        mqttpublish.single(
            topic,
            payload=payload,
            retain=True,
            qos=1,
            hostname=host,
            port=port,
            auth=auth,
        )
        logger.debug("shadow delta published retain topic=%s delta=%s", topic, payload)
    except Exception as exc:
        logger.warning("shadow delta publish failed for %s: %s", device.serial_number, exc)
