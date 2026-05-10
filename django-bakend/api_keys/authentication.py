"""DRF authentication backend for API keys.

Reads `Authorization: ApiKey <plain_key>` header, hashes it, and looks
up the matching APIKey. Attaches the key's tenant to request.tenant so
that existing TenantRolePermission and queryset filters work as normal.
"""
from django.utils import timezone
from rest_framework import authentication, exceptions

from .models import APIKey


class APIKeyAuthentication(authentication.BaseAuthentication):
    keyword = "ApiKey"

    def authenticate(self, request):
        auth = authentication.get_authorization_header(request).decode()
        if not auth.startswith(self.keyword + " "):
            return None

        plain = auth[len(self.keyword) + 1:].strip()
        if not plain:
            raise exceptions.AuthenticationFailed("Empty API key.")

        key_hash = APIKey.hash_key(plain)
        try:
            key = (
                APIKey.objects
                .select_related("tenant", "created_by")
                .get(key_hash=key_hash)
            )
        except APIKey.DoesNotExist:
            raise exceptions.AuthenticationFailed("Invalid API key.")

        if not key.is_valid():
            raise exceptions.AuthenticationFailed("API key revoked or expired.")

        # Stamp last_used_at without triggering auto_now fields on other columns.
        APIKey.objects.filter(pk=key.pk).update(last_used_at=timezone.now())

        # Attach tenant context so existing permission classes work unchanged.
        request.tenant = key.tenant
        request.tenant_id = key.tenant_id
        request.tenant_slug = key.tenant.slug
        request.role = "OWNER"  # API keys carry full tenant owner privileges
        request.api_key = key

        # Return (user, auth) — use created_by as the user when available.
        user = key.created_by
        return (user, key)

    def authenticate_header(self, request):
        return self.keyword
