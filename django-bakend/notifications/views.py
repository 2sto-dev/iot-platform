import logging

from rest_framework import generics, status
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from rest_framework.views import APIView

from tenants.permissions import TenantRolePermission
from .models import NotificationChannel, NotificationEvent
from .sender import send_async
from .serializers import NotificationChannelSerializer, NotificationEventSerializer

logger = logging.getLogger(__name__)
_WRITE_ROLES = {"OWNER", "ADMIN"}


def _get_tenant(request):
    tenant = getattr(request, "tenant", None)
    if tenant is None:
        raise PermissionDenied("No active tenant context.")
    return tenant


def _require_write(request):
    user = request.user
    if user.is_superuser or user.has_perm("clients.view_device"):
        return
    if getattr(request, "role", None) not in _WRITE_ROLES:
        raise PermissionDenied("Only OWNER and ADMIN can manage notification channels.")


class ChannelListCreateView(generics.ListCreateAPIView):
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = NotificationChannelSerializer

    def get_queryset(self):
        return NotificationChannel.objects.filter(tenant=_get_tenant(self.request))

    def perform_create(self, serializer):
        _require_write(self.request)
        serializer.save(tenant=_get_tenant(self.request))


class ChannelDetailView(generics.RetrieveUpdateDestroyAPIView):
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = NotificationChannelSerializer

    def get_object(self):
        tenant = _get_tenant(self.request)
        try:
            return NotificationChannel.objects.get(pk=self.kwargs["pk"], tenant=tenant)
        except NotificationChannel.DoesNotExist:
            from rest_framework.exceptions import NotFound
            raise NotFound()

    def update(self, request, *args, **kwargs):
        _require_write(request)
        return super().update(request, *args, **kwargs)

    def destroy(self, request, *args, **kwargs):
        _require_write(request)
        return super().destroy(request, *args, **kwargs)


class ChannelTestView(APIView):
    """POST /api/v1/notifications/channels/{id}/test/ — send test notification."""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def post(self, request, pk):
        _require_write(request)
        tenant = _get_tenant(request)
        try:
            channel = NotificationChannel.objects.get(pk=pk, tenant=tenant)
        except NotificationChannel.DoesNotExist:
            return Response(status=status.HTTP_404_NOT_FOUND)
        event = NotificationEvent.objects.create(
            channel=channel,
            channel_name=channel.name,
            title="Test notification",
            body=f"This is a test from IoT Platform for channel '{channel.name}'.",
            context={"test": True, "channel_id": channel.id},
        )
        send_async(event.id)
        return Response({"event_id": event.id, "status": "queued"})


class EventListView(generics.ListAPIView):
    """GET /api/v1/notifications/events/ — event history for tenant."""
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = NotificationEventSerializer

    def get_queryset(self):
        tenant = _get_tenant(self.request)
        qs = (
            NotificationEvent.objects
            .filter(channel__tenant=tenant)
            .select_related("channel")
        )
        ch = self.request.query_params.get("channel")
        if ch:
            qs = qs.filter(channel_id=ch)
        st = self.request.query_params.get("status")
        if st:
            qs = qs.filter(status=st)
        return qs[:500]


# ── Internal endpoint (called by Go rule-engine) ─────────────────────────────

class InternalNotifyView(APIView):
    """POST /api/internal/notifications/trigger/ — service account only.

    Go rule-engine calls this when a 'notify' action fires.
    Creates a NotificationEvent and dispatches async.
    """
    permission_classes = [IsAuthenticated]

    def post(self, request):
        user = request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            raise PermissionDenied("Service account required.")
        data = request.data
        channel_id = data.get("channel_id")
        try:
            channel = NotificationChannel.objects.get(pk=channel_id)
        except NotificationChannel.DoesNotExist:
            return Response({"detail": f"Channel {channel_id} not found."}, status=404)
        if not channel.enabled:
            return Response({"detail": "Channel disabled."}, status=200)
        event = NotificationEvent.objects.create(
            channel=channel,
            channel_name=channel.name,
            rule_execution_id=data.get("rule_execution_id"),
            title=data.get("title", ""),
            body=data.get("body", ""),
            context=data.get("context", {}),
        )
        send_async(event.id)
        return Response({"event_id": event.id}, status=201)
