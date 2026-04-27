"""Tests for the hardened TenantMiddleware (Faza 1.9):
- membership re-check at every request
- 403 if user lacks active membership in token's tenant
- request.tenant set as Tenant instance
- ?tenant= query param ignored for regular users
"""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device
from tenants.middleware import _membership_cache, invalidate_membership_cache
from tenants.models import Membership, Tenant


@pytest.fixture(autouse=True)
def clear_membership_cache():
    invalidate_membership_cache()
    yield
    invalidate_membership_cache()


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def acme(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def globex(db):
    return Tenant.objects.create(name="Globex", slug="globex")


@pytest.fixture
def alice(db):
    return get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")


def login(api, username, password="pw", tenant_slug=None):
    payload = {"username": username, "password": password}
    if tenant_slug:
        payload["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", payload, format="json")
    assert r.status_code == 200, r.json()
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")
    return r.json()["access"]


def test_revoked_membership_blocks_access(api, alice, acme):
    """User logs in, then membership is revoked → JWT is still valid but middleware → 403."""
    m = Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    login(api, "alice")

    r = api.get("/api/devices/")
    assert r.status_code == 200

    m.delete()  # revoke; signal invalidates cache
    r = api.get("/api/devices/")
    assert r.status_code == 403
    assert "membership" in r.json()["detail"].lower()


def test_suspended_tenant_blocks_access(api, alice, acme):
    """Tenant becomes suspended after login → middleware → 403."""
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    login(api, "alice")

    r = api.get("/api/devices/")
    assert r.status_code == 200

    acme.status = Tenant.Status.SUSPENDED
    acme.save()  # signal invalidates cache

    r = api.get("/api/devices/")
    assert r.status_code == 403


def test_query_param_tenant_ignored_for_regular_user(api, alice, acme, globex):
    """Even if a regular user tries ?tenant=globex.id, they only see acme devices."""
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    Device.objects.create(client=alice, tenant=acme, serial_number="ACME-1", device_type="shelly_em")
    bob = get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")
    Device.objects.create(client=bob, tenant=globex, serial_number="GLOBEX-1", device_type="shelly_em")

    login(api, "alice")
    # Try to spoof: use ?tenant=globex.id → ignored, alice still sees only acme.
    r = api.get(f"/api/devices/?tenant={globex.id}")
    assert r.status_code == 200
    serials = sorted(d["serial_number"] for d in r.json())
    assert serials == ["ACME-1"]


def test_query_param_tenant_works_for_service_account(api, acme, globex):
    """Service account CAN use ?tenant= to scope cross-tenant queries."""
    from django.contrib.auth.models import Permission
    from django.contrib.contenttypes.models import ContentType

    User = get_user_model()
    svc = User.objects.create_user(username="iot-ingest", password="pw", prenume="Ingest")
    ct = ContentType.objects.get_for_model(Device)
    svc.user_permissions.add(Permission.objects.get(content_type=ct, codename="view_device"))

    a = User.objects.create_user(username="a", password="pw", prenume="A")
    b = User.objects.create_user(username="b", password="pw", prenume="B")
    Device.objects.create(client=a, tenant=acme, serial_number="ACME-1", device_type="shelly_em")
    Device.objects.create(client=b, tenant=globex, serial_number="GLOBEX-1", device_type="shelly_em")

    api.force_authenticate(user=svc)
    r = api.get(f"/api/devices/?tenant={acme.id}")
    assert r.status_code == 200
    serials = [d["serial_number"] for d in r.json()]
    assert serials == ["ACME-1"]


def test_membership_cache_hit(alice, acme):
    """Second resolve in TTL window doesn't hit DB."""
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    invalidate_membership_cache()

    from tenants.middleware import _resolve_tenant
    t1 = _resolve_tenant(alice.id, acme.id)
    assert t1 is not None
    # Second call hits cache.
    assert (alice.id, acme.id) in _membership_cache
    t2 = _resolve_tenant(alice.id, acme.id)
    assert t2 is t1


def test_signal_invalidates_cache_on_membership_delete(alice, acme):
    """Deleting a Membership clears its cache entry via post_delete signal."""
    m = Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    from tenants.middleware import _resolve_tenant
    _resolve_tenant(alice.id, acme.id)
    assert (alice.id, acme.id) in _membership_cache
    m.delete()
    assert (alice.id, acme.id) not in _membership_cache
