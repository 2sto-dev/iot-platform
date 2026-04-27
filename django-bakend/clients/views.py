from rest_framework import serializers as drf_serializers
from rest_framework import viewsets
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import IsAuthenticated
from rest_framework_simplejwt.views import TokenObtainPairView

from tenants.permissions import TenantRolePermission
from .models import Device
from .serializers import DeviceSerializer
from .tokens import CustomTokenObtainPairSerializer


def _is_cross_tenant(user):
    """Service accounts and superusers operate cross-tenant."""
    return user.is_superuser or user.has_perm("clients.view_device")


class DeviceViewSet(viewsets.ModelViewSet):
    """CRUD pentru Device cu izolare per-tenant + RBAC.

    Reguli de filtrare (Faza 1.9 hardened):
    - Superuser/service account → cross-tenant; pot folosi `?tenant=<id>` pentru a scopa.
    - User autentificat cu request.tenant (setat de TenantMiddleware) → DOAR device-urile
      tenantului lui. Param-ul `?tenant=` din query e IGNORAT pentru acești useri.
    - Fără request.tenant și non-cross-tenant → 403 (PermissionDenied).
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = DeviceSerializer
    queryset = Device.objects.all()

    def get_queryset(self):
        user = self.request.user
        if not user.is_authenticated:
            return Device.objects.none()

        if _is_cross_tenant(user):
            qs = Device.objects.all()
            # Cross-tenant accounts pot folosi ?tenant= pentru a filtra la un tenant specific.
            tenant_filter = self.request.query_params.get("tenant")
            if tenant_filter:
                qs = qs.filter(tenant_id=tenant_filter)
        else:
            tenant = getattr(self.request, "tenant", None)
            if tenant is None:
                # Middleware n-a setat tenant (membership invalid sau token fără tenant_id) →
                # refuzăm explicit, fără să degradăm la queryset gol (care ar masca leak-uri).
                raise PermissionDenied("No active tenant context.")
            qs = Device.objects.for_tenant(tenant)
            # Param-ul ?tenant= e IGNORAT pentru utilizatori normali (anti-spoof).

        username = self.request.query_params.get("username")
        if username:
            qs = qs.filter(client__username=username)
        return qs

    def perform_create(self, serializer):
        user = self.request.user
        if _is_cross_tenant(user):
            serializer.save()
            return
        tenant = getattr(self.request, "tenant", None)
        if tenant is None:
            raise drf_serializers.ValidationError({"tenant": "No tenant context in token."})
        # Forțează tenant + client din JWT/request, ignoră payload-ul.
        serializer.save(tenant=tenant, client=user)


class CustomTokenObtainPairView(TokenObtainPairView):
    """View pentru login cu user/parolă → JWT"""
    serializer_class = CustomTokenObtainPairSerializer
