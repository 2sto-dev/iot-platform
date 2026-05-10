"""OTA service views (Faza 3.5).

Staged rollout flow:
  1. Operator creează firmware (POST /api/ota/firmware/)
  2. Operator pornește rollout (POST /api/ota/rollouts/) → status=canary
     - Selectează canary_percent% din device-urile eligibile; publică MQTT down/ota
  3. Operator avansează (POST /api/ota/rollouts/{id}/advance/)
     - Verifică error_rate; dacă > threshold → rollback automat
     - Altfel: adaugă step_percent% device-uri noi → publish MQTT
     - La 100% → status=complete
  4. Rollback manual (POST /api/ota/rollouts/{id}/rollback/)

Device-urile raportează status pe up/ota → Go worker → PATCH /api/ota/devices/{serial}/status/
"""
import json
import logging
import random

from django.conf import settings
from django.shortcuts import get_object_or_404
from django.utils import timezone
from rest_framework import serializers as drf_serializers
from rest_framework import status
from rest_framework.exceptions import PermissionDenied, ValidationError
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from rest_framework.views import APIView

from clients.models import Device
from tenants.permissions import TenantRolePermission

from .models import DeviceOTAStatus, Firmware, RolloutPlan
from .serializers import (
    DeviceOTAStatusSerializer,
    FirmwareSerializer,
    RolloutCreateSerializer,
    RolloutPlanSerializer,
)

logger = logging.getLogger(__name__)


def _is_cross_tenant(user):
    return user.is_superuser or user.has_perm("clients.view_device")


def _require_write_role(request):
    role = getattr(request, "role", None)
    if not _is_cross_tenant(request.user) and role not in {"OWNER", "ADMIN"}:
        raise PermissionDenied("Only OWNER or ADMIN can manage OTA.")


def _get_tenant(request):
    tenant = getattr(request, "tenant", None)
    if tenant is None and not _is_cross_tenant(request.user):
        raise PermissionDenied("No active tenant context.")
    return tenant


def _publish_ota_command(device, firmware):
    """Publică comanda OTA pe MQTT (tenants/{tid}/devices/{serial}/down/ota)."""
    broker = getattr(settings, "MQTT_BROKER", "")
    if not broker:
        return
    try:
        import paho.mqtt.publish as mqttpublish
        from clients.mqtt_publisher import _parse_broker
        host, port = _parse_broker(broker)
        topic = f"tenants/{device.tenant_id}/devices/{device.serial_number}/down/ota"
        payload = json.dumps({
            "firmware_id": firmware.id,
            "version": firmware.version,
            "url": firmware.file_url,
            "checksum_sha256": firmware.checksum_sha256,
            "size_bytes": firmware.size_bytes,
        })
        auth = None
        user = getattr(settings, "MQTT_SERVICE_USER", "")
        passwd = getattr(settings, "MQTT_SERVICE_PASS", "")
        if user:
            auth = {"username": user, "password": passwd}
        mqttpublish.single(topic, payload=payload, qos=1, retain=False,
                           hostname=host, port=port, auth=auth)
    except Exception as exc:
        logger.warning("OTA MQTT publish failed for %s: %s", device.serial_number, exc)


def _dispatch_batch(rollout, devices):
    """Creează DeviceOTAStatus + publică MQTT pentru o listă de device-uri."""
    now = timezone.now()
    for device in devices:
        obj, created = DeviceOTAStatus.objects.get_or_create(
            device=device,
            firmware=rollout.firmware,
            defaults={"rollout": rollout, "status": DeviceOTAStatus.Status.SENT, "sent_at": now},
        )
        if not created:
            continue
        _publish_ota_command(device, rollout.firmware)


def _eligible_devices(rollout):
    """Device-uri din tenant eligibile (device_type potrivit, fără succes anterior)."""
    already_sent = DeviceOTAStatus.objects.filter(
        firmware=rollout.firmware
    ).values_list("device_id", flat=True)
    return list(
        Device.objects.filter(
            tenant=rollout.tenant,
            device_type=rollout.firmware.device_type,
        ).exclude(id__in=already_sent)
    )


