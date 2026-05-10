"""Tests pentru Faza 3.3 — DeviceCommand (downlink commands + ACK tracking)."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device, DeviceCommand
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


def test_create_command_queued(api, device, owner, tenant, settings):
    settings.REDIS_URL = ""  # disable Redis in tests
    _login(api, "alice", tenant_slug="acme")
    r = api.post(
        f"/api/devices/{device.id}/commands/",
        {"action": "turn_off_relay", "payload": {"relay": 0}},
        format="json",
    )
    assert r.status_code == 201
    data = r.json()
    assert data["status"] == "queued"
    assert DeviceCommand.objects.filter(device=device, action="turn_off_relay").exists()


def test_owner_can_create_command(api, device, owner, tenant, settings):
    settings.REDIS_URL = ""
    _login(api, "alice", tenant_slug="acme")
    r = api.post(
        f"/api/devices/{device.id}/commands/",
        {"action": "restart"},
        format="json",
    )
    assert r.status_code == 201


def test_viewer_cannot_create_command(api, device, viewer, tenant, settings):
    settings.REDIS_URL = ""
    _login(api, "viewer1", tenant_slug="acme")
    r = api.post(
        f"/api/devices/{device.id}/commands/",
        {"action": "restart"},
        format="json",
    )
    assert r.status_code == 403


def test_list_commands(api, device, owner, tenant, settings):
    settings.REDIS_URL = ""
    DeviceCommand.objects.create(device=device, tenant=tenant, action="restart")
    DeviceCommand.objects.create(device=device, tenant=tenant, action="turn_off_relay")
    _login(api, "alice", tenant_slug="acme")
    r = api.get(f"/api/devices/{device.id}/commands/")
    assert r.status_code == 200
    assert len(r.json()) == 2


def test_get_command_detail(api, device, owner, tenant, settings):
    settings.REDIS_URL = ""
    cmd = DeviceCommand.objects.create(device=device, tenant=tenant, action="restart")
    _login(api, "alice", tenant_slug="acme")
    r = api.get(f"/api/devices/{device.id}/commands/{cmd.id}/")
    assert r.status_code == 200
    assert r.json()["action"] == "restart"
    assert r.json()["timed_out"] is False


def test_ack_updates_status_executed(api, device, service_account, tenant, settings):
    settings.REDIS_URL = ""
    cmd = DeviceCommand.objects.create(device=device, tenant=tenant, action="restart")
    _login(api, "svc", password="svc-pass")
    r = api.patch(
        f"/api/devices/{device.id}/commands/{cmd.id}/ack/",
        {"status": "executed", "result": {"ok": True}},
        format="json",
    )
    assert r.status_code == 200
    data = r.json()
    assert data["status"] == "executed"
    cmd.refresh_from_db()
    assert cmd.result == {"ok": True}
    assert cmd.executed_at is not None


def test_ack_updates_status_failed(api, device, service_account, tenant, settings):
    settings.REDIS_URL = ""
    cmd = DeviceCommand.objects.create(device=device, tenant=tenant, action="restart")
    _login(api, "svc", password="svc-pass")
    r = api.patch(
        f"/api/devices/{device.id}/commands/{cmd.id}/ack/",
        {"status": "failed", "result": {"error": "timeout"}},
        format="json",
    )
    assert r.status_code == 200
    assert r.json()["status"] == "failed"


def test_ack_requires_service_account(api, device, owner, tenant, settings):
    settings.REDIS_URL = ""
    cmd = DeviceCommand.objects.create(device=device, tenant=tenant, action="restart")
    _login(api, "alice", tenant_slug="acme")
    r = api.patch(
        f"/api/devices/{device.id}/commands/{cmd.id}/ack/",
        {"status": "executed"},
        format="json",
    )
    assert r.status_code == 403


def test_list_commands_scoped_to_tenant(api, device, tenant, owner, db, settings):
    settings.REDIS_URL = ""
    other_tenant = Tenant.objects.create(name="Other", slug="other")
    other_user = get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")
    Membership.objects.create(user=other_user, tenant=other_tenant, role=Membership.Role.OWNER)

    DeviceCommand.objects.create(device=device, tenant=tenant, action="alice_cmd")

    other_device = Device.objects.create(
        client=other_user, tenant=other_tenant, serial_number="OTHER001", device_type="shelly_em"
    )
    DeviceCommand.objects.create(device=other_device, tenant=other_tenant, action="bob_cmd")

    _login(api, "alice", tenant_slug="acme")
    r = api.get(f"/api/devices/{device.id}/commands/")
    assert r.status_code == 200
    actions = [c["action"] for c in r.json()]
    assert "alice_cmd" in actions
    assert "bob_cmd" not in actions

    # Bob cannot see Alice's device
    bob_api = APIClient()
    _login(bob_api, "bob", tenant_slug="other")
    r2 = bob_api.get(f"/api/devices/{device.id}/commands/")
    assert r2.status_code == 404
