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

        requested = self.context["request"].data.get("tenant_slug")
        is_service = self.user.is_superuser or self.user.has_perm("clients.view_device")

        # Service accounts (superuser or clients.view_device) bypass membership check.
        if is_service:
            if requested:
                # Superuser logged into a specific tenant context — embed tenant in JWT.
                try:
                    tenant = Tenant.objects.get(slug=requested, status=Tenant.Status.ACTIVE)
                except Tenant.DoesNotExist:
                    raise serializers.ValidationError(
                        {"tenant_slug": f"Tenant '{requested}' not found or inactive."}
                    )
                # Nu mai includem `is_service` în JWT — middleware-ul derivă privilegiul
                # server-side din `user.is_superuser` la fiecare request, ca să nu expunem
                # bit-ul de admin în browser (XSS exfil risk).
                refresh = self.get_token(self.user)
                refresh["tenant_id"] = tenant.id
                refresh["tenant_slug"] = tenant.slug
                refresh["role"] = "OWNER"
                return {
                    "refresh": str(refresh),
                    "access": str(refresh.access_token),
                    "tenant_slug": tenant.slug,
                    "role": "OWNER",
                }
            # No tenant_slug → pure cross-tenant token (no tenant claims).
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
