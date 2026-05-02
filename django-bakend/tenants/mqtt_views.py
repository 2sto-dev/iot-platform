"""EMQX HTTP Auth/ACL hook endpoints (Faza 2.1).

EMQX 5.x calls:
  POST /api/mqtt/auth/  — on CONNECT (authenticate device/service-account)
  POST /api/mqtt/acl/   — on PUBLISH/SUBSCRIBE (topic-level authorization)

Security: requests must carry X-Hook-Secret header matching MQTT_HOOK_SECRET env var.
If MQTT_HOOK_SECRET is empty (dev mode), the header check is skipped.

Response format (EMQX 5.x):
  {"result": "allow"} / {"result": "deny"} with HTTP 200
  {"result": "allow", "is_superuser": true} — bypasses ACL (service account)
"""
import json

from decouple import config
from django.http import JsonResponse
from django.utils.decorators import method_decorator
from django.views import View
from django.views.decorators.csrf import csrf_exempt

from clients.models import Device

# Citim din .env via decouple (os.getenv nu vede .env-ul Django, decouple da).
_SERVICE_USER = config("DJANGO_SERVICE_USER", default="")
_SERVICE_PASS = config("DJANGO_SERVICE_PASS", default="")
_HOOK_SECRET = config("MQTT_HOOK_SECRET", default="")

_ALLOW = {"result": "allow"}
_DENY = {"result": "deny"}
_ALLOW_SUPER = {"result": "allow", "is_superuser": True}


def _secret_ok(request):
    if not _HOOK_SECRET:
        return True
    return request.headers.get("X-Hook-Secret") == _HOOK_SECRET


def _parse_body(request):
    try:
        return json.loads(request.body), None
    except (ValueError, KeyError):
        return None, JsonResponse(_DENY, status=400)


@method_decorator(csrf_exempt, name="dispatch")
class MQTTAuthView(View):
    """POST /api/mqtt/auth/ — called by EMQX on every CONNECT."""

    def post(self, request):
        if not _secret_ok(request):
            return JsonResponse(_DENY, status=403)

        data, err = _parse_body(request)
        if err:
            return err

        username = data.get("username", "")
        password = data.get("password", "")

        # Service account: password-validated, gets superuser flag (bypasses ACL).
        if _SERVICE_USER and username == _SERVICE_USER:
            if _SERVICE_PASS and password != _SERVICE_PASS:
                return JsonResponse(_DENY)
            return JsonResponse(_ALLOW_SUPER)

        # Device: exists in DB → allow. Password check deferred to Faza 3.1 credentials.
        if username and Device.objects.filter(serial_number=username).exists():
            return JsonResponse(_ALLOW)

        return JsonResponse(_DENY)


@method_decorator(csrf_exempt, name="dispatch")
class MQTTACLView(View):
    """POST /api/mqtt/acl/ — called by EMQX on PUBLISH/SUBSCRIBE."""

    def post(self, request):
        if not _secret_ok(request):
            return JsonResponse(_DENY, status=403)

        data, err = _parse_body(request)
        if err:
            return err

        username = data.get("username", "")
        topic = data.get("topic", "")
        action = data.get("action", "")  # "publish" or "subscribe"

        # Service account is superuser in auth → EMQX won't call ACL, but allow anyway.
        if _SERVICE_USER and username == _SERVICE_USER:
            return JsonResponse(_ALLOW)

        # Device: look up by serial_number → verify tenant matches topic.
        try:
            device = Device.objects.select_related("tenant").get(serial_number=username)
        except Device.DoesNotExist:
            return JsonResponse(_DENY)

        tid = device.tenant_id
        serial = device.serial_number

        if action == "publish":
            # New tenant-scoped topics: tenants/{tid}/devices/{serial}/up/{stream}
            if topic.startswith(f"tenants/{tid}/devices/{serial}/up/"):
                return JsonResponse(_ALLOW)
            # Legacy vendor topics (bridge in 2.2 translates these to tenant-scoped)
            if (
                topic.startswith(f"shellies/{serial}/")
                or topic.startswith(f"tele/{serial}/")
                or topic == f"zigbee2mqtt/{serial}"
            ):
                return JsonResponse(_ALLOW)

        elif action == "subscribe":
            # Devices receive commands on down/ subtree
            if topic.startswith(f"tenants/{tid}/devices/{serial}/down/"):
                return JsonResponse(_ALLOW)
            # Legacy command topics for Shelly/Tasmota
            if topic.startswith(f"shellies/{serial}/command") or topic.startswith(f"cmnd/{serial}/"):
                return JsonResponse(_ALLOW)

        return JsonResponse(_DENY)
