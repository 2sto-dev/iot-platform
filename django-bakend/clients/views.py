from rest_framework import serializers as drf_serializers
from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from tenants.permissions import TenantRolePermission
from .models import Device
from .serializers import DeviceSerializer
from rest_framework_simplejwt.views import TokenObtainPairView
from .tokens import CustomTokenObtainPairSerializer


class DeviceViewSet(viewsets.ModelViewSet):
    """CRUD pentru Device cu izolare per-tenant + RBAC.

    - Superuser sau service account (perm clients.view_device) → cross-tenant.
    - User autentificat cu tenant context → device-urile din tenantul lui.
    - Filtru opțional: ?username=<username>.
    - RBAC: gestionat de TenantRolePermission (vezi tenants/permissions.py).
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]
    serializer_class = DeviceSerializer
    queryset = Device.objects.all()

    def get_queryset(self):
        user = self.request.user
        if not user.is_authenticated:
            return Device.objects.none()
        if user.is_superuser or user.has_perm("clients.view_device"):
            qs = Device.objects.all()
        else:
            tenant_id = getattr(self.request, "tenant_id", None)
            qs = Device.objects.for_tenant(tenant_id)
        username = self.request.query_params.get("username")
        if username:
            qs = qs.filter(client__username=username)
        return qs

    def perform_create(self, serializer):
        user = self.request.user
        if user.is_superuser or user.has_perm("clients.add_device"):
            serializer.save()
            return
        tenant_id = getattr(self.request, "tenant_id", None)
        if tenant_id is None:
            raise drf_serializers.ValidationError({"tenant": "No tenant context in token."})
        serializer.save(tenant_id=tenant_id, client=user)


class CustomTokenObtainPairView(TokenObtainPairView):
    """View pentru login cu user/parolă → JWT"""
    serializer_class = CustomTokenObtainPairSerializer
