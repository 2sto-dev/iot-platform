"""Tests for DeviceViewSet — auth, tenant isolation, service-account bypass."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device
from tenants.models import Membership, Tenant


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


@pytest.fixture
def bob(db):
    return get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")


@pytest.fixture
def alice_in_acme(alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    return alice


@pytest.fixture
def bob_in_globex(bob, globex):
    Membership.objects.create(user=bob, tenant=globex, role=Membership.Role.OWNER)
    return bob


@pytest.fixture
def acme_device(alice, acme):
    return Device.objects.create(
        client=alice,
        tenant=acme,
        serial_number="ACME-001",
        device_type="shelly_em",
    )


@pytest.fixture
def globex_device(bob, globex):
    return Device.objects.create(
        client=bob,
        tenant=globex,
        serial_number="GLOBEX-001",
        device_type="nous_at",
    )


def login(api, username, password="pw", tenant_slug=None):
    payload = {"username": username, "password": password}
    if tenant_slug:
        payload["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", payload, format="json")
    assert r.status_code == 200, r.json()
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")


def test_anonymous_request_rejected(api):
    response = api.get("/api/devices/")
    assert response.status_code == 401


def test_user_sees_devices_in_own_tenant(api, alice_in_acme, acme_device, bob_in_globex, globex_device):
    login(api, "alice")
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["ACME-001"]


def test_user_does_not_see_other_tenant_devices(api, alice_in_acme, acme_device, bob_in_globex, globex_device):
    login(api, "bob")
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["GLOBEX-001"]


def test_superuser_sees_all_devices(api, acme_device, globex_device):
    User = get_user_model()
    User.objects.create_superuser(username="root", password="pw", prenume="Root")
    api.force_authenticate(user=User.objects.get(username="root"))
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["ACME-001", "GLOBEX-001"]


def test_service_account_sees_all_devices(api, acme_device, globex_device):
    """User with `clients.view_device` permission bypasses tenant isolation (cross-tenant ingest)."""
    from django.contrib.auth.models import Permission
    from django.contrib.contenttypes.models import ContentType

    User = get_user_model()
    svc = User.objects.create_user(username="iot-ingest", password="pw", prenume="Ingest")
    ct = ContentType.objects.get_for_model(Device)
    svc.user_permissions.add(Permission.objects.get(content_type=ct, codename="view_device"))
    api.force_authenticate(user=svc)
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["ACME-001", "GLOBEX-001"]


def test_filter_by_username_query_param(api, alice_in_acme, acme_device):
    other = get_user_model().objects.create_user(username="charlie", password="pw", prenume="C")
    Device.objects.create(client=other, tenant=acme_device.tenant, serial_number="CHARLIE-1", device_type="shelly_em")
    login(api, "alice")
    r = api.get("/api/devices/?username=alice")
    assert r.status_code == 200
    serials = [d["serial_number"] for d in r.json()]
    assert serials == ["ACME-001"]


def test_filter_by_tenant_query_param_for_service_account(api, acme_device, globex_device):
    """Service account uses ?tenant= to scope cross-tenant queries."""
    from django.contrib.auth.models import Permission
    from django.contrib.contenttypes.models import ContentType
    User = get_user_model()
    svc = User.objects.create_user(username="iot-ingest", password="pw", prenume="Ingest")
    ct = ContentType.objects.get_for_model(Device)
    svc.user_permissions.add(Permission.objects.get(content_type=ct, codename="view_device"))
    api.force_authenticate(user=svc)
    r = api.get(f"/api/devices/?tenant={acme_device.tenant_id}")
    assert r.status_code == 200
    serials = [d["serial_number"] for d in r.json()]
    assert serials == ["ACME-001"]


def test_device_create_uses_tenant_from_jwt(api, alice_in_acme, acme, globex):
    """Even if user POSTs tenant=globex.id, the JWT's tenant (acme) is used."""
    login(api, "alice")
    response = api.post(
        "/api/devices/",
        {
            "serial_number": "NEW-001",
            "description": "test",
            "device_type": "shelly_em",
            "tenant": globex.id,  # spoof attempt with a real other tenant
        },
        format="json",
    )
    assert response.status_code == 201, response.json()
    assert response.json()["tenant"] == acme.id  # NOT globex
    assert response.json()["client"] == alice_in_acme.id
