"""Publish device-cache-invalidate notifications to Redis when a Device changes.

Format mesaj: {"serial": "<serial_number>"}
Special: "" payload = invalidate-all (re-fetch).

Go-ul (internal/cache.SubscribeInvalidations) ascultă pe canalul `device-cache-invalidate`
și șterge entry-ul din Redis pe save/delete → propagare <1s a schimbărilor de tenant.

Faza 2.4. Redis e opțional: dacă REDIS_URL nu e setat, semnalele devin no-op (logging la
debug nivel).
"""
import json
import logging

from django.conf import settings
from django.db.models.signals import post_delete, post_save
from django.dispatch import receiver

from .models import Device

logger = logging.getLogger(__name__)

INVALIDATE_CHANNEL = "device-cache-invalidate"

_redis_client = None


def _get_redis():
    global _redis_client
    if _redis_client is not None:
        return _redis_client
    url = getattr(settings, "REDIS_URL", None)
    if not url:
        return None
    try:
        import redis  # type: ignore
    except ImportError:
        logger.warning("redis-py nu e instalat; cache invalidation dezactivat")
        return None
    try:
        _redis_client = redis.Redis.from_url(url, socket_connect_timeout=2, socket_timeout=2)
        _redis_client.ping()
        return _redis_client
    except Exception as e:
        logger.warning("Redis indisponibil (%s); cache invalidation dezactivat", e)
        _redis_client = None
        return None


def _publish(serial: str):
    rdb = _get_redis()
    if rdb is None:
        return
    try:
        rdb.publish(INVALIDATE_CHANNEL, json.dumps({"serial": serial}))
    except Exception as e:
        logger.warning("publish invalidation eșuat pentru %s: %s", serial, e)


@receiver(post_save, sender=Device)
def _on_device_save(sender, instance, **kwargs):
    _publish(instance.serial_number)


@receiver(post_delete, sender=Device)
def _on_device_delete(sender, instance, **kwargs):
    _publish(instance.serial_number)
