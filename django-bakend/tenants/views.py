from django.http import JsonResponse
from rest_framework.views import APIView
from rest_framework.permissions import AllowAny
from django.conf import settings

from clients.models import Device
from .models import Tenant

ALLOWED_LEGACY_PREFIXES = ("shellies/", "tele/", "zigbee2mqtt/")
BRIDGE_USER = getattr(settings, "MQTT_BRIDGE_USER", "mqtt-bridge")


class MQTTACLView(APIView):
    permission_classes = [AllowAny]  # EMQX nu folosește JWT; endpoint intern (protejat la nivel de rețea)

    def post(self, request):
        # EMQX 5: { "clientid": "...", "username": "...", "topic": "...", "action": "publish|subscribe" }
        clientid = (request.data.get("clientid") or "").strip()
        username = (request.data.get("username") or "").strip()
        topic = (request.data.get("topic") or "").strip()
        action = (request.data.get("action") or "").strip()

        # 1) Legacy vendor topics: allow (bridge va republish-ui; în Faza 3 vom închide)
        for p in ALLOWED_LEGACY_PREFIXES:
            if topic.startswith(p):
                return JsonResponse({"result": "allow"})

        # 2) Tenant-scoped topics: tenants/{tid}/devices/{did}/up|down/...
        if topic.startswith("tenants/"):
            parts = topic.split("/")
            if len(parts) < 6 or parts[0] != "tenants" or parts[2] != "devices":
                return JsonResponse({"result": "deny"})
            tid_str, did, direction = parts[1], parts[3], parts[4]
            if direction not in ("up", "down"):
                return JsonResponse({"result": "deny"})
            try:
                tid = int(tid_str)
                if tid <= 0:
                    raise ValueError()
            except Exception:
                return JsonResponse({"result": "deny"})

            try:
                dev = Device.objects.select_related("tenant").get(serial_number=did)
            except Device.DoesNotExist:
                return JsonResponse({"result": "deny"})

            if dev.tenant_id != tid:
                return JsonResponse({"result": "deny"})

            # Publish 'up' permis pentru device (clientid=serial) sau bridge (username fix)
            if action == "publish" and direction == "up":
                if username == BRIDGE_USER or clientid == dev.serial_number:
                    return JsonResponse({"result": "allow"})
                return JsonResponse({"result": "deny"})

            # Subscribe conservator: doar la up/# în Faza 2
            if action == "subscribe":
                if direction == "up":
                    return JsonResponse({"result": "allow"})
                return JsonResponse({"result": "deny"})

        return JsonResponse({"result": "deny"})


class TenantPlanView(APIView):
    permission_classes = [AllowAny]  # apelat de service-ul Go cu service account JWT

    def get(self, request, tenant_id: int):
        try:
            t = Tenant.objects.get(id=tenant_id, status=Tenant.Status.ACTIVE)
        except Tenant.DoesNotExist:
            return JsonResponse({"detail": "not found"}, status=404)
        return JsonResponse({"plan": t.plan})
