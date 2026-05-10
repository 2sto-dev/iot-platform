import hashlib
import secrets

from django.conf import settings
from django.db import models
from django.utils import timezone


class APIKey(models.Model):
    tenant = models.ForeignKey(
        "tenants.Tenant",
        on_delete=models.CASCADE,
        related_name="api_keys",
    )
    created_by = models.ForeignKey(
        settings.AUTH_USER_MODEL,
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name="created_api_keys",
    )
    name = models.CharField(max_length=100)
    prefix = models.CharField(max_length=8, editable=False)
    key_hash = models.CharField(max_length=64, editable=False)
    scopes = models.JSONField(default=list)
    expires_at = models.DateTimeField(null=True, blank=True)
    last_used_at = models.DateTimeField(null=True, blank=True)
    revoked = models.BooleanField(default=False)
    created_at = models.DateTimeField(auto_now_add=True)

    class Meta:
        ordering = ["-created_at"]

    @classmethod
    def generate(cls, tenant, name, created_by=None, scopes=None, expires_at=None):
        """Create a new APIKey and return (instance, plain_key).

        The plain_key is returned only once and never stored.
        """
        plain = secrets.token_urlsafe(32)
        prefix = plain[:8]
        key_hash = hashlib.sha256(plain.encode()).hexdigest()
        instance = cls.objects.create(
            tenant=tenant,
            created_by=created_by,
            name=name,
            prefix=prefix,
            key_hash=key_hash,
            scopes=scopes or [],
            expires_at=expires_at,
        )
        return instance, plain

    def is_valid(self) -> bool:
        if self.revoked:
            return False
        if self.expires_at and self.expires_at < timezone.now():
            return False
        return True

    @staticmethod
    def hash_key(plain: str) -> str:
        return hashlib.sha256(plain.encode()).hexdigest()
