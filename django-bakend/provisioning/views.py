"""Faza 3.2 — Activation flow: endpoint public pentru prima boot a unui device."""
import hashlib
import json

from django.contrib.auth.hashers import make_password
from django.http import JsonResponse
from django.utils import timezone
from django.utils.decorators import method_decorator
from django.views import View
from django.views.decorators.csrf import csrf_exempt

from clients.models import Device
from .models import ActivationToken


def _sha256(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


@method_decorator(csrf_exempt, name="dispatch")
class ActivateView(View):
    """POST /api/provisioning/activate/ — fără JWT, apelat de device la prima pornire.

    Body JSON: {serial_number, activation_token, mqtt_password}
    Response: {activated: true, serial_number} sau 400 cu eroare.
    """

    def post(self, request):
        try:
            data = json.loads(request.body)
        except (ValueError, KeyError):
            return JsonResponse({"error": "Invalid JSON"}, status=400)

        serial = data.get("serial_number", "").strip()
        token_plain = data.get("activation_token", "").strip()
        mqtt_password = data.get("mqtt_password", "").strip()

        if not serial or not token_plain or not mqtt_password:
            return JsonResponse({"error": "serial_number, activation_token and mqtt_password are required"}, status=400)

        if len(mqtt_password) < 8:
            return JsonResponse({"error": "mqtt_password must be at least 8 characters"}, status=400)

        try:
            device = Device.objects.get(serial_number=serial)
        except Device.DoesNotExist:
            return JsonResponse({"error": "Device not found"}, status=400)

        try:
            token_obj = ActivationToken.objects.get(device=device)
        except ActivationToken.DoesNotExist:
            return JsonResponse({"error": "No activation token for this device"}, status=400)

        if not token_obj.is_valid():
            return JsonResponse({"error": "Activation token expired or already used"}, status=400)

        if token_obj.token_hash != _sha256(token_plain):
            return JsonResponse({"error": "Invalid activation token"}, status=400)

        device.mqtt_password_hash = make_password(mqtt_password, hasher="bcrypt_sha256")
        device.save(update_fields=["mqtt_password_hash"])

        token_obj.used = True
        token_obj.save(update_fields=["used"])

        return JsonResponse({"activated": True, "serial_number": serial})
