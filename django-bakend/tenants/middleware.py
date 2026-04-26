"""Decodes the JWT (if present) to expose tenant context on the request.

Doesn't enforce authentication — DRF's JWTAuthentication does that. This middleware
just populates request.tenant_id / tenant_slug / role early so managers, permissions
and views can read them without re-decoding the token.
"""
import jwt
from django.conf import settings


class TenantMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response
        self.signing_key = settings.SIMPLE_JWT["SIGNING_KEY"]
        self.algorithm = settings.SIMPLE_JWT.get("ALGORITHM", "HS256")

    def __call__(self, request):
        request.tenant_id = None
        request.tenant_slug = None
        request.role = None

        auth = request.META.get("HTTP_AUTHORIZATION", "")
        if auth.startswith("Bearer "):
            token = auth[len("Bearer "):]
            try:
                claims = jwt.decode(token, self.signing_key, algorithms=[self.algorithm])
                request.tenant_id = claims.get("tenant_id")
                request.tenant_slug = claims.get("tenant_slug")
                request.role = claims.get("role")
            except jwt.PyJWTError:
                pass

        return self.get_response(request)
