"""DRF permission classes for tenant role-based access.

Role matrix (per HTTP method):
- safe (GET/HEAD/OPTIONS): all roles
- POST/PUT/PATCH:           OWNER, ADMIN, OPERATOR, INSTALLER
- DELETE:                   OWNER, ADMIN
"""
from rest_framework.permissions import SAFE_METHODS, BasePermission


SAFE = set(SAFE_METHODS)
WRITE_ROLES = {"OWNER", "ADMIN", "OPERATOR", "INSTALLER"}
DELETE_ROLES = {"OWNER", "ADMIN"}
READ_ROLES = {"OWNER", "ADMIN", "OPERATOR", "VIEWER", "INSTALLER"}


def _bypass(user):
    """Superuser and service accounts (with explicit Django perms) bypass tenant RBAC."""
    if not user.is_authenticated:
        return False
    return user.is_superuser or user.has_perm("clients.view_device")


class TenantRolePermission(BasePermission):
    """Reads role from request.role (set by TenantMiddleware) and gates by method."""

    def has_permission(self, request, view):
        if not request.user.is_authenticated:
            return False
        if _bypass(request.user):
            return True
        role = getattr(request, "role", None)
        if role is None:
            return False
        if request.method in SAFE:
            return role in READ_ROLES
        if request.method == "DELETE":
            return role in DELETE_ROLES
        return role in WRITE_ROLES
