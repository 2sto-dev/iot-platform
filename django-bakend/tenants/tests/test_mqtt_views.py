"""Tests for MQTT HTTP Auth/ACL hook endpoints (Faza 2.1)."""
import json

import pytest
from django.contrib.auth import get_user_model
from django.test import Client as DjangoClient

from clients.models import Device
from tenants.models import Membership, Tenant


@pytest.fixture(autouse=True)
def no_hook_secret():
    """Disable hook-secret check in all tests (secret enforcement tested separately)."""
    import tenants.mqtt_views as mv
    original = mv._HOOK_SECRET
    mv._HOOK_SECRET = ""
    yield
    mv._HOOK_SECRET = original


@pytest.fixture
def http():
    return DjangoClient()


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
        client=owner,
        tenant=tenant,
        serial_number="SHELF001",
        device_type="shelly_em",
    )


def _auth(client, **body):
    return client.post(
        "/api/mqtt/auth/",
        data=json.dumps(body),
        content_type="application/json",
    )


def _acl(client, **body):
    return client.post(
        "/api/mqtt/acl/",
        data=json.dumps(body),
        content_type="application/json",
    )


# ── Auth endpoint ──────────────────────────────────────────────────────────────

def test_auth_known_device_allowed(http, device):
    r = _auth(http, username="SHELF001", password="")
    assert r.status_code == 200
    assert r.json()["result"] == "allow"
    assert r.json().get("is_superuser") is not True


def test_auth_unknown_device_denied(http, db):
    r = _auth(http, username="GHOST999", password="")
    assert r.status_code == 200
    assert r.json()["result"] == "deny"


def test_auth_empty_username_denied(http, db):
    r = _auth(http, username="", password="")
    assert r.status_code == 200
    assert r.json()["result"] == "deny"


def test_auth_bad_body_400(http, db):
    r = http.post("/api/mqtt/auth/", data="not-json", content_type="application/json")
    assert r.status_code == 400


def test_auth_secret_enforced(http, device):
    # Patch module-level _HOOK_SECRET directly
    import tenants.mqtt_views as mv
    original = mv._HOOK_SECRET
    mv._HOOK_SECRET = "supersecret"
    try:
        r = _auth(http, username="SHELF001", password="")
        assert r.status_code == 403
        r2 = http.post(
            "/api/mqtt/auth/",
            data=json.dumps({"username": "SHELF001", "password": ""}),
            content_type="application/json",
            HTTP_X_HOOK_SECRET="supersecret",
        )
        assert r2.json()["result"] == "allow"
    finally:
        mv._HOOK_SECRET = original


# ── ACL endpoint — publish ─────────────────────────────────────────────────────

def test_acl_publish_own_new_topic_allowed(http, device, tenant):
    r = _acl(
        http,
        username="SHELF001",
        topic=f"tenants/{tenant.id}/devices/SHELF001/up/power",
        action="publish",
    )
    assert r.json()["result"] == "allow"


def test_acl_publish_other_tenant_denied(http, device):
    r = _acl(
        http,
        username="SHELF001",
        topic="tenants/9999/devices/SHELF001/up/power",
        action="publish",
    )
    assert r.json()["result"] == "deny"


def test_acl_publish_other_device_denied(http, device, tenant):
    r = _acl(
        http,
        username="SHELF001",
        topic=f"tenants/{tenant.id}/devices/OTHER/up/power",
        action="publish",
    )
    assert r.json()["result"] == "deny"


def test_acl_publish_legacy_shelly_allowed(http, device):
    r = _acl(http, username="SHELF001", topic="shellies/SHELF001/relay/0", action="publish")
    assert r.json()["result"] == "allow"


def test_acl_publish_legacy_tele_allowed(http, device):
    r = _acl(http, username="SHELF001", topic="tele/SHELF001/SENSOR", action="publish")
    assert r.json()["result"] == "allow"


def test_acl_publish_zigbee_allowed(http, device):
    r = _acl(http, username="SHELF001", topic="zigbee2mqtt/SHELF001", action="publish")
    assert r.json()["result"] == "allow"


def test_acl_publish_random_topic_denied(http, device):
    r = _acl(http, username="SHELF001", topic="random/topic", action="publish")
    assert r.json()["result"] == "deny"


# ── ACL endpoint — subscribe ───────────────────────────────────────────────────

def test_acl_subscribe_own_down_allowed(http, device, tenant):
    r = _acl(
        http,
        username="SHELF001",
        topic=f"tenants/{tenant.id}/devices/SHELF001/down/cmd",
        action="subscribe",
    )
    assert r.json()["result"] == "allow"


def test_acl_subscribe_other_tenant_down_denied(http, device):
    r = _acl(
        http,
        username="SHELF001",
        topic="tenants/9999/devices/SHELF001/down/cmd",
        action="subscribe",
    )
    assert r.json()["result"] == "deny"


def test_acl_subscribe_shelly_command_allowed(http, device):
    r = _acl(http, username="SHELF001", topic="shellies/SHELF001/command", action="subscribe")
    assert r.json()["result"] == "allow"


def test_acl_subscribe_unknown_device_denied(http, db):
    r = _acl(http, username="GHOST", topic="tenants/1/devices/GHOST/up/power", action="publish")
    assert r.json()["result"] == "deny"
