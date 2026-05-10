from rest_framework import generics, status
from rest_framework.exceptions import PermissionDenied
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response

from tenants.permissions import TenantRolePermission
from .models import APIKey
from .serializers import APIKeySerializer, APIKeyCreateSerializer, APIKeyCreateResponseSerializer

_CREATE_ROLES = {"OWNER", "ADMIN"}


class APIKeyListCreateView(generics.GenericAPIView):
    """GET /api/v1/api-keys/ — list keys for the current tenant.
    POST /api/v1/api-keys/ — create a new key (returns plain_key once).

    OWNER/ADMIN only.
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def _require_write_role(self):
        user = self.request.user
        if user.is_superuser or user.has_perm("clients.view_device"):
            return
        role = getattr(self.request, "role", None)
        if role not in _CREATE_ROLES:
            raise PermissionDenied("Only OWNER and ADMIN can manage API keys.")

    def _get_tenant(self):
        tenant = getattr(self.request, "tenant", None)
        if tenant is None:
            raise PermissionDenied("No active tenant context.")
        return tenant

    def get(self, request):
        self._require_write_role()
        tenant = self._get_tenant()
        keys = APIKey.objects.filter(tenant=tenant, revoked=False)
        return Response(APIKeySerializer(keys, many=True).data)

    def post(self, request):
        self._require_write_role()
        tenant = self._get_tenant()
        ser = APIKeyCreateSerializer(data=request.data)
        ser.is_valid(raise_exception=True)
        key, plain = APIKey.generate(
            tenant=tenant,
            name=ser.validated_data["name"],
            created_by=request.user if request.user.is_authenticated else None,
            scopes=ser.validated_data.get("scopes", []),
            expires_at=ser.validated_data.get("expires_at"),
        )
        data = APIKeyCreateResponseSerializer(key).data
        data["plain_key"] = plain
        return Response(data, status=status.HTTP_201_CREATED)


class APIKeyRevokeView(generics.GenericAPIView):
    """DELETE /api/v1/api-keys/{id}/ — revoke a key.

    OWNER/ADMIN only; key must belong to the current tenant.
    """
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def delete(self, request, pk):
        user = self.request.user
        if not (user.is_superuser or user.has_perm("clients.view_device")):
            role = getattr(request, "role", None)
            if role not in _CREATE_ROLES:
                raise PermissionDenied("Only OWNER and ADMIN can revoke API keys.")

        tenant = getattr(request, "tenant", None)
        if tenant is None:
            raise PermissionDenied("No active tenant context.")

        key = APIKey.objects.filter(pk=pk, tenant=tenant).first()
        if key is None:
            return Response(status=status.HTTP_404_NOT_FOUND)
        key.revoked = True
        key.save(update_fields=["revoked"])
        return Response(status=status.HTTP_204_NO_CONTENT)
