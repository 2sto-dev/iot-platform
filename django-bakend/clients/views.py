from rest_framework import serializers as drf_serializers
from rest_framework import viewsets
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from django.shortcuts import get_object_or_404
from django.contrib.auth import get_user_model
from .models import Device
from .serializers import DeviceSerializer
from rest_framework_simplejwt.views import TokenObtainPairView
from .tokens import CustomTokenObtainPairSerializer


class DeviceViewSet(viewsets.ModelViewSet):
    """CRUD pentru Device cu izolare per-tenant.

    - Superuser sau service account (perm `clients.view_device`) → cross-tenant.
    - User autentificat cu tenant context → vede TOATE device-urile din tenantul lui.
    - Fără tenant context → empty queryset.
    """
    serializer_class = DeviceSerializer
    queryset = Device.objects.all()

    def get_queryset(self):
        user = self.request.user
        if not user.is_authenticated:
            return Device.objects.none()
        if user.is_superuser or user.has_perm("clients.view_device"):
            return Device.objects.all()
        tenant_id = getattr(self.request, "tenant_id", None)
        return Device.objects.for_tenant(tenant_id)

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


@api_view(["GET"])
@permission_classes([IsAuthenticated])
def user_devices(request, username):
    """
    Returnează device-urile pentru userul specificat.
    - Superuser poate vedea device-urile oricui.
    - User normal vede doar device-urile proprii.
    """
    Client = get_user_model()
    user = get_object_or_404(Client, username=username)

    if request.user != user and not request.user.is_superuser:
        return Response({"detail": "Not authorized"}, status=403)

    devices = Device.objects.filter(client=user)
    serializer = DeviceSerializer(devices, many=True)
    return Response(serializer.data)
