import json
import logging

from rest_framework import generics, status
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from rest_framework.views import APIView

from tenants.permissions import TenantRolePermission
from .models import Rule, RuleExecution
from .serializers import RuleSerializer, RuleExecutionSerializer

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
        raise PermissionDenied("Only OWNER and ADMIN can manage rules.")


class RuleListCreateView(generics.ListCreateAPIView):
    """GET /api/v1/rules/ — list rules; POST — create rule."""
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = RuleSerializer

    def get_queryset(self):
        tenant = _get_tenant(self.request)
        qs = Rule.objects.filter(tenant=tenant)
        enabled = self.request.query_params.get("enabled")
        if enabled is not None:
            qs = qs.filter(enabled=enabled.lower() == "true")
        stream = self.request.query_params.get("stream")
        if stream:
            qs = qs.filter(trigger_stream_pattern__icontains=stream)
        return qs

    def perform_create(self, serializer):
        _require_write(self.request)
        tenant = _get_tenant(self.request)
        serializer.save(tenant=tenant)


class RuleDetailView(generics.RetrieveUpdateDestroyAPIView):
    """GET/PATCH/PUT/DELETE /api/v1/rules/{id}/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = RuleSerializer

    def get_object(self):
        tenant = _get_tenant(self.request)
        try:
            return Rule.objects.get(pk=self.kwargs["pk"], tenant=tenant)
        except Rule.DoesNotExist:
            from rest_framework.exceptions import NotFound
            raise NotFound()

    def update(self, request, *args, **kwargs):
        _require_write(request)
        return super().update(request, *args, **kwargs)

    def destroy(self, request, *args, **kwargs):
        _require_write(request)
        return super().destroy(request, *args, **kwargs)


class RuleToggleView(APIView):
    """PATCH /api/v1/rules/{id}/toggle/ — enable/disable a rule."""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def patch(self, request, pk):
        _require_write(request)
        tenant = _get_tenant(request)
        try:
            rule = Rule.objects.get(pk=pk, tenant=tenant)
        except Rule.DoesNotExist:
            return Response(status=status.HTTP_404_NOT_FOUND)
        rule.enabled = not rule.enabled
        rule.save(update_fields=["enabled"])
        return Response({"id": rule.id, "enabled": rule.enabled})


class RuleExecutionListView(generics.ListAPIView):
    """GET /api/v1/rules/{id}/executions/ — execution history for a rule."""
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = RuleExecutionSerializer

    def get_queryset(self):
        tenant = _get_tenant(self.request)
        try:
            rule = Rule.objects.get(pk=self.kwargs["pk"], tenant=tenant)
        except Rule.DoesNotExist:
            from rest_framework.exceptions import NotFound
            raise NotFound()
        qs = RuleExecution.objects.filter(rule=rule)
        device = self.request.query_params.get("device")
        if device:
            qs = qs.filter(device_serial=device)
        return qs[:200]


class RuleExecutionAllView(generics.ListAPIView):
    """GET /api/v1/rules/executions/ — all executions for tenant."""
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = RuleExecutionSerializer

    def get_queryset(self):
        tenant = _get_tenant(self.request)
        qs = RuleExecution.objects.filter(tenant=tenant).select_related("rule")
        device = self.request.query_params.get("device")
        if device:
            qs = qs.filter(device_serial=device)
        rule_id = self.request.query_params.get("rule")
        if rule_id:
            qs = qs.filter(rule_id=rule_id)
        return qs[:500]


# ── Internal endpoint (called by Go rule-engine) ─────────────────────────────

class InternalRuleListView(APIView):
    """GET /api/internal/rules/?tenant_id=2 — service account only.

    Returns all enabled rules for a tenant (used by Go rule-engine cache miss).
    """
    permission_classes = [IsAuthenticated]

    def get(self, request):
        user = request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            raise PermissionDenied("Service account required.")
        tenant_id = request.query_params.get("tenant_id")
        if not tenant_id:
            return Response({"detail": "tenant_id required."}, status=400)
        rules = Rule.objects.filter(tenant_id=tenant_id, enabled=True)
        return Response(RuleSerializer(rules, many=True).data)


class InternalRuleLogView(APIView):
    """POST /api/internal/rules/log/ — log a rule execution (Go rule-engine)."""
    permission_classes = [IsAuthenticated]

    def post(self, request):
        user = request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            raise PermissionDenied("Service account required.")
        data = request.data
        try:
            rule_id = data.get("rule_id")
            rule = Rule.objects.filter(pk=rule_id).first() if rule_id else None
            RuleExecution.objects.create(
                rule=rule,
                rule_name=data.get("rule_name", ""),
                tenant_id=data["tenant_id"],
                device_serial=data["device_serial"],
                stream=data.get("stream", ""),
                conditions_snapshot=data.get("conditions_snapshot", {}),
                actions_taken=data.get("actions_taken", []),
                status=data.get("status", RuleExecution.Status.TRIGGERED),
                error_message=data.get("error_message", ""),
            )
        except Exception as exc:
            logger.error("rule log: %s", exc)
            return Response({"detail": str(exc)}, status=400)
        return Response({"logged": True}, status=201)
