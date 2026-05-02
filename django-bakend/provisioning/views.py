import json
import secrets
from typing import Any, Dict

from django.http import JsonResponse, HttpRequest
from django.shortcuts import get_object_or_404
from django.views.decorators.csrf import csrf_exempt
from django.utils.decorators import method_decorator  # type: ignore[import]
from rest_framework.views import APIView
from rest_framework.permissions import IsAuthenticated, AllowAny
from rest_framework.authentication import SessionAuthentication  # type: ignore[import]

from clients.models import Device
from .models import DeviceCredential


class RotateCredentialView(APIView):
    # În test folosim force_login (session) pentru simplitate; în producție JWT va trece prin IsAuthenticated generic
    authentication_classes = [SessionAuthentication]
    permission_classes = [IsAuthenticated]

    def post(self, request: HttpRequest, device_id: int, *args, **kwargs):
        # Simplu: superuser sau service account (view_device) — RBAC fin se poate adăuga ulterior
        user = request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            return JsonResponse({"detail": "forbidden"}, status=403)

        device = get_object_or_404(Device, id=device_id)
        plain = secrets.token_urlsafe(24)
        cred, _ = DeviceCredential.objects.get_or_create(device=device)
        cred.set_secret(plain)
        cred.save()
        # Returnăm secretul o singură dată
        return JsonResponse(
            {"device": device.id, "serial": device.serial_number, "secret": plain}
        )


@method_decorator(csrf_exempt, name="dispatch")
class MQTTAuthView(APIView):
    permission_classes = [AllowAny]  # EMQX nu trimite JWT; endpoint intern cu firewall

    def post(self, request: HttpRequest, *args, **kwargs):
        try:
            data: Dict[str, Any] = (
                json.loads(request.body.decode("utf-8")) if request.body else {}
            )
        except Exception:
            data = {}
        username = (data.get("username") or "").strip()
        password = (data.get("password") or "").strip()
        clientid = (data.get("clientid") or "").strip()

        if not username or not password:
            return JsonResponse({"result": "deny"})

        try:
            device = Device.objects.get(serial_number=username)
        except Device.DoesNotExist:
            return JsonResponse({"result": "deny"})

        try:
            cred = device.credential
        except DeviceCredential.DoesNotExist:
            return JsonResponse({"result": "deny"})

        if cred.status != DeviceCredential.Status.ACTIVE:
            return JsonResponse({"result": "deny"})

        if not cred.verify(password):
            return JsonResponse({"result": "deny"})

        # Opțional: verificare că clientid == serial_number (convenție recomandată)
        if clientid and clientid != device.serial_number:
            return JsonResponse({"result": "deny"})

        return JsonResponse({"result": "allow"})
