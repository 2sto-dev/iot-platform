"""Teste pentru Faza 3.4 completă — shadow delta push via retained MQTT."""
from unittest.mock import patch

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


def test_patch_desired_publishes_delta(api, device, owner, tenant, settings):
    settings.MQTT_BROKER = ""  # no-op in tests
    _login(api, "alice", tenant_slug="acme")
    with patch("clients.views.publish_shadow_delta") as mock_pub:
        r = api.patch(
            f"/api/devices/{device.id}/shadow/",
            {"desired": {"relay_0": "off"}},
            format="json",
        )
    assert r.status_code == 200
    mock_pub.assert_called_once()
    args = mock_pub.call_args[0]
    assert args[1] == {"relay_0": "off"}  # delta = desired (reported still empty)


def test_patch_desired_delta_excludes_synced_keys(api, device, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    shadow = DeviceShadow.objects.create(
        device=device,
        reported={"relay_0": "off", "temp": 22},
        desired={"relay_0": "off"},
    )
    _login(api, "alice", tenant_slug="acme")
    with patch("clients.views.publish_shadow_delta") as mock_pub:
        api.patch(
            f"/api/devices/{device.id}/shadow/",
            {"desired": {"temp": 25}},
            format="json",
        )
    delta = mock_pub.call_args[0][1]
    # relay_0 is already in sync → only temp in delta
    assert "relay_0" not in delta
    assert delta.get("temp") == 25


def test_reported_update_republishes_delta(api, device, settings):
    settings.MQTT_BROKER = ""
    svc = get_user_model().objects.create_superuser(username="svc2", password="p", prenume="S")
    svc_api = APIClient()
    _login(svc_api, "svc2", password="p")

    DeviceShadow.objects.create(device=device, desired={"relay_0": "off"}, reported={})

    with patch("clients.views.publish_shadow_delta") as mock_pub:
        svc_api.patch(
            f"/api/shadow/reported/?serial=SHELF001",
            {"reported": {"relay_0": "off"}},
            format="json",
        )
    delta = mock_pub.call_args[0][1]
    assert delta == {}  # fully synced


def test_publisher_no_op_when_broker_not_set(device, settings):
    settings.MQTT_BROKER = ""
    from clients.mqtt_publisher import publish_shadow_delta
    # Must not raise even without paho-mqtt or broker
    publish_shadow_delta(device, {"relay": "off"})


def test_publisher_no_op_on_broker_unavailable(device, settings):
    settings.MQTT_BROKER = "127.0.0.1:19999"  # nici un broker pe portul ăsta
    from clients.mqtt_publisher import publish_shadow_delta
    # Trebuie să înghită excepția și să logheze warning, nu să ridice
    publish_shadow_delta(device, {"relay": "off"})
