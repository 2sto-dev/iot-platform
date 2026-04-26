from django.db import models


class TenantQuerySet(models.QuerySet):
    """QuerySet for models with a `tenant` FK. Adds .for_tenant() for explicit scoping."""

    def for_tenant(self, tenant):
        if tenant is None:
            return self.none()
        if hasattr(tenant, "id"):
            return self.filter(tenant_id=tenant.id)
        return self.filter(tenant_id=tenant)
