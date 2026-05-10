import json
import logging
import secrets

from django.conf import settings
from django.contrib.auth import authenticate
from django.contrib.auth.hashers import make_password
from django.db import IntegrityError
from django.shortcuts import get_object_or_404
from drf_spectacular.utils import extend_schema, inline_serializer
from rest_framework import generics, serializers as drf_serializers
from rest_framework import serializers as s, status, viewsets
from rest_framework.decorators import action
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import AllowAny, IsAuthenticated
from rest_framework.response import Response
from rest_framework.views import APIView
from rest_framework_simplejwt.views import TokenObtainPairView

from tenants.models import Membership, Tenant
from tenants.permissions import TenantRolePermission
from .models import Device, DeviceCommand, DeviceShadow
from .mqtt_publisher import publish_shadow_delta
from .serializers import (
    DeviceCommandSerializer,
    DeviceSerializer,
    DeviceShadowSerializer,
    DeviceShadowReportedSerializer,
)
from .tokens import CustomTokenObtainPairSerializer

logger = logging.getLogger(__name__)


def _get_redis():
    url = getattr(settings, "REDIS_URL", None)
    if not url:
        return None
    try:
        import redis
        rdb = redis.Redis.from_url(url, socket_connect_timeout=2, socket_timeout=2)
        rdb.ping()
        return rdb
    except Exception as exc:
        logger.warning("Redis indisponibil pentru comenzi: %s", exc)
        return None


def _is_cross_tenant(user):
    """Service accounts and superusers operate cross-tenant."""
    return user.is_superuser or user.has_perm("clients.view_device")


