"""Tests pentru Faza 3.4 — Device shadow (reported + desired + delta)."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device, DeviceShadow
from tenants.models import Membership, Tenant


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def owner(db, tenant):
    user = get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.OWNER)
    return user


@pytest.fixture
def viewer(db, tenant):
    user = get_user_model().objects.create_user(username="viewer1", password="pw", prenume="Viewer")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.VIEWER)
    return user


@pytest.fixture
def service_account(db):
    return get_user_model().objects.create_superuser(
        username="svc", password="svc-pass", prenume="Service"
    )


@pytest.fixture
def device(db, owner, tenant):
    return Device.objects.create(
        client=owner, tenant=tenant, serial_number="SHELF001", device_type="shelly_em"
    )


def _login(api, username, password="pw", tenant_slug=None):
    payload = {"username": username, "password": password}
    if tenant_slug:
        payload["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", payload, format="json")
    assert r.status_code == 200, r.json()
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")


def test_shadow_created_on_first_get(api, device, owner, tenant):
    _login(api, "alice", tenant_slug="acme")
    r = api.get(f"/api/devices/{device.id}/shadow/")
    assert r.status_code == 200
    data = r.json()
    assert data["reported"] == {}
    assert data["desired"] == {}
    assert data["delta"] == {}
    assert data["version"] == 0
    assert DeviceShadow.objects.filter(device=device).exists()


def test_get_shadow_idempotent(api, device, owner, tenant):
    _login(api, "alice", tenant_slug="acme")
    api.get(f"/api/devices/{device.id}/shadow/")
    api.get(f"/api/devices/{device.id}/shadow/")
    assert DeviceShadow.objects.filter(device=device).count() == 1


def test_patch_desired_updates_shadow(api, device, owner, tenant):
    _login(api, "alice", tenant_slug="acme")
    r = api.patch(
        f"/api/devices/{device.id}/shadow/",
        {"desired": {"relay_0": "off"}},
        format="json",
    )
    assert r.status_code == 200
    data = r.json()
    assert data["desired"]["relay_0"] == "off"
    assert data["delta"]["relay_0"] == "off"
    shadow = DeviceShadow.objects.get(device=device)
    assert shadow.desired == {"relay_0": "off"}
    assert shadow.version == 1


def test_delta_clears_when_reported_matches(api, device, owner, tenant, service_account):
    _login(api, "alice", tenant_slug="acme")
    api.patch(f"/api/devices/{device.id}/shadow/", {"desired": {"relay_0": "off"}}, format="json")

    svc_api = APIClient()
    _login(svc_api, "svc", password="svc-pass")
    svc_api.patch(
        f"/api/devices/{device.id}/shadow/reported/",
        {"reported": {"relay_0": "off"}},
        format="json",
    )

    r = api.get(f"/api/devices/{device.id}/shadow/")
    assert r.json()["delta"] == {}


def test_viewer_can_read_shadow(api, device, viewer, tenant):
    _login(api, "viewer1", tenant_slug="acme")
    r = api.get(f"/api/devices/{device.id}/shadow/")
    assert r.status_code == 200


def test_viewer_cannot_patch_desired(api, device, viewer, tenant):
    _login(api, "viewer1", tenant_slug="acme")
    r = api.patch(
        f"/api/devices/{device.id}/shadow/",
        {"desired": {"relay_0": "off"}},
        format="json",
    )
    assert r.status_code == 403


def test_reported_update_service_account(api, device, service_account):
    _login(api, "svc", password="svc-pass")
    r = api.patch(
        f"/api/devices/{device.id}/shadow/reported/",
        {"reported": {"temp": 22.5}},
        format="json",
    )
    assert r.status_code == 200
    shadow = DeviceShadow.objects.get(device=device)
    assert shadow.reported["temp"] == 22.5


def test_reported_update_normal_user_rejected(api, device, owner, tenant):
    _login(api, "alice", tenant_slug="acme")
    r = api.patch(
        f"/api/devices/{device.id}/shadow/reported/",
        {"reported": {"temp": 22.5}},
        format="json",
    )
    assert r.status_code == 403


def test_shadow_not_accessible_cross_tenant(api, device, tenant, db):
    other_tenant = Tenant.objects.create(name="Other", slug="other")
    other_user = get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")
    Membership.objects.create(user=other_user, tenant=other_tenant, role=Membership.Role.OWNER)
    _login(api, "bob", tenant_slug="other")
    r = api.get(f"/api/devices/{device.id}/shadow/")
    assert r.status_code == 404
