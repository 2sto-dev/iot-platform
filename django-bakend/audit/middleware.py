"""AuditMiddleware — logs tenant-scoped CRUD requests to AuditLog.

Fires after the response is produced so it never blocks the view.
Only records requests where:
  - HTTP method is POST, PUT, PATCH, or DELETE
  - request.tenant is set (tenant-scoped endpoint)
  - response status < 500 (no point logging server errors as CRUD actions)
  - path is not in the skip list (admin, token, mqtt, provisioning/activate)
"""
import logging

from .models import AuditLog

logger = logging.getLogger(__name__)

_SKIP_PREFIXES = (
    "/admin/",
    "/api/token/",
    "/api/mqtt/",
    "/api/provisioning/activate",
)

_METHOD_TO_ACTION = {
    "POST": AuditLog.Action.CREATE,
    "PUT": AuditLog.Action.UPDATE,
    "PATCH": AuditLog.Action.UPDATE,
    "DELETE": AuditLog.Action.DELETE,
}


def _resource_type_from_path(path: str) -> str:
    """Best-effort: extract resource type from the URL path segment after /api/."""
    parts = [p for p in path.strip("/").split("/") if p]
    if len(parts) >= 2 and parts[0] in ("api", "v1"):
        return parts[1]
    if parts:
        return parts[0]
    return "unknown"


def _resource_id_from_path(path: str) -> str:
    """Extract last numeric or slug segment as the resource ID, if present."""
    parts = [p for p in path.strip("/").split("/") if p]
    if len(parts) >= 2:
        candidate = parts[-1]
        if candidate not in (
            "credentials", "rotate", "shadow", "reported", "ack",
            "commands", "activate", "revoke",
        ):
            return candidate
    return ""


class AuditMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response

    def __call__(self, request):
        response = self.get_response(request)

        if request.method not in _METHOD_TO_ACTION:
            return response

        path = request.path
        if any(path.startswith(p) for p in _SKIP_PREFIXES):
            return response

        tenant = getattr(request, "tenant", None)
        if tenant is None:
            return response

        if response.status_code >= 500:
            return response

        user = getattr(request, "user", None)
        actor = user if (user and user.is_authenticated) else None

        ip = (
            request.META.get("HTTP_X_FORWARDED_FOR", "").split(",")[0].strip()
            or request.META.get("REMOTE_ADDR")
        ) or None

        try:
            AuditLog.objects.create(
                tenant=tenant,
                actor=actor,
                action=_METHOD_TO_ACTION[request.method],
                resource_type=_resource_type_from_path(path),
                resource_id=_resource_id_from_path(path),
                metadata={"path": path, "status": response.status_code},
                ip=ip,
            )
        except Exception:
            logger.exception("AuditMiddleware: failed to write log")

        return response
