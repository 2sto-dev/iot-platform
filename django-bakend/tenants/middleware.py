"""Decodes the JWT (if present), validates membership, and exposes tenant context.

After Faza 1.9 hardening:
- Membership re-checked at every request (with in-process TTL cache to avoid DB hit/req).
- If JWT carries `tenant_id` but the user no longer has an active membership → 403.
- `request.tenant` (Tenant instance) is set, in addition to `request.tenant_id` (int).
- `MULTI_TENANT_ENABLED=False` in settings disables the entire multi-tenant gate (kill-switch).
"""
import time

import jwt
from django.conf import settings
from django.http import JsonResponse

from .models import Membership, Tenant


_membership_cache = {}  # (user_id, tenant_id) -> (Tenant or None, expires_at_ts)
_CACHE_TTL = 60  # seconds


def _resolve_tenant(user_id, tenant_id):
    """Return the Tenant instance if (user, tenant) is an active membership; else None.

    Cached in-process for _CACHE_TTL seconds so we don't hit DB on every request.
    Normalizes ids to int so cache keys match between JWT (string) and signals (int).
    """
    if user_id is None or tenant_id is None:
        return None
    try:
        user_id = int(user_id)
        tenant_id = int(tenant_id)
    except (TypeError, ValueError):
        return None
    key = (user_id, tenant_id)
    now = time.monotonic()
    cached = _membership_cache.get(key)
    if cached and cached[1] > now:
        return cached[0]

    membership = (
        Membership.objects
        .filter(
            user_id=user_id,
            tenant_id=tenant_id,
            tenant__status=Tenant.Status.ACTIVE,
        )
        .select_related("tenant")
        .first()
    )
    tenant = membership.tenant if membership else None
    _membership_cache[key] = (tenant, now + _CACHE_TTL)
    return tenant


def invalidate_membership_cache(user_id=None, tenant_id=None):
    """Used by Django signals when a Membership or Tenant changes."""
    if user_id is None and tenant_id is None:
        _membership_cache.clear()
        return
    if user_id is not None:
        try:
            user_id = int(user_id)
        except (TypeError, ValueError):
            user_id = None
    if tenant_id is not None:
        try:
            tenant_id = int(tenant_id)
        except (TypeError, ValueError):
            tenant_id = None
    keys = list(_membership_cache.keys())
    for k in keys:
        u, t = k
        if (user_id is None or u == user_id) and (tenant_id is None or t == tenant_id):
            _membership_cache.pop(k, None)


class TenantMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response
        self.signing_key = settings.SIMPLE_JWT["SIGNING_KEY"]
        self.algorithm = settings.SIMPLE_JWT.get("ALGORITHM", "HS256")
        self.enabled = getattr(settings, "MULTI_TENANT_ENABLED", True)

    def __call__(self, request):
        request.tenant = None
        request.tenant_id = None
        request.tenant_slug = None
        request.role = None

        # Kill-switch: when False, middleware becomes a no-op (legacy mode).
        if not self.enabled:
            return self.get_response(request)

        auth = request.META.get("HTTP_AUTHORIZATION", "")
        if not auth.startswith("Bearer "):
            return self.get_response(request)

        token = auth[len("Bearer "):]
        try:
            claims = jwt.decode(token, self.signing_key, algorithms=[self.algorithm])
        except jwt.PyJWTError:
            # Let DRF auth produce the proper 401; we don't 403 here on bad JWT.
            return self.get_response(request)

        tenant_id = claims.get("tenant_id")
        user_id = claims.get("user_id")

        # No tenant claim → service account / superuser path; let view-level RBAC decide.
        if tenant_id is None:
            return self.get_response(request)

        if user_id is None:
            # Token is malformed (no user_id) but has tenant_id. Refuse.
            return JsonResponse(
                {"detail": "Token missing user_id; cannot validate tenant membership."},
                status=403,
            )

        tenant = _resolve_tenant(user_id, tenant_id)
        if tenant is None:
            return JsonResponse(
                {"detail": "User has no active membership in the requested tenant."},
                status=403,
            )

        request.tenant = tenant
        request.tenant_id = tenant.id
        request.tenant_slug = tenant.slug
        request.role = claims.get("role")

        return self.get_response(request)