class FirmwareListCreateView(APIView):
    """GET/POST /api/ota/firmware/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def get(self, request):
        tenant = _get_tenant(request)
        qs = Firmware.objects.filter(tenant=tenant) if tenant else Firmware.objects.all()
        return Response(FirmwareSerializer(qs, many=True).data)

    def post(self, request):
        _require_write_role(request)
        tenant = _get_tenant(request)
        serializer = FirmwareSerializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        firmware = serializer.save(
            tenant=tenant or _resolve_tenant_from_request(request),
            created_by=request.user,
        )
        return Response(FirmwareSerializer(firmware).data, status=status.HTTP_201_CREATED)


def _resolve_tenant_from_request(request):
    if _is_cross_tenant(request.user):
        tenant_id = request.data.get("tenant")
        if tenant_id:
            from tenants.models import Tenant
            return get_object_or_404(Tenant, pk=tenant_id)
    raise ValidationError({"tenant": "Required for cross-tenant accounts."})


class FirmwareDetailView(APIView):
    """GET/DELETE /api/ota/firmware/{id}/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def _get_firmware(self, request, pk):
        tenant = _get_tenant(request)
        if tenant:
            return get_object_or_404(Firmware, pk=pk, tenant=tenant)
        return get_object_or_404(Firmware, pk=pk)

    def get(self, request, pk):
        firmware = self._get_firmware(request, pk)
        return Response(FirmwareSerializer(firmware).data)

    def delete(self, request, pk):
        _require_write_role(request)
        firmware = self._get_firmware(request, pk)
        if hasattr(firmware, "rollout") and firmware.rollout.status in {
            RolloutPlan.Status.CANARY, RolloutPlan.Status.ROLLING
        }:
            raise ValidationError("Cannot delete firmware with an active rollout.")
        firmware.delete()
        return Response(status=status.HTTP_204_NO_CONTENT)


class RolloutListCreateView(APIView):
    """GET/POST /api/ota/rollouts/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def get(self, request):
        tenant = _get_tenant(request)
        qs = RolloutPlan.objects.filter(tenant=tenant).select_related("firmware") if tenant \
            else RolloutPlan.objects.all().select_related("firmware")
        return Response(RolloutPlanSerializer(qs, many=True).data)

    def post(self, request):
        _require_write_role(request)
        tenant = _get_tenant(request)
        serializer = RolloutCreateSerializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        d = serializer.validated_data

        firmware = get_object_or_404(
            Firmware, pk=d["firmware_id"],
            **({"tenant": tenant} if tenant else {}),
        )
        if hasattr(firmware, "rollout"):
            raise ValidationError("Firmware already has a rollout plan.")

        rollout = RolloutPlan.objects.create(
            firmware=firmware,
            tenant=firmware.tenant,
            canary_percent=d["canary_percent"],
            target_percent=d["target_percent"],
            step_percent=d["step_percent"],
            error_threshold=d["error_threshold"],
            status=RolloutPlan.Status.CANARY,
            started_at=timezone.now(),
        )

        eligible = _eligible_devices(rollout)
        n_canary = max(1, int(len(eligible) * d["canary_percent"] / 100))
        canary_devices = random.sample(eligible, min(n_canary, len(eligible)))
        _dispatch_batch(rollout, canary_devices)
        rollout.current_percent = d["canary_percent"]
        rollout.save(update_fields=["current_percent"])

        return Response(RolloutPlanSerializer(rollout).data, status=status.HTTP_201_CREATED)


class RolloutDetailView(APIView):
    """GET /api/ota/rollouts/{id}/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def _get_rollout(self, request, pk):
        tenant = _get_tenant(request)
        if tenant:
            return get_object_or_404(RolloutPlan, pk=pk, tenant=tenant)
        return get_object_or_404(RolloutPlan, pk=pk)

    def get(self, request, pk):
        rollout = self._get_rollout(request, pk)
        return Response(RolloutPlanSerializer(rollout).data)


