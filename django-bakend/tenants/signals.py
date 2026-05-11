"""Invalidate the membership cache when Memberships or Tenants change.

Without this, a revoked membership stays "valid" for up to TTL_CACHE seconds.
"""
from django.contrib.auth import get_user_model
from django.db.models.signals import post_delete, post_save
from django.dispatch import receiver

from .middleware import invalidate_membership_cache, invalidate_superuser_cache
from .models import Membership, Tenant


@receiver(post_save, sender=Membership)
@receiver(post_delete, sender=Membership)
def _on_membership_change(sender, instance, **kwargs):
    invalidate_membership_cache(user_id=instance.user_id, tenant_id=instance.tenant_id)


@receiver(post_save, sender=Tenant)
def _on_tenant_change(sender, instance, **kwargs):
    invalidate_membership_cache(tenant_id=instance.id)


# Invalidate superuser cache la fiecare save pe modelul User — apăsă pe scenariul
# "demote admin during active session" (TTL-ul de 60s rămâne fallback). Conectarea
# se face explicit (nu via @receiver) pentru că settings.AUTH_USER_MODEL e string.
def _on_user_change(sender, instance, **kwargs):
    invalidate_superuser_cache(user_id=instance.pk)


post_save.connect(_on_user_change, sender=get_user_model())
