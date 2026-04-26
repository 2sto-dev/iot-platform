"""Tests for TenantRolePermission via DeviceViewSet."""
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
def acme_user_factory(db, acme):
    User = get_user_model()

    def _make(username, role):
        u = User.objects.create_user(username=username, password="pw", prenume=username.title())
        Membership.objects.create(user=u, tenant=acme, role=role)
        return u

    return _make


@pytest.fixture
def acme_device(acme_user_factory, acme):
    owner = acme_user_factory("owner", Membership.Role.OWNER)
    return Device.objects.create(
        client=owner,
        tenant=acme,
        serial_number="ACME-DEV-1",
        device_type="shelly_em",
    )


def login(api, username):
    r = api.post("/api/token/", {"username": username, "password": "pw"}, format="json")
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")


@pytest.mark.parametrize("role", ["OWNER", "ADMIN", "OPERATOR", "VIEWER", "INSTALLER"])
def test_all_roles_can_read(api, acme_user_factory, acme_device, role):
    acme_user_factory(f"user_{role.lower()}", role)
    login(api, f"user_{role.lower()}")
    r = api.get("/api/devices/")
    assert r.status_code == 200, r.json()


@pytest.mark.parametrize("role,expected", [
    ("OWNER", 201),
    ("ADMIN", 201),
    ("OPERATOR", 201),
    ("INSTALLER", 201),
    ("VIEWER", 403),
])
def test_create_device_by_role(api, acme_user_factory, acme, role, expected):
    acme_user_factory(f"user_{role.lower()}", role)
    login(api, f"user_{role.lower()}")
    r = api.post(
        "/api/devices/",
        {"serial_number": f"NEW-{role}", "device_type": "shelly_em"},
        format="json",
    )
    assert r.status_code == expected, r.json()


@pytest.mark.parametrize("role,expected", [
    ("OWNER", 204),
    ("ADMIN", 204),
    ("OPERATOR", 403),
    ("VIEWER", 403),
    ("INSTALLER", 403),
])
def test_delete_device_by_role(api, acme_user_factory, acme_device, role, expected):
    acme_user_factory(f"user_{role.lower()}", role)
    login(api, f"user_{role.lower()}")
    r = api.delete(f"/api/devices/{acme_device.id}/")
    assert r.status_code == expected, getattr(r, "data", r.content)