class RolloutAdvanceView(APIView):
    """POST /api/ota/rollouts/{id}/advance/ — avansează la etapa următoare."""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def post(self, request, pk):
        _require_write_role(request)
        tenant = _get_tenant(request)
        rollout = get_object_or_404(
            RolloutPlan, pk=pk,
            **({"tenant": tenant} if tenant else {}),
        )

        if rollout.status not in {RolloutPlan.Status.CANARY, RolloutPlan.Status.ROLLING}:
            raise ValidationError(f"Cannot advance rollout in status '{rollout.status}'.")

        if rollout.should_auto_rollback():
            rollout.status = RolloutPlan.Status.ROLLED_BACK
            rollout.completed_at = timezone.now()
            rollout.save(update_fields=["status", "completed_at"])
            return Response(
                {"detail": f"Auto-rolled back: error rate {rollout.error_rate:.0%} > threshold {rollout.error_threshold:.0%}"},
                status=status.HTTP_200_OK,
            )

        next_percent = min(rollout.current_percent + rollout.step_percent, rollout.target_percent)
        eligible = _eligible_devices(rollout)
        n_next = max(0, int(len(eligible) * (next_percent - rollout.current_percent) / 100))
        batch = random.sample(eligible, min(n_next, len(eligible)))
        _dispatch_batch(rollout, batch)

        rollout.current_percent = next_percent
        rollout.status = RolloutPlan.Status.COMPLETE if next_percent >= rollout.target_percent \
            else RolloutPlan.Status.ROLLING
        if rollout.status == RolloutPlan.Status.COMPLETE:
            rollout.completed_at = timezone.now()
        rollout.save(update_fields=["status", "current_percent", "completed_at"])
        return Response(RolloutPlanSerializer(rollout).data)


class RolloutPauseView(APIView):
    """POST /api/ota/rollouts/{id}/pause/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def post(self, request, pk):
        _require_write_role(request)
        tenant = _get_tenant(request)
        rollout = get_object_or_404(
            RolloutPlan, pk=pk,
            **({"tenant": tenant} if tenant else {}),
        )
        if rollout.status not in {RolloutPlan.Status.CANARY, RolloutPlan.Status.ROLLING}:
            raise ValidationError(f"Cannot pause rollout in status '{rollout.status}'.")
        rollout.status = RolloutPlan.Status.PAUSED
        rollout.save(update_fields=["status"])
        return Response(RolloutPlanSerializer(rollout).data)


class RolloutRollbackView(APIView):
    """POST /api/ota/rollouts/{id}/rollback/ — rollback manual."""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def post(self, request, pk):
        _require_write_role(request)
        tenant = _get_tenant(request)
        rollout = get_object_or_404(
            RolloutPlan, pk=pk,
            **({"tenant": tenant} if tenant else {}),
        )
        if rollout.status == RolloutPlan.Status.ROLLED_BACK:
            raise ValidationError("Rollout already rolled back.")
        rollout.status = RolloutPlan.Status.ROLLED_BACK
        rollout.completed_at = timezone.now()
        rollout.save(update_fields=["status", "completed_at"])
        return Response(RolloutPlanSerializer(rollout).data)


class DeviceOTAStatusUpdateView(APIView):
    """PATCH /api/ota/devices/{serial}/status/ — device raportează status OTA (service account)."""
    permission_classes = [IsAuthenticated]

    def patch(self, request, serial):
        if not _is_cross_tenant(request.user):
            raise PermissionDenied("Service account required.")

        device = get_object_or_404(Device, serial_number=serial)
        firmware_id = request.data.get("firmware_id")
        new_status = request.data.get("status")
        error_message = request.data.get("error_message", "")

        if not firmware_id or not new_status:
            raise drf_serializers.ValidationError({"detail": "firmware_id and status required."})

        valid = {s.value for s in DeviceOTAStatus.Status}
        if new_status not in valid:
            raise drf_serializers.ValidationError({"status": f"Must be one of {valid}."})

        ota_status = get_object_or_404(DeviceOTAStatus, device=device, firmware_id=firmware_id)
        ota_status.status = new_status
        ota_status.error_message = error_message
        ota_status.save(update_fields=["status", "error_message"])

        if ota_status.rollout.should_auto_rollback():
            rollout = ota_status.rollout
            rollout.status = RolloutPlan.Status.ROLLED_BACK
            rollout.completed_at = timezone.now()
            rollout.save(update_fields=["status", "completed_at"])
            logger.warning(
                "Auto rollback triggered for rollout %d: error_rate=%.2f",
                rollout.id, rollout.error_rate,
            )

        return Response(DeviceOTAStatusSerializer(ota_status).data)


class DeviceOTAHistoryView(APIView):
    """GET /api/devices/{pk}/ota/ — istoricul OTA al unui device."""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def get(self, request, pk):
        tenant = _get_tenant(request)
        if tenant:
            device = get_object_or_404(Device, pk=pk, tenant=tenant)
        else:
            device = get_object_or_404(Device, pk=pk)
        statuses = DeviceOTAStatus.objects.filter(device=device).select_related(
            "firmware"
        ).order_by("-updated_at")
        return Response(DeviceOTAStatusSerializer(statuses, many=True).data)
