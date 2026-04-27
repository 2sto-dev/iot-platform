from rest_framework import serializers
from rest_framework_simplejwt.serializers import TokenObtainPairSerializer

from tenants.models import Membership, Tenant


class CustomTokenObtainPairSerializer(TokenObtainPairSerializer):
    """Login that embeds tenant context (tenant_id, tenant_slug, role) in the JWT.

    - 0 active memberships → 400.
    - 1 active membership → implicit; `tenant_slug` (if sent) must match.
    - >=2 active memberships → request must include `tenant_slug` to disambiguate.
    """

    def validate(self, attrs):
        # Authenticate without letting the parent build tokens (we need tenant info first).
        super(TokenObtainPairSerializer, self).validate(attrs)

        # Service accounts (clients.view_device) bypass tenant selection — they're
        # cross-tenant by design. Token has no tenant_id/role claims.
        if self.user.has_perm("clients.view_device") and not self.user.is_superuser:
            refresh = self.get_token(self.user)
            return {"refresh": str(refresh), "access": str(refresh.access_token)}

        memberships = list(
            Membership.objects.filter(user=self.user, tenant__status=Tenant.Status.ACTIVE)
            .select_related("tenant")
        )
        if not memberships:
            raise serializers.ValidationError("User has no active tenant membership.")

        requested = self.context["request"].data.get("tenant_slug")
        if len(memberships) > 1:
            if not requested:
                slugs = sorted(m.tenant.slug for m in memberships)
                raise serializers.ValidationError(
                    {"tenant_slug": f"User belongs to multiple tenants. Specify one of: {slugs}"}
                )
            membership = next((m for m in memberships if m.tenant.slug == requested), None)
            if membership is None:
                raise serializers.ValidationError(
                    {"tenant_slug": "User is not a member of the requested tenant."}
                )
        else:
            membership = memberships[0]
            if requested and requested != membership.tenant.slug:
                raise serializers.ValidationError(
                    {"tenant_slug": "User is not a member of the requested tenant."}
                )

        refresh = self.get_token(self.user)
        refresh["tenant_id"] = membership.tenant_id
        refresh["tenant_slug"] = membership.tenant.slug
        refresh["role"] = membership.role

        return {
            "refresh": str(refresh),
            "access": str(refresh.access_token),
            "tenant_slug": membership.tenant.slug,
            "role": membership.role,
        }

    @classmethod
    def get_token(cls, user):
        token = super().get_token(user)
        token["username"] = user.username
        token["iss"] = "django"
        return token
