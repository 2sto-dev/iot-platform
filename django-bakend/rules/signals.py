"""Invalidate Redis rule cache when a Rule is saved or deleted."""
import logging

from django.db.models.signals import post_delete, post_save
from django.dispatch import receiver

from .models import Rule

logger = logging.getLogger(__name__)


def _invalidate(tenant_id):
    try:
        from django.conf import settings
        import redis as _redis
        url = getattr(settings, "REDIS_URL", None)
        if not url:
            return
        rdb = _redis.Redis.from_url(url, socket_connect_timeout=1, socket_timeout=1)
        key = f"rules:v1:{tenant_id}"
        rdb.delete(key)
        logger.debug("rules: cache invalidated for tenant %s", tenant_id)
    except Exception as exc:
        logger.warning("rules: cache invalidation failed: %s", exc)


@receiver(post_save, sender=Rule)
def on_rule_save(sender, instance, **kwargs):
    _invalidate(instance.tenant_id)


@receiver(post_delete, sender=Rule)
def on_rule_delete(sender, instance, **kwargs):
    _invalidate(instance.tenant_id)
