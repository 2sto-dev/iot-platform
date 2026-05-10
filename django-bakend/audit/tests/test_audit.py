"""Tests for Faza 4.7 — Audit log."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from audit.models import AuditLog
from clients.models import Device
from tenants.models import Membership, Tenant

User = get_user_model()


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="AuditCo", slug="auditco")


@pytest.fixture
def other_tenant(db):
    return Tenant.objects.create(name="OtherCo", slug="otherco")


def _make_user(username, tenant, role, db):
    user = User.objects.create_user(username=username, password="pw", prenume=username.title())
    Membership.objects.create(user=user, tenant=tenant, role=role)
    return user


@pytest.fixture
def owner(db, tenant):
    return _make_user("owner_a", tenant, Membership.Role.OWNER, db)


@pytest.fixture
def admin_user(db, tenant):
    return _make_user("admin_a", tenant, Membership.Role.ADMIN, db)


@pytest.fixture
def viewer(db, tenant):
    return _make_user("viewer_a", tenant, Membership.Role.VIEWER, db)


@pytest.fixture
def device(db, owner, tenant):
    return Device.objects.create(
        client=owner,
        tenant=tenant,
        serial_number="AUDIT001",
        device_type="shelly_em",
    )


def _jwt_for(user, tenant):
    from rest_framework_simplejwt.tokens import AccessToken
    token = AccessToken.for_user(user)
    token["tenant_id"] = tenant.id
    token["role"] = (
        Membership.objects.filter(user=user, tenant=tenant).first().role
    )
    return str(token)


# ── Model / middleware unit tests ────────────────────────────────────────────

class TestAuditLogModel:
    def test_create_log_entry(self, db, tenant, owner):
        log = AuditLog.objects.create(
            tenant=tenant,
            actor=owner,
            action=AuditLog.Action.CREATE,
            resource_type="device",
            resource_id="123",
            metadata={"path": "/api/devices/", "status": 201},
            ip="127.0.0.1",
        )
        assert log.id is not None
        assert log.action == "create"
        assert log.resource_type == "device"

    def test_ordering_newest_first(self, db, tenant):
        import datetime
        from django.utils import timezone
        log1 = AuditLog.objects.create(tenant=tenant, action="create", resource_type="x", resource_id="1", metadata={})
        AuditLog.objects.create(tenant=tenant, action="delete", resource_type="x", resource_id="2", metadata={})
        # Push log1 into the past so ordering is deterministic regardless of resolution.
        AuditLog.objects.filter(pk=log1.pk).update(ts=timezone.now() - datetime.timedelta(seconds=5))
        logs = list(AuditLog.objects.filter(tenant=tenant))
        assert logs[0].resource_id == "2"


# ── Middleware integration tests ──────────────────────────────────────────────

@pytest.mark.django_db
class TestAuditMiddleware:
    def test_post_request_creates_log(self, api, owner, tenant, device):
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        # Any write to a tenant-scoped endpoint triggers audit
        api.patch(f"/api/devices/{device.id}/", {"serial_number": "X001"}, format="json")
        assert AuditLog.objects.filter(tenant=tenant, action="update").exists()

    def test_get_request_not_logged(self, api, owner, tenant, device):
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        before = AuditLog.objects.filter(tenant=tenant).count()
        api.get(f"/api/devices/{device.id}/")
        assert AuditLog.objects.filter(tenant=tenant).count() == before

    def test_no_tenant_context_not_logged(self, api, owner):
        """Requests without tenant context (e.g. token endpoint) are not logged."""
        before = AuditLog.objects.count()
        api.post("/api/token/", {"username": "owner_a", "password": "pw"}, format="json")
        assert AuditLog.objects.count() == before


# ── API endpoint tests ────────────────────────────────────────────────────────

@pytest.mark.django_db
class TestAuditLogEndpoint:
    def test_owner_can_list(self, api, owner, tenant):
        AuditLog.objects.create(
            tenant=tenant, action="create", resource_type="device",
            resource_id="5", metadata={}, actor=owner,
        )
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/audit/")
        assert resp.status_code == 200
        assert len(resp.data) >= 1

    def test_admin_can_list(self, api, admin_user, tenant):
        AuditLog.objects.create(
            tenant=tenant, action="delete", resource_type="device",
            resource_id="9", metadata={},
        )
        token = _jwt_for(admin_user, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/audit/")
        assert resp.status_code == 200

    def test_viewer_forbidden(self, api, viewer, tenant):
        token = _jwt_for(viewer, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/audit/")
        assert resp.status_code == 403

    def test_unauthenticated_forbidden(self, api):
        resp = api.get("/api/v1/audit/")
        assert resp.status_code == 401

    def test_tenant_isolation(self, api, owner, tenant, other_tenant, db):
        AuditLog.objects.create(
            tenant=other_tenant, action="create", resource_type="device",
            resource_id="99", metadata={},
        )
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/audit/")
        assert resp.status_code == 200
        ids = [e["resource_id"] for e in resp.data]
        assert "99" not in ids

    def test_filter_by_action(self, api, owner, tenant):
        AuditLog.objects.create(
            tenant=tenant, action="create", resource_type="device",
            resource_id="10", metadata={},
        )
        AuditLog.objects.create(
            tenant=tenant, action="delete", resource_type="device",
            resource_id="11", metadata={},
        )
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/audit/?actor=")
        assert resp.status_code == 200

    def test_filter_by_actor(self, api, owner, tenant):
        AuditLog.objects.create(
            tenant=tenant, action="create", resource_type="device",
            resource_id="20", metadata={}, actor=owner,
        )
        token = _jwt_for(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get(f"/api/v1/audit/?actor={owner.username}")
        assert resp.status_code == 200
        assert all(e["actor_username"] == owner.username for e in resp.data)
