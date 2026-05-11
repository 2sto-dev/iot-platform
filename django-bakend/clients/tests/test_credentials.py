"""Tests for Faza 3.1 — Device credentials (mqtt_password_hash + rotate endpoint)."""
import json

import pytest
from django.contrib.auth import get_user_model
from django.contrib.auth.hashers import make_password, check_password
from django.test import Client as DjangoClient
from rest_framework.test import APIClient

from clients.models import Device
from tenants.models import Membership, Tenant


@pytest.fixture
def api():
    return APIClient()


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
def viewer(db, tenant):
    user = get_user_model().objects.create_user(username="viewer1", password="pw", prenume="Viewer")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.VIEWER)
    return user


@pytest.fixture
def device(db, owner, tenant):
    return Device.objects.create(
        client=owner,
        tenant=tenant,
        serial_number="SHELF001",
        device_type="shelly_em",
    )


def login(api, username, password="pw", tenant_slug=None):
    payload = {"username": username, "password": password}
    if tenant_slug:
        payload["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", payload, format="json")
    assert r.status_code == 200, r.json()
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")


def _auth(client, **body):
    return client.post(
        "/api/mqtt/auth/",
        data=json.dumps(body),
        content_type="application/json",
    )


@pytest.fixture(autouse=True)
def no_hook_secret():
    import tenants.mqtt_views as mv
    original = mv._HOOK_SECRET
    mv._HOOK_SECRET = ""
    yield
    mv._HOOK_SECRET = original


# ── rotate endpoint ────────────────────────────────────────────────────────────

def test_rotate_sets_hash(api, device, owner, tenant):
    login(api, "alice", tenant_slug="acme")
    r = api.post(f"/api/devices/{device.id}/credentials/rotate/")
    assert r.status_code == 200
    data = r.json()
    assert "mqtt_password" in data
    assert len(data["mqtt_password"]) > 10
    device.refresh_from_db()
    assert device.mqtt_password_hash != ""


def test_rotate_returns_plain_password(api, device, owner, tenant):
    login(api, "alice", tenant_slug="acme")
    r = api.post(f"/api/devices/{device.id}/credentials/rotate/")
    plain = r.json()["mqtt_password"]
    device.refresh_from_db()
    assert check_password(plain, device.mqtt_password_hash)


def test_rotate_requires_owner_or_admin(api, device, viewer, tenant):
    login(api, "viewer1", tenant_slug="acme")
    r = api.post(f"/api/devices/{device.id}/credentials/rotate/")
    assert r.status_code == 403


def test_rotate_unauthenticated_rejected(api, device, db):
    r = api.post(f"/api/devices/{device.id}/credentials/rotate/")
    assert r.status_code == 401


# ── EMQX auth cu password hash ─────────────────────────────────────────────────

def test_auth_with_correct_password(http, device):
    device.mqtt_password_hash = make_password("secret123", hasher="bcrypt_sha256")
    device.save()
    r = _auth(http, username="SHELF001", password="secret123")
    assert r.json()["result"] == "allow"


def test_auth_with_wrong_password(http, device):
    device.mqtt_password_hash = make_password("secret123", hasher="bcrypt_sha256")
    device.save()
    r = _auth(http, username="SHELF001", password="wrongpassword")
    assert r.json()["result"] == "deny"


def test_auth_no_hash_denies(http, device):
    # Device fără hash → DENY (regression test: compat-mode-allow eliminată).
    # Migrarea forțată trebuie făcută cu management command force_rotate_mqtt_credentials.
    assert device.mqtt_password_hash == ""
    r = _auth(http, username="SHELF001", password="")
    assert r.json()["result"] == "deny"


def test_auth_no_hash_denies_any_password(http, device):
    # Fără hash, NICIO parolă nu trebuie acceptată — nici random, nici gol, nici "anything".
    r = _auth(http, username="SHELF001", password="anything")
    assert r.json()["result"] == "deny"


def test_auth_unknown_serial_denies(http, db):
    # Defense-in-depth: serial inexistent → deny (același mesaj ca hash gol, nu leak existență).
    r = _auth(http, username="DOES-NOT-EXIST", password="anything")
    assert r.json()["result"] == "deny"