class DeviceViewSet(viewsets.ModelViewSet):
    """CRUD pentru Device cu izolare per-tenant + RBAC.

    Reguli de filtrare (Faza 1.9 hardened + tenant scope fix):
    - User cu request.tenant (orice tip — normal SAU service account/superuser logat pe tenant)
      → DOAR device-urile tenantului. Param-ul `?tenant=` din query e IGNORAT.
    - Cross-tenant FĂRĂ request.tenant (token fără tenant_id) → toate device-urile sau filter
      explicit prin `?tenant=<id>`.
    - Non-cross-tenant fără request.tenant → 403.
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = DeviceSerializer
    queryset = Device.objects.all()

    def get_queryset(self):
        user = self.request.user
        if not user.is_authenticated:
            return Device.objects.none()

        tenant = getattr(self.request, "tenant", None)

        if tenant is not None:
            # Orice user (inclusiv superuser/service account) logat pe un tenant specific
            # vede DOAR device-urile tenantului — anti-leak cross-tenant.
            qs = Device.objects.for_tenant(tenant)
        elif _is_cross_tenant(user):
            # Cross-tenant fără tenant context (token global service-to-service):
            # poate folosi `?tenant=<id>` pentru filtrare explicită.
            qs = Device.objects.all()
            tenant_filter = self.request.query_params.get("tenant")
            if tenant_filter:
                qs = qs.filter(tenant_id=tenant_filter)
        else:
            raise PermissionDenied("No active tenant context.")

        username = self.request.query_params.get("username")
        if username:
            qs = qs.filter(client__username=username)
        return qs

    def perform_create(self, serializer):
        user = self.request.user
        tenant = getattr(self.request, "tenant", None)
        try:
            # Dacă există context de tenant (din JWT), îl folosim — funcționează pentru
            # useri normali și pentru superuseri logați pe un tenant specific.
            if tenant is not None:
                serializer.save(tenant=tenant, client=user)
                return
            # Fără tenant în JWT: doar service accounts / superuseri pot crea, și trebuie să
            # specifice tenant-ul în payload.
            if _is_cross_tenant(user):
                if not serializer.validated_data.get("tenant"):
                    raise drf_serializers.ValidationError(
                        {"tenant": "Cross-tenant accounts must specify tenant in payload."}
                    )
                save_kwargs = {}
                if not serializer.validated_data.get("client"):
                    save_kwargs["client"] = user
                serializer.save(**save_kwargs)
                return
            raise drf_serializers.ValidationError({"tenant": "No tenant context in token."})
        except IntegrityError as e:
            logger.error("Device create IntegrityError: %s", e)
            raise drf_serializers.ValidationError(
                {"serial_number": "A device with this serial number already exists for this tenant."}
            ) from e


    @action(detail=True, methods=["post"], url_path="credentials/rotate")
    def rotate_credentials(self, request, pk=None):
        """POST /api/devices/{id}/credentials/rotate/ — generează parolă MQTT nouă.

        Returnează parola plain o singură dată; ulterior nu mai poate fi recuperată.
        Roluri permise: OWNER, ADMIN (și cross-tenant accounts).
        """
        device = self.get_object()
        role = getattr(request, "role", None)
        if not _is_cross_tenant(request.user) and role not in {"OWNER", "ADMIN"}:
            raise PermissionDenied("Only OWNER or ADMIN can rotate device credentials.")
        plain = secrets.token_urlsafe(24)
        device.mqtt_password_hash = make_password(plain, hasher="bcrypt_sha256")
        device.save(update_fields=["mqtt_password_hash"])
        return Response({"serial_number": device.serial_number, "mqtt_password": plain})


class DeviceShadowView(generics.RetrieveUpdateAPIView):
    """GET/PATCH /api/devices/{pk}/shadow/

    GET  — returnează {reported, desired, delta, version, updated_at}
    PATCH — actualizează doar câmpul desired (user cu JWT).
    Shadow e creat automat la prima accesare.
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = DeviceShadowSerializer
    http_method_names = ["get", "patch", "head", "options"]

    def _get_device(self):
        user = self.request.user
        tenant = getattr(self.request, "tenant", None)
        if tenant is not None:
            return get_object_or_404(Device, pk=self.kwargs["pk"], tenant=tenant)
        if _is_cross_tenant(user):
            return get_object_or_404(Device, pk=self.kwargs["pk"])
        raise PermissionDenied("No active tenant context.")

    def get_object(self):
        device = self._get_device()
        shadow, _ = DeviceShadow.objects.get_or_create(device=device)
        return shadow

    def partial_update(self, request, *args, **kwargs):
        shadow = self.get_object()
        serializer = DeviceShadowSerializer(shadow, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        new_desired = {**shadow.desired, **serializer.validated_data.get("desired", {})}
        shadow.desired = new_desired
        shadow.version += 1
        shadow.save(update_fields=["desired", "version"])
        delta = {k: v for k, v in shadow.desired.items() if shadow.reported.get(k) != v}
        publish_shadow_delta(shadow.device, delta)
        return Response(DeviceShadowSerializer(shadow).data)


class DeviceShadowReportedView(generics.UpdateAPIView):
    """PATCH /api/devices/{pk}/shadow/reported/ — actualizează starea raportată.

    Folosit de service account intern (Go worker) după ce device-ul publică pe /up/shadow.
    Necesită user cu permisiunea clients.view_device (service account / superuser).
    """
    permission_classes = [IsAuthenticated]
    serializer_class = DeviceShadowReportedSerializer
    http_method_names = ["patch", "head", "options"]

    def get_object(self):
        if not _is_cross_tenant(self.request.user):
            raise PermissionDenied("Service account required.")
        device = get_object_or_404(Device, pk=self.kwargs["pk"])
        shadow, _ = DeviceShadow.objects.get_or_create(device=device)
        return shadow

    def partial_update(self, request, *args, **kwargs):
        shadow = self.get_object()
        new_reported = {**shadow.reported, **request.data.get("reported", {})}
        shadow.reported = new_reported
        shadow.version += 1
        shadow.save(update_fields=["reported", "version"])
        delta = {k: v for k, v in shadow.desired.items() if shadow.reported.get(k) != v}
        publish_shadow_delta(shadow.device, delta)
        return Response(DeviceShadowSerializer(shadow).data)


class DeviceShadowReportedBySerialView(generics.UpdateAPIView):
    """PATCH /api/shadow/reported/?serial=<serial> — lookup by serial number.

    Folosit de Go worker (nu cunoaște PK-ul Django, doar serial + tenantID din topic MQTT).
    """
    permission_classes = [IsAuthenticated]
    serializer_class = DeviceShadowReportedSerializer
    http_method_names = ["patch", "head", "options"]

    def get_object(self):
        if not _is_cross_tenant(self.request.user):
            raise PermissionDenied("Service account required.")
        serial = self.request.query_params.get("serial") or self.request.data.get("serial")
        if not serial:
            raise drf_serializers.ValidationError({"serial": "Required."})
        device = get_object_or_404(Device, serial_number=serial)
        shadow, _ = DeviceShadow.objects.get_or_create(device=device)
        return shadow

    def partial_update(self, request, *args, **kwargs):
        shadow = self.get_object()
        new_reported = {**shadow.reported, **request.data.get("reported", {})}
        shadow.reported = new_reported
        shadow.version += 1
        shadow.save(update_fields=["reported", "version"])
        delta = {k: v for k, v in shadow.desired.items() if shadow.reported.get(k) != v}
        publish_shadow_delta(shadow.device, delta)
        return Response(DeviceShadowSerializer(shadow).data)


class DeviceCommandListCreateView(APIView):
    """POST/GET /api/devices/{pk}/commands/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def _get_device(self, request):
        tenant = getattr(request, "tenant", None)
        if tenant is not None:
            return get_object_or_404(Device, pk=self.kwargs["pk"], tenant=tenant)
        if _is_cross_tenant(request.user):
            return get_object_or_404(Device, pk=self.kwargs["pk"])
        raise PermissionDenied("No active tenant context.")

    def get(self, request, pk):
        self.kwargs = {"pk": pk}
        device = self._get_device(request)
        cmds = DeviceCommand.objects.filter(device=device).order_by("-created_at")
        return Response(DeviceCommandSerializer(cmds, many=True).data)

    def post(self, request, pk):
        self.kwargs = {"pk": pk}
        device = self._get_device(request)
        role = getattr(request, "role", None)
        if not _is_cross_tenant(request.user) and role not in {"OWNER", "ADMIN"}:
            raise PermissionDenied("Only OWNER or ADMIN can send commands.")

        serializer = DeviceCommandSerializer(data=request.data)
        serializer.is_valid(raise_exception=True)

        tenant = device.tenant
        cmd = DeviceCommand.objects.create(
            device=device,
            tenant=tenant,
            action=serializer.validated_data["action"],
            payload=serializer.validated_data.get("payload", {}),
        )

        rdb = _get_redis()
        if rdb is not None:
            try:
                rdb.lpush("cmd:queue", json.dumps({
                    "command_id": cmd.id,
                    "tenant_id": device.tenant_id,
                    "serial": device.serial_number,
                    "action": cmd.action,
                    "payload": cmd.payload,
                }))
            except Exception as exc:
                logger.warning("lpush cmd:queue eșuat pentru cmd %d: %s", cmd.id, exc)

        return Response({"id": cmd.id, "status": cmd.status}, status=status.HTTP_201_CREATED)


class DeviceCommandDetailView(APIView):
    """GET /api/devices/{pk}/commands/{cmd_id}/"""
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def _get_command(self, request, pk, cmd_id):
        tenant = getattr(request, "tenant", None)
        if tenant is not None:
            return get_object_or_404(DeviceCommand, pk=cmd_id, device_id=pk, tenant=tenant)
        if _is_cross_tenant(request.user):
            return get_object_or_404(DeviceCommand, pk=cmd_id, device_id=pk)
        raise PermissionDenied("No active tenant context.")

    def get(self, request, pk, cmd_id):
        cmd = self._get_command(request, pk, cmd_id)
        return Response(DeviceCommandSerializer(cmd).data)


class DeviceCommandAckView(APIView):
    """PATCH /api/devices/{pk}/commands/{cmd_id}/ack/ or /api/devices/commands/{cmd_id}/ack/"""
    permission_classes = [IsAuthenticated]

    def patch(self, request, cmd_id, pk=None):
        if not _is_cross_tenant(request.user):
            raise PermissionDenied("Service account required.")
        filters = {"pk": cmd_id}
        if pk is not None:
            filters["device_id"] = pk
        cmd = get_object_or_404(DeviceCommand, **filters)

        new_status = request.data.get("status")
        if new_status not in {DeviceCommand.Status.EXECUTED, DeviceCommand.Status.FAILED, DeviceCommand.Status.SENT}:
            raise drf_serializers.ValidationError({"status": "Must be 'sent', 'executed', or 'failed'."})

        from django.utils import timezone
        update_fields = ["status"]
        cmd.status = new_status
        if new_status == DeviceCommand.Status.SENT and cmd.sent_at is None:
            cmd.sent_at = timezone.now()
            update_fields.append("sent_at")
        elif new_status in {DeviceCommand.Status.EXECUTED, DeviceCommand.Status.FAILED}:
            cmd.result = request.data.get("result", {})
            cmd.executed_at = timezone.now()
            update_fields += ["result", "executed_at"]
        cmd.save(update_fields=update_fields)
        return Response(DeviceCommandSerializer(cmd).data)


class CustomTokenObtainPairView(TokenObtainPairView):
    """View pentru login cu user/parolă → JWT"""
    serializer_class = CustomTokenObtainPairSerializer


class TenantListView(APIView):
    """POST /api/auth/tenants/ — returnează lista de tenanți activi ai userului."""
    permission_classes = [AllowAny]

    @extend_schema(
        request=inline_serializer(
            name="TenantListRequest",
            fields={
                "username": s.CharField(),
                "password": s.CharField(style={"input_type": "password"}),
            },
        ),
        responses=inline_serializer(
            name="TenantListResponse",
            fields={
                "slug": s.CharField(),
                "name": s.CharField(),
                "plan": s.ChoiceField(choices=["free", "pro", "enterprise"]),
                "role": s.ChoiceField(choices=["OWNER", "ADMIN", "OPERATOR", "VIEWER", "INSTALLER"]),
            },
            many=True,
        ),
        summary="Pre-login: lista de tenanți ai userului",
        description=(
            "Endpoint public — nu necesită JWT. "
            "Folosit de Flutter pentru a afișa tenant picker înainte de login. "
            "Nu emite niciun token."
        ),
        tags=["auth"],
    )
    def post(self, request):
        username = request.data.get("username", "").strip()
        password = request.data.get("password", "")
        if not username or not password:
            return Response(
                {"detail": "username și password sunt obligatorii."},
                status=status.HTTP_400_BAD_REQUEST,
            )

        user = authenticate(request, username=username, password=password)
        if user is None:
            return Response(
                {"detail": "Credențiale invalide."},
                status=status.HTTP_401_UNAUTHORIZED,
            )

        memberships = (
            Membership.objects
            .filter(user=user, tenant__status=Tenant.Status.ACTIVE)
            .select_related("tenant")
            .order_by("tenant__name")
        )

        data = [
            {
                "slug": m.tenant.slug,
                "name": m.tenant.name,
                "plan": m.tenant.plan,
                "role": m.role,
            }
            for m in memberships
        ]
        return Response(data)
