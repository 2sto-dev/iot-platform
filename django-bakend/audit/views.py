from rest_framework import generics
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import IsAuthenticated

from tenants.permissions import TenantRolePermission
from .models import AuditLog
from .serializers import AuditLogSerializer

_ALLOWED_ROLES = {"OWNER", "ADMIN"}


class AuditLogListView(generics.ListAPIView):
    """GET /api/v1/audit/ — returns audit log for the current tenant.

    Supports query params: from=<ISO8601>, to=<ISO8601>, actor=<username>
    Restricted to OWNER and ADMIN roles.
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = AuditLogSerializer

    def get_queryset(self):
        role = getattr(self.request, "role", None)
        user = self.request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            if role not in _ALLOWED_ROLES:
                raise PermissionDenied("Only OWNER and ADMIN can access the audit log.")

        tenant = getattr(self.request, "tenant", None)
        if tenant is None:
            raise PermissionDenied("No active tenant context.")

        qs = AuditLog.objects.filter(tenant=tenant).select_related("actor")

        from_ts = self.request.query_params.get("from")
        to_ts = self.request.query_params.get("to")
        actor_name = self.request.query_params.get("actor")

        if from_ts:
            qs = qs.filter(ts__gte=from_ts)
        if to_ts:
            qs = qs.filter(ts__lte=to_ts)
        if actor_name:
            qs = qs.filter(actor__username=actor_name)

        return qs
